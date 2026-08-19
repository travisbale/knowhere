package db

import (
	"embed"
	"fmt"
	"log/slog"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the "pgx5" driver golang-migrate opens from the URL scheme
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// migrateURL points a postgres URL at the pgx driver, which golang-migrate registers under
// its own scheme. Callers pass the same URL they open a pool with.
func migrateURL(databaseURL string) string {
	for _, scheme := range []string{"postgresql://", "postgres://"} {
		if rest, ok := strings.CutPrefix(databaseURL, scheme); ok {
			return "pgx5://" + rest
		}
	}
	return databaseURL
}

// newMigrator creates a migrate instance for embedded SQL migrations.
func newMigrator(fs embed.FS, dir string, databaseURL string) (*migrate.Migrate, error) {
	sourceDriver, err := iofs.New(fs, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to create iofs driver: %w", err)
	}

	migrator, err := migrate.NewWithSourceInstance("iofs", sourceDriver, migrateURL(databaseURL))
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
