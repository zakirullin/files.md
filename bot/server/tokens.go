package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"

	"zakirullin/stuffbot/config"
	"zakirullin/stuffbot/bot/fs"
)

const (
	TokenLength            = 32
	OneTimeTokenExpiration = 10 * time.Minute
)

var (
	oneTimeTokens = make(map[string]oneTimeToken)
	mu            sync.RWMutex
)

var blockedIPs = make(map[string]time.Time)
var blockedIPsMutex sync.RWMutex

type oneTimeToken struct {
	userID    int64
	expiresAt time.Time
}

func GenOneTimeToken(userID int64) string {
	token := genToken()

	mu.Lock()
	oneTimeTokens[token] = oneTimeToken{
		userID:    userID,
		expiresAt: time.Now().Add(OneTimeTokenExpiration),
	}
	mu.Unlock()

	return token
}

func findUserID(token string) (int64, bool) {
	tokens, err := fs.NewFS(config.BotCfg.TokensDir, afero.NewOsFs())
	if err != nil {
		slog.Error("Failed to create file system for tokens", "error", err)
		return 0, false
	}

	data, err := tokens.Read(fs.DirRoot, hashToken(token))
	if err != nil {
		return 0, false
	}

	userID, err := strconv.ParseInt(data, 10, 64)
	if err != nil {
		return 0, false
	}

	return userID, true
}

func IssueToken(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in IssueToken: %v", r)
			http.Error(w, "Internal server error", 500)
		}
	}()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	permanentToken, ok := issueNewPermanentToken(r)
	if !ok {
		http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(map[string]string{"token": permanentToken})
	if err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// TODO CHECK that user id belongs to oneTimeToken ID, or get user id by oneTimeToken
// TODO add tests
// TODO too harsh blocking, we may need to take into account proxies
func tokenMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := getIPFromRemoteAddr(r.RemoteAddr)

		blockedIPsMutex.RLock()
		blockedUntil, isBlocked := blockedIPs[ip]
		blockedIPsMutex.RUnlock()
		if isBlocked && time.Now().Before(blockedUntil) {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		token := r.Header.Get("Authorization")
		userID, ok := findUserID(token)
		if !ok {
			blockedIPsMutex.Lock()
			blockedIPs[ip] = time.Now().Add(10 * time.Minute)
			blockedIPsMutex.Unlock()

			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), "userID", userID)
		next(w, r.WithContext(ctx))
	}
}

// TODO add tests
func issueNewPermanentToken(r *http.Request) (string, bool) {
	// Return false if IP is blocked.
	ipAndPort := strings.Split(r.RemoteAddr, ":")
	ip := ipAndPort[0]
	blockedIPsMutex.RLock()
	blockedUntil, isBlocked := blockedIPs[ip]
	blockedIPsMutex.RUnlock()
	if isBlocked && time.Now().Before(blockedUntil) {
		return "", false
	}

	var req struct {
		OneTimeToken string `json:"oneTimeToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return "", false
	}

	mu.Lock()
	data, exists := oneTimeTokens[req.OneTimeToken]
	if !exists || time.Now().After(data.expiresAt) {
		mu.Unlock()

		// Block IP for 1 minute if token is invalid or expired
		blockedIPsMutex.Lock()
		blockedIPs[ip] = time.Now().Add(1 * time.Minute)
		blockedIPsMutex.Unlock()

		return "", false
	}
	delete(oneTimeTokens, req.OneTimeToken)
	mu.Unlock()

	token := genToken()
	tokens, err := fs.NewFS(config.BotCfg.TokensDir, afero.NewOsFs())
	if err != nil {
		slog.Error("Failed to create file system for tokens", "error", err)
		return "", false
	}
	err = tokens.Write(fs.DirRoot, hashToken(token), strconv.FormatInt(data.userID, 10))
	if err != nil {
		return "", false
	}

	return token, true
}

func genToken() string {
	bytes := make([]byte, TokenLength)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func hashToken(token string) string {
	// A token is a server-generated 32 bytes of entropy, so SHA-256 is fine here.
	// At 1 billion SHA256 hashes per second it would take ~10^60 years to brute force.
	h := sha256.New()
	h.Write([]byte(token + config.BotCfg.TokensSalt))
	return hex.EncodeToString(h.Sum(nil))
}

func getIPFromRemoteAddr(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// If SplitHostPort fails, might be just an IP without port
		if ip := net.ParseIP(remoteAddr); ip != nil {
			return remoteAddr
		}
		return "unknown"
	}
	return host
}
