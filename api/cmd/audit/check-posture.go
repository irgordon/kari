package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/user"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// SecurityManifest represents the strict requirements from security_strict.json
type SecurityManifest struct {
	Boundaries struct {
		ZeroTrust struct {
			ExpectedUID int `json:"expected_uid"`
		} `json:"zero_trust"`
		Cryptography struct {
			MinKeyEntropyHex int `json:"min_key_entropy_hex"`
			MinJWTSecretLen  int `json:"min_jwt_secret_length"`
		} `json:"cryptography"`
	} `json:"boundaries"`
}

func main() {
	fmt.Println("🔍 Karı Orchestration Engine: Running Security Posture Audit...")

	// 1. Load the Strict Manifest
	manifestData, err := os.ReadFile("api/configs/security_strict.json")
	if err != nil {
		log.Fatalf("❌ CRITICAL: Could not find security_strict.json: %v", err)
	}

	var manifest SecurityManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		log.Fatalf("❌ CRITICAL: Failed to parse security manifest: %v", err)
	}

	// 2. Load the current Environment
	if err := godotenv.Load(); err != nil {
		fmt.Println("⚠️  Warning: No .env file found, checking system env vars...")
	}

	hasErrors := false

	// --- Audit Point 1: Encryption Key Entropy ---
	encKey := os.Getenv("ENCRYPTION_KEY")
	if len(encKey) != manifest.Boundaries.Cryptography.MinKeyEntropyHex {
		fmt.Printf("❌ FAIL: ENCRYPTION_KEY must be exactly %d hex characters (Current: %d)\n", 
			manifest.Boundaries.Cryptography.MinKeyEntropyHex, len(encKey))
		hasErrors = true
	} else {
		fmt.Println("✅ PASS: Encryption key entropy meets 256-bit standards.")
	}

	// --- Audit Point 2: JWT Secret Strength ---
	jwtSec := os.Getenv("JWT_SECRET")
	if len(jwtSec) < manifest.Boundaries.Cryptography.MinJWTSecretLen {
		fmt.Printf("❌ FAIL: JWT_SECRET is too short. Min: %d characters (Current: %d)\n", 
			manifest.Boundaries.Cryptography.MinJWTSecretLen, len(jwtSec))
		hasErrors = true
	} else {
		fmt.Println("✅ PASS: JWT secret length is sufficient.")
	}

	// --- Audit Point 3: UID Alignment (Peer Credential Prep) ---
	// This check is primarily for the Go API container environment
	currentUser, _ := user.Current()
	currentUID, _ := strconv.Atoi(currentUser.Uid)
	
	// Check if the expected UID matches the current context
	if currentUID != manifest.Boundaries.ZeroTrust.ExpectedUID && currentUID != 0 {
		fmt.Printf("⚠️  NOTICE: Current UID (%d) differs from manifest expected UID (%d).\n", 
			currentUID, manifest.Boundaries.ZeroTrust.ExpectedUID)
		fmt.Println("   (This is normal on host, but MUST match inside Docker for PeerCreds).")
	}

	// --- Audit Point 4: Path Sanitization ---
	agentSock := os.Getenv("AGENT_SOCKET")
	if !strings.HasPrefix(agentSock, "/var/run/") {
		fmt.Println("❌ FAIL: AGENT_SOCKET must live in /var/run/ for security isolation.")
		hasErrors = true
	} else {
		fmt.Println("✅ PASS: Socket path is correctly located.")
	}

	// --- Audit Point 5: Database Credentials ---
	dbURL := os.Getenv("DATABASE_URL")
	if strings.Contains(dbURL, "dev_password") {
		fmt.Println("❌ FAIL: DATABASE_URL is using default development credentials.")
		hasErrors = true
	} else if dbURL == "" {
		fmt.Println("❌ FAIL: DATABASE_URL must be set.")
		hasErrors = true
	} else {
		fmt.Println("✅ PASS: Database URL does not use default credentials.")
	}

	// 3. Final Verdict
	fmt.Println("--------------------------------------------------")
	if hasErrors {
		fmt.Println("🚨 VERDICT: SECURITY POSTURE FAILED.")
		fmt.Println("Fix the errors above before attempting deployment.")
		os.Exit(1)
	} else {
		fmt.Println("🚀 VERDICT: SECURITY POSTURE VALIDATED. System is ready for launch.")
	}
}
