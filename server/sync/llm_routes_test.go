package sync

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zakirullin/files.md/server/config"
)

func TestLLMRoutesAreAuthenticatedAndReturnNormalizedAuthErrors(t *testing.T) {
	origCfg := config.ServerCfg
	defer func() { config.ServerCfg = origCfg }()
	resetAuthState(t)

	config.ServerCfg = config.Config{
		APIURL:    "https://api.example.test",
		AppURL:    "https://app.example.test",
		TokensDir: t.TempDir(),
	}

	req := httptest.NewRequest(http.MethodPost, "/llmStatus", nil)
	w := httptest.NewRecorder()
	router(log.New(io.Discard, "", 0)).ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.JSONEq(t, `{"status":"error","code":"unauthorized","message":"Authentication is required."}`, w.Body.String())
}

func TestLLMRoutesUseExistingTokenMiddlewareAndDoNotExposeConfig(t *testing.T) {
	origCfg := config.ServerCfg
	defer func() { config.ServerCfg = origCfg }()
	resetAuthState(t)

	token := "valid-token"
	tokensDir := t.TempDir()
	config.ServerCfg = config.Config{
		APIURL:             "https://api.example.test",
		AppURL:             "https://app.example.test",
		TokensDir:          tokensDir,
		LLMProviderBaseURL: "https://llm.example.test/v1",
		LLMModel:           "test-model",
		LLMAPIKey:          "sk-test",
	}
	require.NoError(t, os.WriteFile(filepath.Join(tokensDir, hashToken(token)), []byte("42"), 0o600))

	req := httptest.NewRequest(http.MethodPost, "/llmStatus", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router(log.New(io.Discard, "", 0)).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"available":true`)
	require.Contains(t, w.Body.String(), `"model":"test-model"`)
	require.NotContains(t, w.Body.String(), "sk-test")
	require.NotContains(t, w.Body.String(), "llm.example.test")
}

func resetAuthState(t *testing.T) {
	t.Helper()

	mu.Lock()
	oneTimeTokens = make(map[string]oneTimeToken)
	mu.Unlock()

	blockedIPsMutex.Lock()
	blockedIPs = make(map[string]time.Time)
	blockedIPsMutex.Unlock()
}
