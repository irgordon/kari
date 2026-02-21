package main

import (
	"net/http"
	"os"
)

func main() {
	// Points to the internal port of the Brain
	resp, err := http.Get("http://localhost:8080/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		os.Exit(1) // Docker marks as UNHEALTHY
	}
	os.Exit(0) // Docker marks as HEALTHY
}
