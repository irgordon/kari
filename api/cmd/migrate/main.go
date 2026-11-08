package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	karidb "github.com/irgordon/kari/api/internal/db"
)

func main() {
	if err := runMigration(); err != nil {
		fmt.Fprintln(os.Stderr, "migration failed:", err)
		os.Exit(1)
	}
}

func runMigration() error {
	check := flag.Bool("check", false, "validate schema compatibility without changing it")
	flag.Parse()
	databaseURL, err := requiredDatabaseURL()
	if err != nil {
		return err
	}
	pool, err := openDatabase(databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	version, err := karidb.SchemaVersion(context.Background(), pool)
	if err != nil {
		return err
	}
	plan, err := karidb.LoadMigrationPlan()
	if err != nil {
		return err
	}
	if *check {
		return nil
	}
	if version > 0 && version < len(plan) {
		if err = karidb.CheckMigrationBackup(os.Getenv("KARI_MIGRATION_BACKUP"), version); err != nil {
			return err
		}
	}
	return karidb.Migrate(context.Background(), pool)
}

func requiredDatabaseURL() (string, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return "", fmt.Errorf("DATABASE_URL is required")
	}
	return databaseURL, nil
}

func openDatabase(databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}
