package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/irgordon/kari/api/internal/core/utils"
	"github.com/irgordon/kari/api/internal/db/postgres"
	"github.com/irgordon/kari/api/internal/operations"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "kari-admin:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("commands: init, bundle, install, start, stop, restart, backup, upgrade, restore, recover, setup-token")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	root := flags.String("root", "/opt/kari", "installation directory")
	bundle := flags.String("bundle", "", "release directory")
	checksum := flags.String("manifest-sha256", "", "trusted manifest checksum")
	version := flags.String("version", "", "release version")
	arch := flags.String("arch", "", "release architecture")
	tree := flags.String("tree", "", "source Git tree")
	publicURL := flags.String("public-url", "", "panel origin")
	cert := flags.String("tls-cert", "", "existing TLS certificate")
	key := flags.String("tls-key", "", "existing TLS private key")
	output := flags.String("output", ".env", "new environment file")
	development := flags.Bool("development", false, "local development environment")
	backup := flags.String("backup", "", "control-plane backup directory")
	email := flags.String("email", "", "administrator account")
	passwordFile := flags.String("password-file", "", "root-owned 0600 file containing the replacement password")
	resetMFA := flags.Bool("reset-mfa", false, "explicitly remove a lost authenticator")
	confirm := flags.String("confirm", "", "confirm the restore version or recovered email")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	host := operations.Host{Root: *root}
	switch args[0] {
	case "init":
		return operations.InitEnvironment(*output, *publicURL, *development)
	case "bundle":
		return operations.CreateManifest(*bundle, *version, *arch, *tree)
	case "recover-db":
		password, err := io.ReadAll(io.LimitReader(os.Stdin, 1025))
		if err != nil {
			return err
		}
		return recoverDatabase(ctx, *email, strings.TrimSuffix(string(password), "\n"), *resetMFA)
	}
	if err := host.CheckRoot(); err != nil {
		return err
	}
	switch args[0] {
	case "install":
		return host.Install(ctx, *bundle, *checksum, *publicURL, *cert, *key)
	case "start":
		return host.Start(ctx)
	case "stop":
		return host.Stop(ctx)
	case "restart":
		if err := host.Stop(ctx); err != nil {
			return err
		}
		return host.Start(ctx)
	case "setup-token":
		values, err := host.Environment()
		if err != nil {
			return err
		}
		fmt.Println(values["KARI_SETUP_TOKEN"])
		return nil
	case "backup":
		path, err := host.Backup(ctx)
		if err != nil {
			return err
		}
		fmt.Println(path)
		return host.Start(ctx)
	case "upgrade":
		path, err := host.Upgrade(ctx, *bundle, *checksum)
		if path != "" {
			fmt.Println("Control-plane backup:", path)
		}
		return err
	case "restore":
		if *confirm == "" {
			return fmt.Errorf("restore is destructive; provide --confirm with the backup's release version")
		}
		data, err := os.ReadFile(filepath.Join(*backup, "backup.json"))
		if err != nil {
			return err
		}
		var proof struct {
			Version string `json:"version"`
		}
		if err = json.Unmarshal(data, &proof); err != nil {
			return err
		}
		if proof.Version != *confirm {
			return fmt.Errorf("restore version confirmation does not match backup")
		}
		if err = host.Restore(ctx, *backup); err != nil {
			return err
		}
		fmt.Println("Restored matching release and database. Sessions, email links, and saved recovery codes were invalidated. The authenticator remains enrolled; save fresh recovery codes after re-enrollment.")
		return nil
	case "recover":
		if *email == "" || *confirm != *email {
			return fmt.Errorf("recovery requires --email and the same --confirm value")
		}
		if err := operations.PrivateFile(*passwordFile); err != nil {
			return err
		}
		data, err := os.ReadFile(*passwordFile)
		if err != nil {
			return err
		}
		if err = host.Stop(ctx); err != nil {
			return err
		}
		release, _, err := host.Current()
		if err != nil {
			return err
		}
		values, err := host.Environment()
		if err != nil {
			return err
		}
		command := []string{"run", "--rm", "--no-deps", "--entrypoint", "/app/kari-admin", "api", "recover-db", "--email", *email}
		if *resetMFA {
			command = append(command, "--reset-mfa")
		}
		if err = host.Compose(ctx, release, values, strings.NewReader(string(data)), io.Discard, command...); err != nil {
			return err
		}
		fmt.Println("Administrator credentials recovered; prior sessions and links revoked. Remove the password file after saving your new credentials.")
		return host.Start(ctx)
	default:
		return fmt.Errorf("unknown command %s", args[0])
	}
}

func recoverDatabase(ctx context.Context, email, password string, resetMFA bool) error {
	if os.Getenv("DATABASE_URL") == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if len(password) > 72 {
		return fmt.Errorf("password exceeds 72 bytes")
	}
	if err := utils.ValidatePasswordComplexity(password); err != nil {
		return err
	}
	pool, err := postgres.NewPool(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return postgres.NewUserRepo(pool).EmergencyRecover(ctx, email, string(hash), resetMFA)
}
