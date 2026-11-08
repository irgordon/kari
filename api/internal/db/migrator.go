package db

import (
	"bufio"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	migrationManifest = "migrations/manifest.txt"
	migrationPattern  = "migrations/*.sql"
	migrationLockID   = 126283425
)

//go:embed migrations/manifest.txt migrations/*.sql
var migrationFiles embed.FS

type Migration struct {
	Version  int64
	Name     string
	SQL      string
	Checksum string
}

type BackupProof struct {
	SchemaVersion int               `json:"schema_version"`
	Version       string            `json:"version"`
	Files         map[string]string `json:"files"`
}

func CheckMigrationBackup(path string, schema int) error {
	if path == "" {
		return errors.New("an existing database upgrade requires a verified control-plane backup; use kari-admin upgrade")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 65536 {
		return errors.New("invalid backup proof file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var proof BackupProof
	if err = json.Unmarshal(data, &proof); err != nil {
		return err
	}
	if proof.SchemaVersion != schema || proof.Files["database.dump"] == "" {
		return errors.New("backup does not match the installed database schema")
	}
	dump := filepath.Join(filepath.Dir(path), "database.dump")
	info, err = os.Lstat(dump)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("backup database dump is missing or unsafe")
	}
	file, err := os.Open(dump)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != proof.Files["database.dump"] {
		return errors.New("backup database dump checksum mismatch")
	}
	return nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	plan, err := LoadMigrationPlan()
	if err != nil {
		return err
	}
	return applyMigrationPlan(ctx, pool, plan)
}

func LoadMigrationPlan() ([]Migration, error) {
	names, err := readMigrationManifest()
	if err != nil {
		return nil, err
	}
	if err := validateManifestFiles(names); err != nil {
		return nil, err
	}
	return loadMigrations(names)
}

func applyMigrationPlan(ctx context.Context, pool *pgxpool.Pool, plan []Migration) error {
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer connection.Release()
	if err := lockMigrations(ctx, connection.Conn()); err != nil {
		return err
	}
	defer unlockMigrations(ctx, connection.Conn())
	return applyMigrationsAtomically(ctx, connection.Conn(), plan)
}

func applyMigrationsAtomically(ctx context.Context, connection *pgx.Conn, plan []Migration) error {
	transaction, err := connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer transaction.Rollback(ctx)
	if err := ensureMigrationTable(ctx, transaction); err != nil {
		return err
	}
	rows, err := transaction.Query(ctx, "SELECT version,name,checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		return err
	}
	if _, err = validateAppliedRows(rows, plan); err != nil {
		return err
	}
	if err := applyPendingMigrations(ctx, transaction, plan); err != nil {
		return err
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration transaction: %w", err)
	}
	return nil
}

// SchemaVersion rejects unknown, reordered, and modified history, including
// databases created by a newer executable. It never changes the database.
func SchemaVersion(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	plan, err := LoadMigrationPlan()
	if err != nil {
		return 0, err
	}
	var table *string
	if err = pool.QueryRow(ctx, "SELECT to_regclass('schema_migrations')::text").Scan(&table); err != nil {
		return 0, err
	}
	if table == nil {
		return 0, nil
	}
	rows, err := pool.Query(ctx, "SELECT version,name,checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		return 0, err
	}
	return validateAppliedRows(rows, plan)
}

func validateAppliedRows(rows pgx.Rows, plan []Migration) (int, error) {
	defer rows.Close()
	count := 0
	for rows.Next() {
		var version int64
		var name, checksum string
		if err := rows.Scan(&version, &name, &checksum); err != nil {
			return 0, err
		}
		if count >= len(plan) || version != plan[count].Version || name != plan[count].Name || checksum != plan[count].Checksum {
			return 0, fmt.Errorf("database migration %d is incompatible with this executable; restore matching application and database versions", version)
		}
		count++
	}
	return count, rows.Err()
}

func applyPendingMigrations(ctx context.Context, transaction pgx.Tx, plan []Migration) error {
	for _, migration := range plan {
		applied, err := migrationStatus(ctx, transaction, migration)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := applyMigration(ctx, transaction, migration); err != nil {
			return err
		}
	}
	return nil
}

func migrationStatus(ctx context.Context, transaction pgx.Tx, migration Migration) (bool, error) {
	var checksum string
	err := transaction.QueryRow(ctx,
		"SELECT checksum FROM schema_migrations WHERE version = $1",
		migration.Version,
	).Scan(&checksum)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read migration %s status: %w", migration.Name, err)
	}
	if checksum != migration.Checksum {
		return false, fmt.Errorf("applied migration %s checksum mismatch", migration.Name)
	}
	return true, nil
}

func applyMigration(ctx context.Context, transaction pgx.Tx, migration Migration) error {
	if _, err := transaction.Exec(ctx, migration.SQL); err != nil {
		return fmt.Errorf("apply migration %s: %w", migration.Name, err)
	}
	_, err := transaction.Exec(ctx,
		"INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)",
		migration.Version,
		migration.Name,
		migration.Checksum,
	)
	if err != nil {
		return fmt.Errorf("record migration %s: %w", migration.Name, err)
	}
	return nil
}

func ensureMigrationTable(ctx context.Context, transaction pgx.Tx) error {
	const statement = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`
	if _, err := transaction.Exec(ctx, statement); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	return nil
}

func lockMigrations(ctx context.Context, connection *pgx.Conn) error {
	if _, err := connection.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	return nil
}

func unlockMigrations(ctx context.Context, connection *pgx.Conn) {
	_, _ = connection.Exec(ctx, "SELECT pg_advisory_unlock($1)", migrationLockID)
}

func readMigrationManifest() ([]string, error) {
	manifest, err := migrationFiles.Open(migrationManifest)
	if err != nil {
		return nil, fmt.Errorf("open migration manifest: %w", err)
	}
	defer manifest.Close()
	return scanManifest(manifest), nil
}

func scanManifest(manifest fs.File) []string {
	var names []string
	scanner := bufio.NewScanner(manifest)
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func validateManifestFiles(names []string) error {
	actualPaths, err := fs.Glob(migrationFiles, migrationPattern)
	if err != nil {
		return fmt.Errorf("list migration files: %w", err)
	}
	actualNames := baseNames(actualPaths)
	if strings.Join(names, "\n") != strings.Join(actualNames, "\n") {
		return fmt.Errorf("migration manifest mismatch: expected %v, found %v", names, actualNames)
	}
	return validateMigrationSequence(names)
}

func validateMigrationSequence(names []string) error {
	for index, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}
		if version != int64(index+1) {
			return fmt.Errorf("migration sequence gap at %s", name)
		}
	}
	return nil
}

func baseNames(paths []string) []string {
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		names = append(names, filepath.Base(path))
	}
	sort.Strings(names)
	return names
}

func loadMigrations(names []string) ([]Migration, error) {
	plan := make([]Migration, 0, len(names))
	for _, name := range names {
		migration, err := loadMigration(name)
		if err != nil {
			return nil, err
		}
		plan = append(plan, migration)
	}
	return plan, nil
}

func loadMigration(name string) (Migration, error) {
	content, err := migrationFiles.ReadFile("migrations/" + name)
	if err != nil {
		return Migration{}, fmt.Errorf("read migration %s: %w", name, err)
	}
	version, err := migrationVersion(name)
	if err != nil {
		return Migration{}, err
	}
	return Migration{Version: version, Name: name, SQL: string(content), Checksum: checksum(content)}, nil
}

func migrationVersion(name string) (int64, error) {
	prefix, _, found := strings.Cut(name, "_")
	if !found {
		return 0, fmt.Errorf("invalid migration name: %s", name)
	}
	version, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid migration version %s: %w", name, err)
	}
	return version, nil
}

func checksum(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}
