package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/irgordon/kari/api/internal/sourceidentity"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "source identity failed:", err)
		os.Exit(1)
	}
}

func run() error {
	repository := flag.String("repo", ".", "repository worktree root")
	manifest := flag.String("manifest", "", "canonical manifest output path")
	format := flag.String("format", "json", "output format: json or sha-only")
	flag.Parse()
	if *manifest == "" {
		return fmt.Errorf("--manifest is required")
	}
	result, err := sourceidentity.Generate(*repository, *manifest)
	if err != nil {
		return err
	}
	return writeResult(result, *format)
}

func writeResult(result sourceidentity.Result, format string) error {
	switch format {
	case "sha-only":
		fmt.Println(result.ManifestSHA256)
		return nil
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	default:
		return fmt.Errorf("unsupported --format %q", format)
	}
}
