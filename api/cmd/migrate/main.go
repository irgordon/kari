package main

import (
	"context"
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
	databaseURL, err := requiredDatabaseURL()
	if err != nil {
		return err
	}
	pool, err := openDatabase(databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
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
