// Package repository provides data access interfaces and implementations
// for the Literature Review Service.
//
// # Overview
//
// This package defines repository interfaces and their PostgreSQL implementations
// following the repository pattern to abstract data persistence from business logic.
//
// # Repository Interfaces
//
// The package provides the following repository interfaces:
//
//   - ReviewRepository: Manages literature review request lifecycle and state
//   - PaperRepository: Manages academic paper persistence and deduplication
//   - KeywordRepository: Manages keyword tracking and search operations
//
// # Thread Safety
//
// All repository implementations are safe for concurrent use by multiple goroutines.
// The underlying pgxpool handles connection pooling and synchronization.
//
// # Error Handling
//
// All methods return domain-specific errors from the domain package.
// Wrap database errors with context using fmt.Errorf with %w verb.
// Common errors include:
//
//   - domain.ErrNotFound: Resource does not exist
//   - domain.ErrAlreadyExists: Unique constraint violation
//   - domain.ErrInvalidInput: Invalid parameters provided
//
// # Transactions
//
// Use the DBTX interface to support both pool and transaction contexts.
// Pass transaction from database.DB.WithTransaction for atomic operations.
//
// # Usage Pattern
//
// Repositories are typically created at application startup and passed to services:
//
//	db, _ := database.New(ctx, cfg, logger)
//	reviewRepo := repository.NewPgReviewRepository(db)
//	paperRepo := repository.NewPgPaperRepository(db)
//	keywordRepo := repository.NewPgKeywordRepository(db)
package repository

import (
	"github.com/helixir/literature-review-service/internal/database"
)

// DBTX is the database interface supporting both pool and transaction contexts.
// This allows repositories to work with both direct pool connections and transactions.
//
// # Constructor Pattern
//
// Repository implementations follow a constructor pattern that accepts DBTX:
//
//	type PgReviewRepository struct {
//	    db DBTX
//	}
//
//	func NewPgReviewRepository(db DBTX) *PgReviewRepository {
//	    return &PgReviewRepository{db: db}
//	}
//
// This design enables:
//   - Direct usage with a connection pool for standard operations
//   - Transaction support by passing a transaction (pgx.Tx) instead
//   - Easy testing with mock implementations of DBTX
//
// # Transaction Usage Example
//
//	err := db.WithTransaction(ctx, func(tx pgx.Tx) error {
//	    // Create a transactional repository instance
//	    txRepo := repository.NewPgReviewRepository(tx)
//	    // All operations within this function use the same transaction
//	    return txRepo.Create(ctx, review)
//	})
type DBTX = database.DBTX

// Filter pagination defaults and limits.
const (
	defaultFilterLimit = 100
	maxFilterLimit     = 1000
)

// applyPaginationDefaults normalizes limit and offset values for filter queries.
// It clamps limit to [1, maxFilterLimit] and ensures offset >= 0.
func applyPaginationDefaults(limit, offset *int) {
	if *limit <= 0 {
		*limit = defaultFilterLimit
	}
	if *limit > maxFilterLimit {
		*limit = maxFilterLimit
	}
	if *offset < 0 {
		*offset = 0
	}
}
