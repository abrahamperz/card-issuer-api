package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/novopayment/card-issuer-api/internal/config"
	"github.com/novopayment/card-issuer-api/internal/crypto"
	"github.com/novopayment/card-issuer-api/internal/domain"
	"github.com/novopayment/card-issuer-api/internal/handler"
	"github.com/novopayment/card-issuer-api/internal/middleware"
	"github.com/novopayment/card-issuer-api/internal/platform/database"
	"github.com/novopayment/card-issuer-api/internal/platform/logger"
	"github.com/novopayment/card-issuer-api/internal/repository"
	"github.com/novopayment/card-issuer-api/internal/repository/memory"
	"github.com/novopayment/card-issuer-api/internal/service"
)

func main() {
	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Configuration error: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize PAN-safe structured logger
	appLogger := logger.New(cfg.LogLevel, cfg.LogFormat, os.Stdout)
	appLogger.Info("starting card issuer API service",
		"addr", cfg.ServerAddr,
		"storage_driver", cfg.StorageDriver,
		"batch_chunk_size", cfg.BatchChunkSize,
		"batch_workers", cfg.BatchWorkerCount,
	)

	// 3. Initialize cryptographic components (Key Separation Principle)
	encryptor, err := crypto.NewAESGCMEncryptor(cfg.EncryptionKey)
	if err != nil {
		appLogger.Error("failed to initialize AES-GCM encryptor", "error", err)
		os.Exit(1)
	}

	indexer, err := crypto.NewHMACBlindIndexer(cfg.BlindIndexKey)
	if err != nil {
		appLogger.Error("failed to initialize HMAC blind indexer", "error", err)
		os.Exit(1)
	}

	// 4. Modular Repository & Driver Setup (Hexagonal Ports & Adapters)
	var (
		cardRepo         repository.CardRepository
		cardholderRepo   repository.CardholderRepository
		auditRepo        repository.AuditRepository
		batchRepo        repository.BatchRepository
		idempotencyStore middleware.IdempotencyStore
		tenantLookup     middleware.TenantLookup
		txManager        repository.TxManager
		healthChecker    handler.HealthChecker
		cleanupFunc      func()
	)

	if cfg.StorageDriver == "memory" {
		appLogger.Info("using modular IN-MEMORY storage driver (zero external dependencies required)")

		memCardRepo := memory.NewMemoryCardRepository()
		memCardholderRepo := memory.NewMemoryCardholderRepository()
		memAuditRepo := memory.NewMemoryAuditRepository()
		memBatchRepo := memory.NewMemoryBatchRepository()
		memIdempRepo := memory.NewMemoryIdempotencyRepository()
		memTenantRepo := memory.NewMemoryTenantRepository()

		cardRepo = memCardRepo
		cardholderRepo = memCardholderRepo
		auditRepo = memAuditRepo
		batchRepo = memBatchRepo
		idempotencyStore = repository.NewIdempotencyStoreAdapter(memIdempRepo)
		tenantLookup = memTenantRepo
		txManager = memory.NewMemoryTxManager(memCardRepo, memCardholderRepo, memAuditRepo, memBatchRepo)
		healthChecker = &memory.MemoryHealthChecker{}
		cleanupFunc = func() {}

		// Pre-seed mock data for immediate live testing
		seedDemoData(memTenantRepo, memCardholderRepo, memCardRepo, encryptor, indexer, appLogger)

	} else {
		appLogger.Info("using POSTGRESQL storage driver with Row-Level Security", "dsn", cfg.DatabaseDSN)

		db, err := database.New(
			cfg.DatabaseDSN,
			cfg.MaxOpenConns,
			cfg.MaxIdleConns,
			cfg.ConnMaxLifetime,
			appLogger,
		)
		if err != nil {
			appLogger.Error("failed to connect to database", "error", err)
			os.Exit(1)
		}
		cleanupFunc = func() { _ = db.Close() }

		txManager = repository.NewTxManager(db.DB)
		cardRepo = repository.NewCardRepository(db.DB)
		cardholderRepo = repository.NewCardholderRepository(db.DB)
		auditRepo = repository.NewAuditRepository(db.DB)
		batchRepo = repository.NewBatchRepository(db.DB)
		idempRepo := repository.NewIdempotencyRepository(db.DB)
		idempotencyStore = repository.NewIdempotencyStoreAdapter(idempRepo)
		tenantLookup = repository.NewTenantRepository(db.DB)
		healthChecker = db
	}
	defer cleanupFunc()

	// 5. Initialize Business Services (Pure Domain & Crypto Logic)
	cardService := service.NewCardService(cardRepo, auditRepo, encryptor, indexer, txManager, appLogger)
	cardholderService := service.NewCardholderService(cardholderRepo)
	batchService := service.NewBatchService(
		cardRepo,
		batchRepo,
		auditRepo,
		txManager,
		service.BatchConfig{
			MaxBatchSize: cfg.BatchMaxSize,
			ChunkSize:    cfg.BatchChunkSize,
			WorkerCount:  cfg.BatchWorkerCount,
		},
		appLogger,
	)

	// 6. Initialize HTTP Handlers
	cardHandler := handler.NewCardHandler(cardService)
	cardholderHandler := handler.NewCardholderHandler(cardholderService)
	batchHandler := handler.NewBatchHandler(batchService)
	healthHandler := handler.NewHealthHandler(healthChecker)

	// 7. Compose Middlewares & Router
	router := handler.NewRouter(
		cardHandler,
		cardholderHandler,
		batchHandler,
		healthHandler,
		middleware.Recovery(appLogger),
		middleware.RequestID(),
		middleware.AuditLog(appLogger),
		middleware.Auth(tenantLookup),
		middleware.TenantEnforcement(),
		middleware.Idempotency(idempotencyStore),
	)

	// 8. Start HTTP Server
	server := &http.Server{
		Addr:         cfg.ServerAddr,
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  120 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		appLogger.Info("HTTP server listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	// 9. Graceful Shutdown
	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		appLogger.Error("server error encountered", "error", err)
		os.Exit(1)

	case sig := <-shutdownSignal:
		appLogger.Info("shutdown signal received, initiating graceful shutdown", "signal", sig.String())

		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			appLogger.Error("graceful server shutdown failed, forcing close", "error", err)
			_ = server.Close()
		}
	}

	appLogger.Info("card issuer API service stopped cleanly")
}

func seedDemoData(
	tenantRepo *memory.MemoryTenantRepository,
	chRepo *memory.MemoryCardholderRepository,
	cardRepo *memory.MemoryCardRepository,
	encryptor crypto.Encryptor,
	indexer crypto.BlindIndexer,
	log *loggerWrapper,
) {
	ctx := context.Background()

	// 1. Seed Tenant
	demoTenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	demoAPIKey := "dev-api-key-tenant-1"
	_, _ = tenantRepo.CreateWithID(demoTenantID, "NovoBank International (Demo Tenant)", demoAPIKey)

	// 2. Seed Cardholder
	demoCardholderID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	demoCH := &domain.Cardholder{
		ID:        demoCardholderID,
		TenantID:  demoTenantID,
		FirstName: "Elena",
		LastName:  "Rostova",
		Email:     "elena.rostova@example.com",
		Phone:     "+15551234567",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_ = chRepo.Create(ctx, demoCH)

	// 3. Seed Card in PENDING status
	cardPendingID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	cardPending := &domain.Card{
		ID:           cardPendingID,
		TenantID:     demoTenantID,
		CardholderID: demoCardholderID,
		Status:       domain.CardStatusPending,
		Version:      1,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	_ = cardRepo.Create(ctx, cardPending)

	// 4. Seed Card in ACTIVE status (with encrypted PAN & blind index)
	cardActiveID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	pan := domain.GeneratePAN()
	panBytes := []byte(pan.RawValue())
	defer crypto.Zeroize(panBytes)
	encPAN, _ := encryptor.Encrypt(panBytes, []byte(demoTenantID.String()))
	bIndex := indexer.ComputeIndex(panBytes)

	cardActive := &domain.Card{
		ID:             cardActiveID,
		TenantID:       demoTenantID,
		CardholderID:   demoCardholderID,
		PANEncrypted:   encPAN,
		PANBlindIndex:  bIndex,
		LastFourDigits: pan.LastFour(),
		ExpiryMonth:    12,
		ExpiryYear:     2029,
		Status:         domain.CardStatusActive,
		Version:        2,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	_ = cardRepo.Create(ctx, cardActive)

	fmt.Println("\n=========================================================================")
	fmt.Println("🚀 DEMO READY (Zero-dependency modular mode)")
	fmt.Println("-------------------------------------------------------------------------")
	fmt.Println("• Demo Tenant ID:   " + demoTenantID.String())
	fmt.Println("• Demo API Key:     " + demoAPIKey)
	fmt.Println("• Demo Cardholder:  " + demoCardholderID.String() + " (Elena Rostova)")
	fmt.Println("• Sample PENDING:   " + cardPendingID.String())
	fmt.Println("• Sample ACTIVE:    " + cardActiveID.String())
	fmt.Println("-------------------------------------------------------------------------")
	fmt.Println("Test with curl:")
	fmt.Printf("curl -H \"Authorization: Bearer %s\" http://localhost:8080/v1/cards/%s\n", demoAPIKey, cardActiveID.String())
	fmt.Println("=========================================================================")
}

type loggerWrapper = slog.Logger
