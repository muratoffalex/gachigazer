package database

import (
	"database/sql"
	"embed"
	"fmt"

	_ "modernc.org/sqlite"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func RunMigrations(db *sql.DB) error {
	goose.SetBaseFS(migrationsFS)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("failed to set dialect: %w", err)
	}

	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}
