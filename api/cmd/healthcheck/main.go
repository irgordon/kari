package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	// 🛡️ Zero-Trust: Tight timeout to prevent hanging health checks
	client := http.Client{
		Timeout: 2 * time.Second,
	}

	// Internal health endpoint on the Brain
	resp, err := client.Get("http://localhost:8080/health")
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "Healthcheck failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Healthcheck failed: Received status %d\n", resp.StatusCode)
		os.Exit(1)
	}

	// System is Operational
	os.Exit(0)
}
