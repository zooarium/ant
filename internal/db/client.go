package db

import (
	"context"
	"fmt"
	"log/slog"

	"ant/ent"
	"ant/ent/migrate"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

// NewClient opens an ent client for the configured driver and runs
// auto-migration. Supported drivers: "sqlite3" (default) and "postgres".
func NewClient(driver, path, dsn string) (*ent.Client, error) {
	switch driver {
	case "", "sqlite3":
		return NewSQLiteClient(path)
	case "postgres":
		return NewPostgresClient(dsn)
	default:
		return nil, fmt.Errorf("unsupported database driver: %q", driver)
	}
}

func NewSQLiteClient(path string) (*ent.Client, error) {
	slog.Info("opening sqlite connection", "path", path)
	client, err := ent.Open(dialect.SQLite, fmt.Sprintf("file:%s?cache=shared&_fk=1", path))
	if err != nil {
		slog.Error("failed to open sqlite connection", "path", path, "error", err)
		return nil, fmt.Errorf("failed opening connection to sqlite: %w", err)
	}
	return migrateClient(client)
}

func NewPostgresClient(dsn string) (*ent.Client, error) {
	slog.Info("opening postgres connection")
	drv, err := entsql.Open(dialect.Postgres, dsn)
	if err != nil {
		slog.Error("failed to open postgres connection", "error", err)
		return nil, fmt.Errorf("failed opening connection to postgres: %w", err)
	}
	client := ent.NewClient(ent.Driver(drv))
	return migrateClient(client)
}

func migrateClient(client *ent.Client) (*ent.Client, error) {
	slog.Info("running auto migration")
	// Per-table autoincrement starting from 1 (no global unique ID ranges).
	if err := client.Schema.Create(
		context.Background(),
		migrate.WithDropColumn(true),
		migrate.WithDropIndex(true),
	); err != nil {
		slog.Error("failed to create schema resources", "error", err)
		if cerr := client.Close(); cerr != nil {
			slog.Error("failed to close client after schema creation failure", "error", cerr)
		}
		return nil, fmt.Errorf("failed creating schema resources: %w", err)
	}

	slog.Info("database initialization completed")
	return client, nil
}
