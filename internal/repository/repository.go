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
type DBTX = database.DBTX
