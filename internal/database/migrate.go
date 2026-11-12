package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/takahironago/mini-game-backend/internal/config"
	"github.com/takahironago/mini-game-backend/migrations"
)

// NewMigrator opens a dedicated migration connection (multiStatements on)
// and builds a golang-migrate instance over the embedded SQL files. The
// returned close function releases the connection.
func NewMigrator(ctx context.Context, c config.DBConfig) (*migrate.Migrate, func() error, error) {
	db, err := openForMigrations(ctx, c)
	if err != nil {
		return nil, nil, err
	}
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("migration source: %w", err)
	}
	driver, err := migratemysql.WithInstance(db, &migratemysql.Config{})
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("migration driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "mysql", driver)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("migrator: %w", err)
	}
	return m, db.Close, nil
}

// MigrateUp applies all pending migrations. ErrNoChange is not an error.
func MigrateUp(ctx context.Context, c config.DBConfig) error {
	m, closeFn, err := NewMigrator(ctx, c)
	if err != nil {
		return err
	}
	defer closeFn()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
