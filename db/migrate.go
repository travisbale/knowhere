package db

import (
	"embed"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// newMigrator creates a migrate instance for embedded SQL migrations.
// Callers must import the appropriate database driver for their URL scheme.
func newMigrator(fs embed.FS, dir string, databaseURL string) (*migrate.Migrate, error) {
	sourceDriver, err := iofs.New(fs, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to create iofs driver: %w", err)
	}

	migrator, err := migrate.NewWithSourceInstance("iofs", sourceDriver, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrate instance: %w", err)
	}

	return migrator, nil
}

// MigrateUp applies all pending migrations
func MigrateUp(fs embed.FS, dir string, databaseURL string) error {
	migrator, err := newMigrator(fs, dir, databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		if srcErr, dbErr := migrator.Close(); srcErr != nil || dbErr != nil {
			slog.Error("failed to close migrator", "source", srcErr, "database", dbErr)
		}
	}()

	if err := migrator.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}
	return nil
}

// MigrateDown rolls back the last migration
func MigrateDown(fs embed.FS, dir string, databaseURL string) error {
	migrator, err := newMigrator(fs, dir, databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		if srcErr, dbErr := migrator.Close(); srcErr != nil || dbErr != nil {
			slog.Error("failed to close migrator", "source", srcErr, "database", dbErr)
		}
	}()

	if err := migrator.Steps(-1); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to rollback migration: %w", err)
	}
	return nil
}

// MigrateVersion returns the current migration version and dirty state
func MigrateVersion(fs embed.FS, dir string, databaseURL string) (version uint, dirty bool, err error) {
	migrator, err := newMigrator(fs, dir, databaseURL)
	if err != nil {
		return 0, false, err
	}
	defer func() {
		if srcErr, dbErr := migrator.Close(); srcErr != nil || dbErr != nil {
			slog.Error("failed to close migrator", "source", srcErr, "database", dbErr)
		}
	}()

	version, dirty, err = migrator.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return 0, false, fmt.Errorf("failed to get migration version: %w", err)
	}

	return version, dirty, nil
}
