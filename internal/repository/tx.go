package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/novopayment/card-issuer-api/internal/domain"
	"github.com/novopayment/card-issuer-api/internal/platform/database"
)

// txManager implements TxManager using *sql.DB.
type txManager struct {
	db *sql.DB
}

// NewTxManager creates a new transaction manager.
func NewTxManager(db *sql.DB) TxManager {
	return &txManager{db: db}
}

// WithTransaction executes fn within a database transaction.
// It handles Begin, Commit, Rollback, and RLS tenant context setup.
//
// If fn returns an error, the transaction is rolled back.
// If fn panics, the transaction is rolled back and the panic is re-raised.
// RLS session variable is set at the start of each transaction as defense-in-depth.
func (m *txManager) WithTransaction(ctx context.Context, fn func(ctx context.Context, repos TxRepositories) error) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Set RLS session variable for defense-in-depth tenant isolation
	tenantID, ok := domain.TenantIDFromContext(ctx)
	if ok {
		if err := database.SetTenantContext(ctx, tx, tenantID.String()); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to set tenant context: %w", err)
		}
	}

	// Recover from panics to ensure rollback
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	// Create transaction-scoped repositories
	repos := &txRepos{tx: tx}

	if err := fn(ctx, repos); err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// txRepos provides transaction-scoped repository access.
// Each repository created here shares the same *sql.Tx.
type txRepos struct {
	tx *sql.Tx
}

func (r *txRepos) Cards() CardRepository {
	return NewCardRepository(r.tx)
}

func (r *txRepos) Cardholders() CardholderRepository {
	return NewCardholderRepository(r.tx)
}

func (r *txRepos) Audit() AuditRepository {
	return NewAuditRepository(r.tx)
}

func (r *txRepos) Batches() BatchRepository {
	return NewBatchRepository(r.tx)
}
