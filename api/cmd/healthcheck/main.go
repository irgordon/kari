package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	target := healthcheckTarget()

	// 🛡️ SLA: Tight timeout for orchestration responsiveness
	client := http.Client{
		Timeout: 2 * time.Second,
	}

	start := time.Now()
	resp, err := client.Get(target)

	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Kari Brain Unreachable: %v (Duration: %v)\n", err, time.Since(start))
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// This captures scenarios where the Brain is alive but the Muscle link is dead
		fmt.Fprintf(os.Stderr, "⚠️ Kari Brain Paralyzed: Received HTTP %d (Duration: %v)\n", resp.StatusCode, time.Since(start))
		os.Exit(1)
	}

	// Success remains silent to keep Docker logs clean
	os.Exit(0)
}

func healthcheckTarget() string {
	target := os.Getenv("HEALTHCHECK_TARGET")
	if target == "" {
		return "http://localhost:8080/ready"
	}
	return target
}
