package db

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migrate_mysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate runs all pending up migrations.
func Migrate(db *sql.DB) error {
	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migrate: source driver: %w", err)
	}

	dbDriver, err := migrate_mysql.WithInstance(db, &migrate_mysql.Config{})
	if err != nil {
		return fmt.Errorf("migrate: db driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "mysql", dbDriver)
	if err != nil {
		return fmt.Errorf("migrate: new instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate: up: %w", err)
	}

	return nil
}
