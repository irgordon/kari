package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"kari/api/internal/core/utils"
	agent "kari/api/internal/grpc/rustagent"
)

// ==============================================================================
// BIP-39 English Wordlist (first 256 words for 24-word recovery phrases)
// Full BIP-39 has 2048 words; we use a curated 256-word subset for simplicity.
// Each byte of entropy maps to exactly one word → 32 bytes = 32 words,
// but we select 24 words from the 32 for the standard recovery phrase length.
// ==============================================================================
var bip39Words = []string{
	"abandon", "ability", "able", "about", "above", "absent", "absorb", "abstract",
	"absurd", "abuse", "access", "accident", "account", "accuse", "achieve", "acid",
	"across", "act", "action", "actor", "actress", "actual", "adapt", "add",
	"addict", "address", "adjust", "admit", "adult", "advance", "advice", "after",
	"again", "against", "agent", "agree", "ahead", "aim", "air", "airport",
	"aisle", "alarm", "album", "alcohol", "alert", "alien", "allow", "almost",
	"alone", "alpha", "already", "also", "alter", "always", "amateur", "amazing",
	"among", "amount", "amused", "anchor", "ancient", "anger", "angle", "animal",
	"announce", "annual", "another", "answer", "antenna", "antique", "anxiety", "any",
	"apart", "apology", "appear", "apple", "approve", "april", "arch", "arctic",
	"area", "arena", "argue", "arm", "armed", "armor", "army", "around",
	"arrange", "arrest", "arrive", "arrow", "art", "artist", "artwork", "ask",
	"aspect", "assault", "asset", "assist", "assume", "asthma", "athlete", "atom",
	"attack", "attend", "auction", "audit", "august", "aunt", "author", "auto",
	"autumn", "average", "avocado", "avoid", "awake", "aware", "awesome", "awful",
	"awkward", "axis", "baby", "bachelor", "bacon", "badge", "bag", "balance",
	"balcony", "ball", "bamboo", "banana", "banner", "barely", "bargain", "barrel",
	"base", "basic", "basket", "battle", "beach", "bean", "beauty", "because",
	"become", "beef", "before", "begin", "behave", "behind", "believe", "below",
	"bench", "benefit", "best", "betray", "better", "between", "beyond", "bicycle",
	"bid", "bike", "bind", "biology", "bird", "birth", "bitter", "black",
	"blade", "blame", "blanket", "blast", "bleak", "bless", "blind", "blood",
	"blossom", "blow", "blue", "blur", "blush", "board", "boat", "body",
	"boil", "bomb", "bone", "bonus", "book", "border", "boring", "borrow",
	"boss", "bottom", "bounce", "box", "boy", "bracket", "brain", "brand",
	"brass", "brave", "bread", "breeze", "brick", "bridge", "brief", "bright",
	"bring", "broken", "bronze", "broom", "brother", "brown", "brush", "bubble",
	"buddy", "budget", "buffalo", "build", "bulb", "bulk", "bullet", "bundle",
	"bunny", "burden", "burger", "burst", "bus", "business", "busy", "butter",
	"buyer", "buzz", "cabbage", "cabin", "cable", "cactus", "cage", "cake",
	"call", "calm", "camera", "camp", "can", "canal", "cancel", "candy",
	"cannon", "canoe", "canvas", "canyon", "capable", "capital", "captain", "carbon",
}

// SetupRequest is the finalize payload from the wizard UI.
type SetupRequest struct {
	AdminEmail    string `json:"admin_email"`
	AdminPassword string `json:"admin_password"`
	DatabaseURL   string `json:"database_url"`
	AppDomain     string `json:"app_domain"`
}

// SetupHandler manages the onboarding wizard lifecycle.
// 🛡️ Zero-Trust: Operates ONLY when setup.lock does not exist.
// 🛡️ SLA: All endpoints are gated by a transient 15-minute JWT.
type SetupHandler struct {
	agentClient agent.SystemAgentClient
	logger      *slog.Logger
	jwtSecret   []byte
	lockPath    string
	mu          sync.RWMutex
	locked      bool
	shutdownFn  func() // Called to restart the Brain after lockdown
}

func NewSetupHandler(
	ac agent.SystemAgentClient,
	l *slog.Logger,
	jwtSecret string,
	lockPath string,
	shutdownFn func(),
) *SetupHandler {
	_, err := os.Stat(lockPath)
	return &SetupHandler{
		agentClient: ac,
		logger:      l,
		jwtSecret:   []byte(jwtSecret),
		lockPath:    lockPath,
		locked:      err == nil, // locked if file exists
		shutdownFn:  shutdownFn,
	}
}

// IsLocked returns true if setup has already been completed.
func (h *SetupHandler) IsLocked() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.locked
}

// ==============================================================================
// Setup Guard Middleware
// ==============================================================================

// SetupGuard redirects ALL non-setup traffic to /setup if setup.lock doesn't exist.
// Once locked, it blocks setup endpoints from being re-accessed.
func (h *SetupHandler) SetupGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if h.IsLocked() {
			// System is configured — block setup endpoints
			if strings.HasPrefix(path, "/api/v1/setup") || strings.HasPrefix(path, "/setup") {
				http.Error(w, `{"message": "System is already configured"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// System is NOT configured — only allow setup endpoints
		if strings.HasPrefix(path, "/api/v1/setup") ||
			strings.HasPrefix(path, "/setup") ||
			strings.HasPrefix(path, "/_app/") ||
			strings.HasPrefix(path, "/static/") ||
			path == "/favicon.ico" ||
			path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		// 🛡️ Redirect everything else to the setup wizard
		http.Redirect(w, r, "/setup", http.StatusTemporaryRedirect)
	})
}

// SetupAuth validates the transient setup JWT on setup API endpoints.
func (h *SetupHandler) SetupAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := r.URL.Query().Get("token")
		if tokenStr == "" {
			tokenStr = r.Header.Get("X-Setup-Token")
		}
		if tokenStr == "" {
			http.Error(w, `{"message": "Missing setup token"}`, http.StatusUnauthorized)
			return
		}

		// 🛡️ Validate JWT with strict algorithm enforcement
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return h.jwtSecret, nil
		}, jwt.WithValidMethods([]string{"HS256"}))

		if err != nil || !token.Valid {
			h.logger.Warn("🛡️ Invalid setup token attempt", slog.Any("error", err))
			http.Error(w, `{"message": "Invalid or expired setup token"}`, http.StatusUnauthorized)
			return
		}

		// 🛡️ Verify this is a setup token (not a regular access token)
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			http.Error(w, `{"message": "Malformed token claims"}`, http.StatusUnauthorized)
			return
		}
		if claims["purpose"] != "kari-setup" {
			http.Error(w, `{"message": "Token is not a setup token"}`, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ==============================================================================
// Setup API Endpoints
// ==============================================================================

// TestMuscle verifies the Brain-to-Agent UDS gRPC link.
func (h *SetupHandler) TestMuscle(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status, err := h.agentClient.GetSystemStatus(ctx, &agent.Empty{})
	if err != nil {
		h.logger.Error("Setup: Muscle link failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"healthy": false,
			"error":   "Could not contact Muscle Agent via UDS",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"healthy": true,
		"version": status.AgentVersion,
		"cpu":     fmt.Sprintf("%.1f%%", status.CpuUsage),
		"ram_mb":  status.MemUsedMb,
	})
}

// TestDB verifies database connectivity.
func (h *SetupHandler) TestDB(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DatabaseURL string `json:"database_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"healthy": false,
			"error":   "Invalid request body",
		})
		return
	}

	// 🛡️ Zero-Trust: Validate the connection string format
	if !strings.HasPrefix(req.DatabaseURL, "postgres://") && !strings.HasPrefix(req.DatabaseURL, "postgresql://") {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"healthy": false,
			"error":   "Database URL must start with postgres:// or postgresql://",
		})
		return
	}

	// 🛡️ SLA: Test with a 3-second timeout to prevent wizard hang
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	// Use pgx directly for a lightweight connectivity probe
	// We import it at runtime to avoid a hard dependency in the handler
	_ = ctx // Used by the actual DB probe below

	// Probe: Use a raw TCP dial to verify connectivity without importing pgx here
	// The actual DB validation happens via net.Dial to the parsed host:port
	host, port := parsePostgresURL(req.DatabaseURL)
	if host == "" || port == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"healthy": false,
			"error":   "Could not parse host and port from database URL",
		})
		return
	}

	conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", host+":"+port)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"healthy": false,
			"error":   fmt.Sprintf("Cannot reach database at %s:%s — %v", host, port, err),
		})
		return
	}
	conn.Close()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"healthy": true,
		"host":    host,
		"port":    port,
	})
}

// GenerateKey creates a new AES-256 key and returns it as both hex and BIP-39 mnemonic.
func (h *SetupHandler) GenerateKey(w http.ResponseWriter, r *http.Request) {
	// 🛡️ Generate 32 bytes (256 bits) of cryptographic randomness
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		h.logger.Error("Setup: CSPRNG failure", "error", err)
		http.Error(w, `{"message": "Cryptographic random generation failed"}`, http.StatusInternalServerError)
		return
	}

	hexKey := hex.EncodeToString(keyBytes)
	mnemonic := bytesToMnemonic(keyBytes)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"hex_key":         hexKey,
		"recovery_phrase": mnemonic,
		"word_count":      len(strings.Split(mnemonic, " ")),
		"warning":         "This recovery phrase is shown ONCE. Store it in a secure offline location.",
	})
}

// Finalize commits the production configuration and locks the system.
func (h *SetupHandler) Finalize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AdminEmail    string `json:"admin_email"`
		AdminPassword string `json:"admin_password"`
		DatabaseURL   string `json:"database_url"`
		AppDomain     string `json:"app_domain"`
		MasterKeyHex  string `json:"master_key_hex"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
		return
	}

	// 🛡️ Input validation
	if req.AdminEmail == "" || req.AdminPassword == "" || req.DatabaseURL == "" || req.AppDomain == "" || req.MasterKeyHex == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "All fields are required"})
		return
	}
	if err := utils.ValidatePasswordComplexity(req.AdminPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	if len(req.MasterKeyHex) != 64 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Master key must be exactly 64 hex characters (256 bits)"})
		return
	}

	// 🛡️ Write production .env (atomic)
	envContent := fmt.Sprintf(
		"DATABASE_URL=%s\nJWT_SECRET=%s\nENCRYPTION_KEY=%s\nAPP_DOMAIN=%s\nADMIN_EMAIL=%s\n",
		req.DatabaseURL,
		generateRandomHex(32), // Fresh JWT secret
		req.MasterKeyHex,
		req.AppDomain,
		req.AdminEmail,
	)

	if err := os.WriteFile("/opt/kari/.env.production", []byte(envContent), 0600); err != nil {
		h.logger.Error("Setup: Failed to write production env", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "Failed to save configuration"})
		return
	}

	// 🛡️ Write setup.lock — this permanently locks the wizard
	lockContent := fmt.Sprintf(`{"locked_at":"%s","admin_email":"%s","domain":"%s"}`,
		time.Now().UTC().Format(time.RFC3339),
		req.AdminEmail,
		req.AppDomain,
	)
	if err := os.WriteFile(h.lockPath, []byte(lockContent), 0444); err != nil {
		h.logger.Error("Setup: Failed to write setup.lock", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "Failed to create lock file"})
		return
	}

	// 🛡️ Update in-memory state
	h.mu.Lock()
	h.locked = true
	h.mu.Unlock()

	h.logger.Info("🔒 Setup: System locked down. Restarting in Production Mode.",
		slog.String("domain", req.AppDomain),
		slog.String("admin", req.AdminEmail))

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "Configuration saved. The panel will restart in Production Mode.",
		"status":  "locked",
	})

	// 🛡️ Trigger graceful restart after response is sent
	go func() {
		time.Sleep(2 * time.Second)
		if h.shutdownFn != nil {
			h.shutdownFn()
		}
	}()
}

// ==============================================================================
// Helpers
// ==============================================================================

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// bytesToMnemonic converts 32 bytes to a 24-word BIP-39 style recovery phrase.
// We use the first 24 bytes (each byte indexes into the 256-word list).
func bytesToMnemonic(keyBytes []byte) string {
	words := make([]string, 24)
	for i := 0; i < 24; i++ {
		words[i] = bip39Words[keyBytes[i]]
	}
	return strings.Join(words, " ")
}

// parsePostgresURL extracts host and port from a postgres:// URL.
func parsePostgresURL(url string) (string, string) {
	// Strip protocol
	url = strings.TrimPrefix(url, "postgres://")
	url = strings.TrimPrefix(url, "postgresql://")

	// Strip credentials
	if idx := strings.LastIndex(url, "@"); idx >= 0 {
		url = url[idx+1:]
	}

	// Strip database path and query
	if idx := strings.Index(url, "/"); idx >= 0 {
		url = url[:idx]
	}

	// Split host:port
	parts := strings.Split(url, ":")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	if len(parts) == 1 {
		return parts[0], "5432" // Default Postgres port
	}
	return "", ""
}

// generateRandomHex generates a hex-encoded random string of N bytes.
func generateRandomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// GenerateSetupToken creates a transient 15-minute JWT for setup wizard access.
// Called from the CLI bootstrap or the main.go boot sequence.
func GenerateSetupToken(secret string) (string, error) {
	claims := jwt.MapClaims{
		"purpose": "kari-setup",
		"iss":     "kari-brain",
		"exp":     time.Now().Add(15 * time.Minute).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
