// Package database provides database connectivity and management for the literature review service.
package database

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixir/literature-review-service/internal/config"
)

// TestDBTX_Interface verifies that DBTX interface is properly defined.
// This test ensures the interface can be used for both pool and transaction operations.
func TestDBTX_Interface(t *testing.T) {
	t.Run("DBTX interface is properly defined", func(t *testing.T) {
		// Verify the interface methods exist by checking the type
		// This is a compile-time check - if DBTX doesn't have these methods,
		// the code won't compile
		var _ DBTX = (*mockDBTX)(nil)
	})
}

// mockDBTX is a mock implementation of DBTX for interface verification.
type mockDBTX struct{}

func (m *mockDBTX) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (m *mockDBTX) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockDBTX) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return nil
}

func (m *mockDBTX) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return nil
}

// TestDatabaseConfig_DSN verifies config DSN generation works correctly.
func TestDatabaseConfig_DSN(t *testing.T) {
	t.Run("generates valid DSN with all parameters", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			Host:                   "localhost",
			Port:                   5432,
			User:                   "litreview",
			Password:               "secret",
			Name:                   "literature_review_service",
			SSLMode:                "disable",
			ConnectTimeout:         10 * time.Second,
			StatementCacheCapacity: 512,
		}

		dsn := cfg.DSN()

		assert.Contains(t, dsn, "postgres://")
		assert.Contains(t, dsn, "litreview")
		assert.Contains(t, dsn, "localhost:5432")
		assert.Contains(t, dsn, "literature_review_service")
		assert.Contains(t, dsn, "sslmode=disable")
		assert.Contains(t, dsn, "connect_timeout=10")
	})

	t.Run("escapes special characters in user and password", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "user@domain",
			Password: "pass/word",
			Name:     "testdb",
			SSLMode:  "require",
		}

		dsn := cfg.DSN()

		// URL encoding should escape @ and /
		assert.Contains(t, dsn, "user%40domain")
		assert.Contains(t, dsn, "pass%2Fword")
	})
}

// TestHealthCheckTimeout verifies the health check timeout constant is properly defined.
func TestHealthCheckTimeout(t *testing.T) {
	t.Run("health check timeout is 5 seconds", func(t *testing.T) {
		assert.Equal(t, 5*time.Second, HealthCheckTimeout)
	})
}

// TestHealthStatus_Fields verifies HealthStatus struct construction and JSON serialization.
func TestHealthStatus_Fields(t *testing.T) {
	t.Run("all fields populated", func(t *testing.T) {
		hs := HealthStatus{
			Status:            "unhealthy",
			Error:             "connection refused",
			TotalConns:        10,
			AcquiredConns:     3,
			IdleConns:         7,
			ConstructingConns: 0,
			MaxConns:          50,
		}

		assert.Equal(t, "unhealthy", hs.Status)
		assert.Equal(t, "connection refused", hs.Error)
		assert.Equal(t, int32(10), hs.TotalConns)
		assert.Equal(t, int32(3), hs.AcquiredConns)
		assert.Equal(t, int32(7), hs.IdleConns)
		assert.Equal(t, int32(0), hs.ConstructingConns)
		assert.Equal(t, int32(50), hs.MaxConns)

		// Verify JSON includes error field when populated
		data, err := json.Marshal(hs)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"error":"connection refused"`)
	})

	t.Run("empty error field is omitted from JSON", func(t *testing.T) {
		hs := HealthStatus{
			Status:   "healthy",
			Error:    "",
			MaxConns: 50,
		}

		data, err := json.Marshal(hs)
		require.NoError(t, err)

		assert.NotContains(t, string(data), `"error"`)
		assert.Contains(t, string(data), `"status":"healthy"`)
	})

	t.Run("empty error field round-trips through JSON", func(t *testing.T) {
		original := HealthStatus{
			Status:   "healthy",
			MaxConns: 25,
		}

		data, err := json.Marshal(original)
		require.NoError(t, err)

		var decoded HealthStatus
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, original.Status, decoded.Status)
		assert.Equal(t, "", decoded.Error)
		assert.Equal(t, original.MaxConns, decoded.MaxConns)
	})
}

// TestDatabaseConfig_DSN_EdgeCases tests DSN generation with edge-case inputs.
func TestDatabaseConfig_DSN_EdgeCases(t *testing.T) {
	t.Run("special characters in password are URL-encoded", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "admin",
			Password: "p@ss:w0rd!#$%^&*()",
			Name:     "testdb",
			SSLMode:  "disable",
		}

		dsn := cfg.DSN()

		// The password must be URL-encoded in the DSN
		assert.Contains(t, dsn, "postgres://")
		// Verify the raw special characters are NOT present unescaped
		assert.NotContains(t, dsn, "p@ss:w0rd")
		// Verify the DSN is parseable by pgxpool
		_, err := parseTestDSN(dsn)
		assert.NoError(t, err)
	})

	t.Run("empty password produces valid DSN", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "admin",
			Password: "",
			Name:     "testdb",
			SSLMode:  "disable",
		}

		dsn := cfg.DSN()

		// Format: postgres://admin:@localhost:5432/testdb?...
		assert.Contains(t, dsn, "admin:@localhost")
		assert.Contains(t, dsn, "postgres://")
		assert.Contains(t, dsn, "testdb")
	})

	t.Run("non-default port is included in DSN", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			Host:     "db.example.com",
			Port:     15432,
			User:     "user",
			Password: "pass",
			Name:     "mydb",
			SSLMode:  "require",
		}

		dsn := cfg.DSN()

		assert.Contains(t, dsn, "db.example.com:15432")
		assert.Contains(t, dsn, "sslmode=require")
	})

	t.Run("connect timeout zero omits parameter", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			Host:           "localhost",
			Port:           5432,
			User:           "user",
			Password:       "pass",
			Name:           "testdb",
			SSLMode:        "disable",
			ConnectTimeout: 0,
		}

		dsn := cfg.DSN()

		assert.NotContains(t, dsn, "connect_timeout")
	})
}

// parseTestDSN is a helper that validates a DSN string is parseable by pgxpool.
func parseTestDSN(dsn string) (interface{}, error) {
	// Use pgxpool.ParseConfig to validate the DSN is well-formed.
	return pgxpool.ParseConfig(dsn)
}

// TestNew_InvalidConfig tests that New returns an error for unreachable configurations.
func TestNew_InvalidConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := zerolog.Nop()

	t.Run("unreachable host returns error", func(t *testing.T) {
		// 192.0.2.1 is TEST-NET-1 (RFC 5737), guaranteed unroutable.
		cfg := &config.DatabaseConfig{
			Host:              "192.0.2.1",
			Port:              5432,
			Name:              "testdb",
			User:              "user",
			Password:          "pass",
			SSLMode:           "disable",
			MaxConns:          5,
			MinConns:          1,
			MaxConnLifetime:   time.Hour,
			MaxConnIdleTime:   30 * time.Minute,
			HealthCheckPeriod: 30 * time.Second,
			ConnectTimeout:    2 * time.Second,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		db, err := New(ctx, cfg, logger)
		require.Error(t, err)
		assert.Nil(t, db)
	})
}

// TestNew_ConnectionError is an integration test that expects error on bad host.
func TestNew_ConnectionError(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	logger := zerolog.Nop()

	t.Run("connection with invalid host fails", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			Host:              "invalid-host-that-does-not-exist",
			Port:              5432,
			Name:              "nonexistent_db",
			User:              "nobody",
			Password:          "wrong",
			SSLMode:           "disable",
			MaxConns:          10,
			MinConns:          2,
			MaxConnLifetime:   time.Hour,
			MaxConnIdleTime:   30 * time.Minute,
			HealthCheckPeriod: 30 * time.Second,
			ConnectTimeout:    2 * time.Second,
		}

		ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		db, err := New(ctxTimeout, cfg, logger)
		assert.Error(t, err)
		assert.Nil(t, db)
	})

	t.Run("connection with invalid port fails", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			Host:              "localhost",
			Port:              59999, // Invalid port
			Name:              "testdb",
			User:              "test",
			Password:          "test",
			SSLMode:           "disable",
			MaxConns:          10,
			MinConns:          2,
			MaxConnLifetime:   time.Hour,
			MaxConnIdleTime:   30 * time.Minute,
			HealthCheckPeriod: 30 * time.Second,
			ConnectTimeout:    2 * time.Second,
		}

		ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		db, err := New(ctxTimeout, cfg, logger)
		assert.Error(t, err)
		assert.Nil(t, db)
	})
}

// TestDB_Methods tests the DB struct methods with a real database connection.
// These are integration tests that require a running PostgreSQL instance.
func TestDB_Methods(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	t.Run("Pool returns underlying pool", func(t *testing.T) {
		pool := db.Pool()
		assert.NotNil(t, pool)
	})

	t.Run("Ping verifies connection", func(t *testing.T) {
		err := db.Ping(ctx)
		assert.NoError(t, err)
	})

	t.Run("Stats returns pool statistics", func(t *testing.T) {
		stats := db.Stats()
		require.NotNil(t, stats)
		assert.GreaterOrEqual(t, stats.MaxConns(), int32(1))
	})

	t.Run("Health returns health information", func(t *testing.T) {
		health := db.Health(ctx)
		assert.Equal(t, "healthy", health.Status)
		assert.GreaterOrEqual(t, health.TotalConns, int32(0))
		assert.GreaterOrEqual(t, health.MaxConns, int32(1))
	})
}

// TestDB_WithTransaction tests the transaction methods.
func TestDB_WithTransaction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	t.Run("successful transaction commits", func(t *testing.T) {
		var result int
		err := db.WithTransaction(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT 42").Scan(&result)
		})
		require.NoError(t, err)
		assert.Equal(t, 42, result)
	})

	t.Run("failed transaction rolls back", func(t *testing.T) {
		expectedErr := errors.New("intentional failure")
		err := db.WithTransaction(ctx, func(tx pgx.Tx) error {
			return expectedErr
		})
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("panic in transaction rolls back and re-panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = db.WithTransaction(ctx, func(tx pgx.Tx) error {
				panic("intentional panic")
			})
		})
	})
}

// TestDB_WithSerializableTransaction tests serializable transactions.
func TestDB_WithSerializableTransaction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	t.Run("serializable transaction executes", func(t *testing.T) {
		var result int
		err := db.WithSerializableTransaction(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT 1").Scan(&result)
		})
		require.NoError(t, err)
		assert.Equal(t, 1, result)
	})
}

// TestDB_WithRepeatableReadTransaction tests repeatable read transactions.
func TestDB_WithRepeatableReadTransaction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	t.Run("repeatable read transaction executes", func(t *testing.T) {
		var result int
		err := db.WithRepeatableReadTransaction(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT 1").Scan(&result)
		})
		require.NoError(t, err)
		assert.Equal(t, 1, result)
	})
}

// TestDB_DBTX tests that DB implements the DBTX interface.
func TestDB_DBTX(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	t.Run("DB implements DBTX interface", func(t *testing.T) {
		// Verify at compile time that *DB implements DBTX
		var _ DBTX = db
	})

	t.Run("Exec works through DBTX", func(t *testing.T) {
		var dbtx DBTX = db
		tag, err := dbtx.Exec(ctx, "SELECT 1")
		require.NoError(t, err)
		assert.NotNil(t, tag)
	})

	t.Run("QueryRow works through DBTX", func(t *testing.T) {
		var dbtx DBTX = db
		row := dbtx.QueryRow(ctx, "SELECT 42")
		var result int
		err := row.Scan(&result)
		require.NoError(t, err)
		assert.Equal(t, 42, result)
	})

	t.Run("Query works through DBTX", func(t *testing.T) {
		var dbtx DBTX = db
		rows, err := dbtx.Query(ctx, "SELECT generate_series(1, 3)")
		require.NoError(t, err)
		defer rows.Close()

		var results []int
		for rows.Next() {
			var val int
			err := rows.Scan(&val)
			require.NoError(t, err)
			results = append(results, val)
		}
		assert.Equal(t, []int{1, 2, 3}, results)
	})

	t.Run("SendBatch works through DBTX", func(t *testing.T) {
		var dbtx DBTX = db
		batch := &pgx.Batch{}
		batch.Queue("SELECT 1")
		batch.Queue("SELECT 2")

		br := dbtx.SendBatch(ctx, batch)
		defer br.Close()

		var val1, val2 int
		err := br.QueryRow().Scan(&val1)
		require.NoError(t, err)
		err = br.QueryRow().Scan(&val2)
		require.NoError(t, err)

		assert.Equal(t, 1, val1)
		assert.Equal(t, 2, val2)
	})
}

// TestDB_Close tests the Close method.
func TestDB_Close(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)

	t.Run("close successfully", func(t *testing.T) {
		assert.NotPanics(t, func() {
			db.Close()
		})
	})

	t.Run("close nil pool does not panic", func(t *testing.T) {
		nilDB := &DB{}
		assert.NotPanics(t, func() {
			nilDB.Close()
		})
	})
}

// TestDB_WithReadOnlyTransaction tests read-only transactions.
func TestDB_WithReadOnlyTransaction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	t.Run("read-only transaction executes SELECT", func(t *testing.T) {
		var result int
		err := db.WithReadOnlyTransaction(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT 42").Scan(&result)
		})
		require.NoError(t, err)
		assert.Equal(t, 42, result)
	})

	t.Run("read-only transaction cannot perform writes", func(t *testing.T) {
		// Create a temp table first outside the read-only transaction
		_, err := db.Exec(ctx, "CREATE TABLE IF NOT EXISTS test_readonly (id int)")
		require.NoError(t, err)
		defer func() {
			_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS test_readonly")
		}()

		// Attempt to INSERT in read-only transaction should fail
		err = db.WithReadOnlyTransaction(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "INSERT INTO test_readonly (id) VALUES (1)")
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read-only")
	})

	t.Run("read-only transaction rollback on error", func(t *testing.T) {
		expectedErr := errors.New("read-only error")
		err := db.WithReadOnlyTransaction(ctx, func(tx pgx.Tx) error {
			return expectedErr
		})
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}

// TestDB_AcquireAdvisoryLock tests session-level advisory locks.
func TestDB_AcquireAdvisoryLock(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	t.Run("acquire lock succeeds on first attempt", func(t *testing.T) {
		lockKey := int64(123456789)

		acquired, err := db.AcquireAdvisoryLock(ctx, lockKey)
		require.NoError(t, err)
		assert.True(t, acquired, "should acquire lock on first attempt")

		// Release the lock
		err = db.ReleaseAdvisoryLock(ctx, lockKey)
		require.NoError(t, err)
	})

	t.Run("lock can be re-acquired after release", func(t *testing.T) {
		lockKey := int64(987654321)

		// First acquisition
		acquired1, err := db.AcquireAdvisoryLock(ctx, lockKey)
		require.NoError(t, err)
		assert.True(t, acquired1)

		// Release the lock
		err = db.ReleaseAdvisoryLock(ctx, lockKey)
		require.NoError(t, err)

		// Should be able to acquire again
		acquired2, err := db.AcquireAdvisoryLock(ctx, lockKey)
		require.NoError(t, err)
		assert.True(t, acquired2)

		// Final release
		err = db.ReleaseAdvisoryLock(ctx, lockKey)
		require.NoError(t, err)
	})

	t.Run("release lock succeeds", func(t *testing.T) {
		lockKey := int64(111222333)

		acquired, err := db.AcquireAdvisoryLock(ctx, lockKey)
		require.NoError(t, err)
		assert.True(t, acquired)

		err = db.ReleaseAdvisoryLock(ctx, lockKey)
		require.NoError(t, err)
	})

	t.Run("release non-held lock succeeds silently", func(t *testing.T) {
		// PostgreSQL pg_advisory_unlock returns false but doesn't error
		// when releasing a lock not held by this session
		lockKey := int64(444555666)
		err := db.ReleaseAdvisoryLock(ctx, lockKey)
		require.NoError(t, err)
	})

	t.Run("different lock keys are independent", func(t *testing.T) {
		lockKey1 := int64(100)
		lockKey2 := int64(200)

		acquired1, err := db.AcquireAdvisoryLock(ctx, lockKey1)
		require.NoError(t, err)
		assert.True(t, acquired1)

		acquired2, err := db.AcquireAdvisoryLock(ctx, lockKey2)
		require.NoError(t, err)
		assert.True(t, acquired2)

		// Release both
		err = db.ReleaseAdvisoryLock(ctx, lockKey1)
		require.NoError(t, err)
		err = db.ReleaseAdvisoryLock(ctx, lockKey2)
		require.NoError(t, err)
	})
}

// TestDB_AcquireAdvisoryLockTx tests transaction-scoped advisory locks.
func TestDB_AcquireAdvisoryLockTx(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	t.Run("acquire tx lock succeeds", func(t *testing.T) {
		lockKey := int64(777888999)

		err := db.WithTransaction(ctx, func(tx pgx.Tx) error {
			err := db.AcquireAdvisoryLockTx(ctx, tx, lockKey)
			require.NoError(t, err)

			// Lock is held, verify we can do work
			var result int
			err = tx.QueryRow(ctx, "SELECT 1").Scan(&result)
			require.NoError(t, err)
			assert.Equal(t, 1, result)

			return nil
		})
		require.NoError(t, err)
		// Lock is automatically released when transaction commits
	})

	t.Run("tx lock released on rollback", func(t *testing.T) {
		lockKey := int64(111333555)
		expectedErr := errors.New("rollback error")

		err := db.WithTransaction(ctx, func(tx pgx.Tx) error {
			err := db.AcquireAdvisoryLockTx(ctx, tx, lockKey)
			require.NoError(t, err)
			return expectedErr
		})
		assert.Equal(t, expectedErr, err)
		// Lock should be released after rollback
	})

	t.Run("tx lock blocks until released", func(t *testing.T) {
		// This test verifies that pg_advisory_xact_lock blocks
		// We can't easily test blocking behavior in a single-threaded test,
		// but we can verify the lock is acquired successfully
		lockKey := int64(222444666)

		err := db.WithTransaction(ctx, func(tx pgx.Tx) error {
			return db.AcquireAdvisoryLockTx(ctx, tx, lockKey)
		})
		require.NoError(t, err)
	})
}

// TestDB_TryAcquireAdvisoryLockTx tests non-blocking transaction-scoped advisory locks.
func TestDB_TryAcquireAdvisoryLockTx(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	t.Run("try acquire tx lock succeeds when available", func(t *testing.T) {
		lockKey := int64(333666999)

		err := db.WithTransaction(ctx, func(tx pgx.Tx) error {
			acquired, err := db.TryAcquireAdvisoryLockTx(ctx, tx, lockKey)
			require.NoError(t, err)
			assert.True(t, acquired, "should acquire lock when available")
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("try acquire returns true within same transaction", func(t *testing.T) {
		lockKey := int64(444777000)

		err := db.WithTransaction(ctx, func(tx pgx.Tx) error {
			// First acquisition
			acquired1, err := db.TryAcquireAdvisoryLockTx(ctx, tx, lockKey)
			require.NoError(t, err)
			assert.True(t, acquired1)

			// Same tx can "re-acquire" (it already holds the lock)
			acquired2, err := db.TryAcquireAdvisoryLockTx(ctx, tx, lockKey)
			require.NoError(t, err)
			assert.True(t, acquired2)

			return nil
		})
		require.NoError(t, err)
	})

	t.Run("different lock keys independent in same tx", func(t *testing.T) {
		lockKey1 := int64(555000111)
		lockKey2 := int64(555000222)

		err := db.WithTransaction(ctx, func(tx pgx.Tx) error {
			acquired1, err := db.TryAcquireAdvisoryLockTx(ctx, tx, lockKey1)
			require.NoError(t, err)
			assert.True(t, acquired1)

			acquired2, err := db.TryAcquireAdvisoryLockTx(ctx, tx, lockKey2)
			require.NoError(t, err)
			assert.True(t, acquired2)

			return nil
		})
		require.NoError(t, err)
	})
}

// TestDB_Health_EdgeCases tests edge cases for health checks.
func TestDB_Health_EdgeCases(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer db.Close()

	t.Run("health check with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		health := db.Health(ctx)
		// With cancelled context, ping should fail
		assert.Equal(t, "unhealthy", health.Status)
		assert.NotEmpty(t, health.Error)
	})

	t.Run("health check returns pool stats", func(t *testing.T) {
		ctx := context.Background()
		health := db.Health(ctx)

		assert.Equal(t, "healthy", health.Status)
		assert.GreaterOrEqual(t, health.MaxConns, int32(1))
		assert.GreaterOrEqual(t, health.TotalConns, int32(0))
	})
}

// setupTestDB creates a test database connection.
func setupTestDB(t *testing.T) *DB {
	t.Helper()

	ctx := context.Background()
	logger := zerolog.Nop()

	// Try test port first (docker-compose.test.yml), then dev port
	ports := []int{5433, 5432}
	var db *DB
	var lastErr error

	for _, port := range ports {
		cfg := &config.DatabaseConfig{
			Host:              "localhost",
			Port:              port,
			Name:              "literature_review_test",
			User:              "litreview_test",
			Password:          "testpassword",
			SSLMode:           "disable",
			MaxConns:          5,
			MinConns:          1,
			MaxConnLifetime:   time.Hour,
			MaxConnIdleTime:   30 * time.Minute,
			HealthCheckPeriod: 30 * time.Second,
			ConnectTimeout:    5 * time.Second,
		}

		var err error
		db, err = New(ctx, cfg, logger)
		if err == nil {
			return db
		}
		lastErr = err
	}

	t.Skipf("Skipping integration test: cannot connect to database: %v", lastErr)
	return nil
}
