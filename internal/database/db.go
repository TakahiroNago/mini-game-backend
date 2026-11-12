// Package database owns the MySQL connection pool, transaction helpers and
// schema migrations. Domain packages depend on the small Executor interface
// so the same repository code runs inside or outside a transaction.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/takahironago/mini-game-backend/internal/config"
)

// Executor is the subset of *sql.DB / *sql.Tx used by repositories.
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

var (
	_ Executor = (*sql.DB)(nil)
	_ Executor = (*sql.Tx)(nil)
)

// DSN builds the go-sql-driver DSN used by the application pool.
// All timestamps are exchanged in UTC; the JST "reward day" is computed in
// application code, never by MySQL.
func DSN(c config.DBConfig) string { return dsn(c, false) }

// dsn is the shared builder. multiStatements is enabled only for the
// migration connection: golang-migrate needs it to run multi-statement SQL
// files, while the application pool keeps it off to shrink the blast radius
// of any future injection bug.
func dsn(c config.DBConfig, multiStatements bool) string {
	mc := mysql.NewConfig()
	mc.User = c.User
	mc.Passwd = c.Password
	mc.Net = "tcp"
	mc.Addr = fmt.Sprintf("%s:%d", c.Host, c.Port)
	mc.DBName = c.Name
	mc.ParseTime = true
	mc.Loc = time.UTC
	mc.Collation = "utf8mb4_0900_ai_ci"
	mc.MultiStatements = multiStatements
	mc.Timeout = 5 * time.Second
	mc.ReadTimeout = 10 * time.Second
	mc.WriteTimeout = 10 * time.Second
	mc.Params = map[string]string{
		"charset":   "utf8mb4",
		"time_zone": "'+00:00'",
	}
	return mc.FormatDSN()
}

// Open creates the application pool and verifies connectivity, retrying
// briefly so the API survives MySQL finishing its startup after us.
func Open(ctx context.Context, c config.DBConfig) (*sql.DB, error) {
	return open(ctx, c, false)
}

// openForMigrations returns a single-connection pool with multiStatements
// enabled, used only while applying schema migrations.
func openForMigrations(ctx context.Context, c config.DBConfig) (*sql.DB, error) {
	c.MaxOpenConns, c.MaxIdleConns = 1, 1
	return open(ctx, c, true)
}

func open(ctx context.Context, c config.DBConfig, multiStatements bool) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn(c, multiStatements))
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	db.SetMaxOpenConns(c.MaxOpenConns)
	db.SetMaxIdleConns(c.MaxIdleConns)
	db.SetConnMaxLifetime(c.ConnMaxLifetime)

	const attempts = 10
	for i := 1; i <= attempts; i++ {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = db.PingContext(pingCtx)
		cancel()
		if err == nil {
			return db, nil
		}
		if i == attempts || ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case <-time.After(time.Duration(i) * 500 * time.Millisecond):
		}
	}
	_ = db.Close()
	return nil, fmt.Errorf("ping mysql: %w", err)
}

// WithTx runs fn inside a transaction, committing on nil error and rolling
// back otherwise (including panics). Repositories receive the *sql.Tx as an
// Executor so business code decides the transaction boundary.
func WithTx(ctx context.Context, db *sql.DB, opts *sql.TxOptions, fn func(tx *sql.Tx) error) (err error) {
	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err = fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
		}
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// MySQL error numbers used to map constraint violations to domain errors.
const (
	ErrNoDuplicateEntry     uint16 = 1062 // ER_DUP_ENTRY
	ErrNoCheckViolated      uint16 = 3819 // ER_CHECK_CONSTRAINT_VIOLATED
	ErrNoLockWaitTimeout    uint16 = 1205 // ER_LOCK_WAIT_TIMEOUT
	ErrNoDeadlock           uint16 = 1213 // ER_LOCK_DEADLOCK
	ErrNoForeignKeyViolated uint16 = 1452 // ER_NO_REFERENCED_ROW_2
)

// IsMySQLError reports whether err is a MySQL server error with the given number.
func IsMySQLError(err error, number uint16) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == number
}

// IsDuplicateEntry reports a UNIQUE / PRIMARY KEY violation.
func IsDuplicateEntry(err error) bool { return IsMySQLError(err, ErrNoDuplicateEntry) }
