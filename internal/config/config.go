// Package config provides environment-based configuration for the Card Issuer API.
// All sensitive values (keys, DSN) are loaded from environment variables.
package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration.
type Config struct {
	// Server
	ServerAddr      string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration

	// Storage & Database
	StorageDriver   string // "postgres" or "memory"
	DatabaseDSN     string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration

	// Cryptography — two separate keys (key separation principle)
	// EncryptionKey is used for AES-256-GCM encryption of PAN data
	EncryptionKey []byte
	// BlindIndexKey is used for HMAC-SHA256 blind indexing of PAN data
	BlindIndexKey []byte

	// Batch Processing
	BatchMaxSize     int
	BatchChunkSize   int
	BatchWorkerCount int

	// Logging
	LogLevel  string
	LogFormat string // "json" or "text"
}

// Load reads configuration from environment variables with sensible defaults.
// If STORAGE_DRIVER is "memory" or DATABASE_DSN is omitted, it defaults to modular in-memory storage.
func Load() (*Config, error) {
	storageDriver := strings.ToLower(getEnv("STORAGE_DRIVER", ""))
	dbDSN := getEnv("DATABASE_DSN", "")

	if storageDriver == "" {
		if dbDSN != "" {
			storageDriver = "postgres"
		} else {
			storageDriver = "memory"
		}
	}

	cfg := &Config{
		ServerAddr:       getEnv("SERVER_ADDR", ":8080"),
		ReadTimeout:      getDurationEnv("READ_TIMEOUT", 15*time.Second),
		WriteTimeout:     getDurationEnv("WRITE_TIMEOUT", 15*time.Second),
		ShutdownTimeout:  getDurationEnv("SHUTDOWN_TIMEOUT", 30*time.Second),
		StorageDriver:    storageDriver,
		DatabaseDSN:      dbDSN,
		MaxOpenConns:     getIntEnv("DB_MAX_OPEN_CONNS", 25),
		MaxIdleConns:     getIntEnv("DB_MAX_IDLE_CONNS", 10),
		ConnMaxLifetime:  getDurationEnv("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		BatchMaxSize:     getIntEnv("BATCH_MAX_SIZE", 1000),
		BatchChunkSize:   getIntEnv("BATCH_CHUNK_SIZE", 50),
		BatchWorkerCount: getIntEnv("BATCH_WORKER_COUNT", 4),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		LogFormat:        getEnv("LOG_FORMAT", "json"),
	}

	// Parse encryption key (hex-encoded, 32 bytes = 64 hex chars)
	encKeyHex := getEnv("ENCRYPTION_KEY", "")
	if encKeyHex == "" {
		if cfg.StorageDriver == "memory" {
			// Provide deterministic development key for instant zero-dependency testing
			encKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		} else {
			return nil, fmt.Errorf("ENCRYPTION_KEY is required")
		}
	}
	encKey, err := hex.DecodeString(encKeyHex)
	if err != nil {
		return nil, fmt.Errorf("ENCRYPTION_KEY must be valid hex: %w", err)
	}
	if len(encKey) != 32 {
		return nil, fmt.Errorf("ENCRYPTION_KEY must be 32 bytes (64 hex chars), got %d bytes", len(encKey))
	}
	cfg.EncryptionKey = encKey

	// Parse blind index key (hex-encoded, >= 32 bytes)
	biKeyHex := getEnv("BLIND_INDEX_KEY", "")
	if biKeyHex == "" {
		if cfg.StorageDriver == "memory" {
			// Provide deterministic development key for instant zero-dependency testing
			biKeyHex = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
		} else {
			return nil, fmt.Errorf("BLIND_INDEX_KEY is required")
		}
	}
	biKey, err := hex.DecodeString(biKeyHex)
	if err != nil {
		return nil, fmt.Errorf("BLIND_INDEX_KEY must be valid hex: %w", err)
	}
	if len(biKey) < 32 {
		return nil, fmt.Errorf("BLIND_INDEX_KEY must be at least 32 bytes (64 hex chars), got %d bytes", len(biKey))
	}
	cfg.BlindIndexKey = biKey

	// Validate database DSN if postgres driver is chosen
	if cfg.StorageDriver == "postgres" && cfg.DatabaseDSN == "" {
		return nil, fmt.Errorf("DATABASE_DSN is required when STORAGE_DRIVER is postgres")
	}

	return cfg, nil
}

// LoadWithDefaults returns a config suitable for standalone testing with memory driver.
func LoadWithDefaults() *Config {
	encKey := make([]byte, 32)
	biKey := make([]byte, 32)
	for i := range encKey {
		encKey[i] = byte(i + 1)
	}
	for i := range biKey {
		biKey[i] = byte(i + 33)
	}

	return &Config{
		ServerAddr:       ":8080",
		ReadTimeout:      15 * time.Second,
		WriteTimeout:     15 * time.Second,
		ShutdownTimeout:  30 * time.Second,
		StorageDriver:    "memory",
		DatabaseDSN:      "",
		MaxOpenConns:     10,
		MaxIdleConns:     5,
		ConnMaxLifetime:  5 * time.Minute,
		EncryptionKey:    encKey,
		BlindIndexKey:    biKey,
		BatchMaxSize:     1000,
		BatchChunkSize:   50,
		BatchWorkerCount: 4,
		LogLevel:         "debug",
		LogFormat:        "text",
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getIntEnv(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

func getDurationEnv(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}
