package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStatusUnavailableIsSanitizedAndDoesNotContactProvider(t *testing.T) {
	provider := &fakeProvider{}
	handler := NewHandler(DefaultConfig(), WithProvider(provider))

	req := authenticatedRequest(http.MethodPost, "/llmStatus", `{}`)
	w := httptest.NewRecorder()
	handler.Status(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 0, provider.calls())
	require.NotContains(t, w.Body.String(), "sk-test")
	require.NotContains(t, w.Body.String(), "llm.example.test")

	var resp statusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "ok", resp.Status)
	require.False(t, resp.Available)
	require.Equal(t, "not_configured", resp.Reason)
}

func TestChatValidatesPayloadOriginAndSizeBeforeProviderCall(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		origin     string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "invalid action",
			body:       `{"action":"translate","prompt":"hi","contexts":[{"source":"selected-chat","text":"hello"}]}`,
			origin:     "https://app.example.test",
			wantStatus: http.StatusBadRequest,
			wantCode:   "bad_request",
		},
		{
			name:       "missing origin",
			body:       `{"action":"ask","prompt":"hi","contexts":[{"source":"selected-chat","text":"hello"}]}`,
			wantStatus: http.StatusForbidden,
			wantCode:   "forbidden",
		},
		{
			name:       "foreign origin",
			body:       `{"action":"ask","prompt":"hi","contexts":[{"source":"selected-chat","text":"hello"}]}`,
			origin:     "https://evil.example.test",
			wantStatus: http.StatusForbidden,
			wantCode:   "forbidden",
		},
		{
			name:       "oversized context",
			body:       `{"action":"ask","prompt":"hi","contexts":[{"source":"selected-chat","text":"abcdef"}]}`,
			origin:     "https://app.example.test",
			wantStatus: http.StatusRequestEntityTooLarge,
			wantCode:   "too_large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &fakeProvider{}
			cfg := testConfig()
			cfg.MaxContextBytes = 5
			handler := NewHandler(cfg, WithProvider(provider))

			req := authenticatedRequest(http.MethodPost, "/llmChat", tt.body)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			w := httptest.NewRecorder()
			handler.Chat(w, req)

			require.Equal(t, tt.wantStatus, w.Code)
			require.Equal(t, 0, provider.calls())
			assertErrorCode(t, w.Body.Bytes(), tt.wantCode)
		})
	}
}

func TestChatRejectsRateQuotaAndConcurrencyBeforeProviderCall(t *testing.T) {
	t.Run("rate limit", func(t *testing.T) {
		provider := &fakeProvider{response: ChatResponse{Model: "test-model", Text: "ok"}}
		cfg := testConfig()
		cfg.UserRequestsPerMinute = 1
		handler := NewHandler(cfg, WithProvider(provider), WithRequestID(func() string { return "req-rate" }))

		first := authenticatedLLMChat(`{"action":"ask","prompt":"hi","contexts":[{"source":"selected-chat","text":"hello"}]}`)
		firstW := httptest.NewRecorder()
		handler.Chat(firstW, first)
		require.Equal(t, http.StatusOK, firstW.Code)

		second := authenticatedLLMChat(`{"action":"ask","prompt":"again","contexts":[{"source":"selected-chat","text":"hello"}]}`)
		secondW := httptest.NewRecorder()
		handler.Chat(secondW, second)
		require.Equal(t, http.StatusTooManyRequests, secondW.Code)
		require.Equal(t, 1, provider.calls())
		assertErrorCode(t, secondW.Body.Bytes(), "rate_limited")
	})

	t.Run("daily quota", func(t *testing.T) {
		provider := &fakeProvider{response: ChatResponse{Model: "test-model", Text: "ok"}}
		cfg := testConfig()
		cfg.UserDailyQuota = 1
		handler := NewHandler(cfg, WithProvider(provider), WithRequestID(func() string { return "req-quota" }))

		firstW := httptest.NewRecorder()
		handler.Chat(firstW, authenticatedLLMChat(`{"action":"ask","prompt":"hi","contexts":[{"source":"selected-chat","text":"hello"}]}`))
		require.Equal(t, http.StatusOK, firstW.Code)

		secondW := httptest.NewRecorder()
		handler.Chat(secondW, authenticatedLLMChat(`{"action":"ask","prompt":"again","contexts":[{"source":"selected-chat","text":"hello"}]}`))
		require.Equal(t, http.StatusTooManyRequests, secondW.Code)
		require.Equal(t, 1, provider.calls())
		assertErrorCode(t, secondW.Body.Bytes(), "rate_limited")
	})

	t.Run("ip rate limit", func(t *testing.T) {
		provider := &fakeProvider{response: ChatResponse{Model: "test-model", Text: "ok"}}
		cfg := testConfig()
		cfg.IPRequestsPerMinute = 1
		handler := NewHandler(cfg, WithProvider(provider), WithRequestID(func() string { return "req-ip-rate" }))

		firstW := httptest.NewRecorder()
		handler.Chat(firstW, authenticatedLLMChat(`{"action":"ask","prompt":"hi","contexts":[{"source":"selected-chat","text":"hello"}]}`))
		require.Equal(t, http.StatusOK, firstW.Code)

		secondW := httptest.NewRecorder()
		handler.Chat(secondW, authenticatedLLMChat(`{"action":"ask","prompt":"again","contexts":[{"source":"selected-chat","text":"hello"}]}`))
		require.Equal(t, http.StatusTooManyRequests, secondW.Code)
		require.Equal(t, 1, provider.calls())
		assertErrorCode(t, secondW.Body.Bytes(), "rate_limited")
	})

	t.Run("concurrency", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		provider := &fakeProvider{fn: func(ctx context.Context, req ChatRequest) (ChatResponse, error) {
			close(started)
			<-release
			return ChatResponse{Model: "test-model", Text: "ok"}, nil
		}}
		cfg := testConfig()
		cfg.UserConcurrency = 1
		handler := NewHandler(cfg, WithProvider(provider))

		firstW := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			handler.Chat(firstW, authenticatedLLMChat(`{"action":"ask","prompt":"hi","contexts":[{"source":"selected-chat","text":"hello"}]}`))
			close(done)
		}()
		<-started

		secondW := httptest.NewRecorder()
		handler.Chat(secondW, authenticatedLLMChat(`{"action":"ask","prompt":"again","contexts":[{"source":"selected-chat","text":"hello"}]}`))
		require.Equal(t, http.StatusTooManyRequests, secondW.Code)
		assertErrorCode(t, secondW.Body.Bytes(), "rate_limited")

		close(release)
		<-done
		require.Equal(t, http.StatusOK, firstW.Code)
		require.Equal(t, 1, provider.calls())
	})
}

func TestChatMapsProviderSuccessAndSanitizesProviderErrors(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var got ChatRequest
		provider := &fakeProvider{fn: func(ctx context.Context, req ChatRequest) (ChatResponse, error) {
			got = req
			return ChatResponse{Model: "provider-model", Text: "draft text"}, nil
		}}
		handler := NewHandler(testConfig(), WithProvider(provider), WithRequestID(func() string { return "req-123" }))

		w := httptest.NewRecorder()
		handler.Chat(w, authenticatedLLMChat(`{"action":"summarize","prompt":"Summarize","contexts":[{"source":"selected-chat","label":"Selected","text":"hello"}],"providerBaseURL":"https://evil.example.test/v1"}`))

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "summarize", got.Action)
		require.Equal(t, "Summarize", got.Prompt)
		require.Equal(t, 1, len(got.Contexts))
		require.Equal(t, 1024, got.MaxOutputTokens)
		require.NotEqual(t, "https://evil.example.test/v1", got.ProviderBaseURL)

		var resp chatResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, "ok", resp.Status)
		require.Equal(t, "req-123", resp.RequestID)
		require.Equal(t, "provider-model", resp.Model)
		require.Equal(t, "draft text", resp.Text)
	})

	t.Run("provider error is safe", func(t *testing.T) {
		provider := &fakeProvider{err: errors.New("raw provider body with sk-test and prompt text")}
		handler := NewHandler(testConfig(), WithProvider(provider))

		w := httptest.NewRecorder()
		handler.Chat(w, authenticatedLLMChat(`{"action":"ask","prompt":"secret prompt","contexts":[{"source":"selected-chat","text":"secret context"}]}`))

		require.Equal(t, http.StatusBadGateway, w.Code)
		body := w.Body.String()
		require.NotContains(t, body, "sk-test")
		require.NotContains(t, body, "secret prompt")
		require.NotContains(t, body, "secret context")
		require.NotContains(t, body, "raw provider body")
		assertErrorCode(t, w.Body.Bytes(), "provider_error")
	})
}

func TestChatMapsProviderTimeout(t *testing.T) {
	provider := &fakeProvider{err: context.DeadlineExceeded}
	cfg := testConfig()
	cfg.Timeout = 10 * time.Millisecond
	handler := NewHandler(cfg, WithProvider(provider))

	w := httptest.NewRecorder()
	handler.Chat(w, authenticatedLLMChat(`{"action":"ask","prompt":"hi","contexts":[{"source":"selected-chat","text":"hello"}]}`))

	require.Equal(t, http.StatusGatewayTimeout, w.Code)
	assertErrorCode(t, w.Body.Bytes(), "provider_timeout")
}

func TestAuthErrorJSONNormalizesMiddlewareFailures(t *testing.T) {
	next := AuthErrorJSON(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}))

	w := httptest.NewRecorder()
	next.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/llmChat", nil))

	require.Equal(t, http.StatusUnauthorized, w.Code)
	assertErrorCode(t, w.Body.Bytes(), "unauthorized")
}

func testConfig() Config {
	cfg := DefaultConfig()
	cfg.ProviderBaseURL = "https://llm.example.test/v1"
	cfg.Model = "test-model"
	cfg.APIKey = "sk-test"
	cfg.AppURL = "https://app.example.test"
	cfg.Timeout = time.Second
	return cfg
}

func authenticatedLLMChat(body string) *http.Request {
	req := authenticatedRequest(http.MethodPost, "/llmChat", body)
	req.Header.Set("Origin", "https://app.example.test")
	return req
}

func authenticatedRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(context.WithValue(req.Context(), UserIDContextKey, int64(42)))
}

func assertErrorCode(t *testing.T, body []byte, want string) {
	t.Helper()

	var resp errorResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	require.Equal(t, "error", resp.Status)
	require.Equal(t, want, resp.Code)
	require.NotEmpty(t, resp.Message)
}

type fakeProvider struct {
	mu       sync.Mutex
	count    int
	response ChatResponse
	err      error
	fn       func(context.Context, ChatRequest) (ChatResponse, error)
}

func (f *fakeProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	f.mu.Lock()
	f.count++
	f.mu.Unlock()

	if f.fn != nil {
		return f.fn(ctx, req)
	}
	return f.response, f.err
}

func (f *fakeProvider) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.count
}

func decodeProviderRequest(t *testing.T, r *http.Request) map[string]any {
	t.Helper()

	var payload map[string]any
	require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
	return payload
}

func TestOpenAIClientUsesConfiguredURLAndTokenCaps(t *testing.T) {
	var got map[string]any
	var gotAuth string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		got = decodeProviderRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"returned-model","choices":[{"message":{"content":"provider draft"}}]}`))
	}))
	defer provider.Close()

	cfg := testConfig()
	cfg.ProviderBaseURL = provider.URL + "/v1"
	cfg.MaxOutputTokens = 17
	client, err := NewOpenAIClient(cfg)
	require.NoError(t, err)

	resp, err := client.Chat(context.Background(), ChatRequest{
		Action:          "ask",
		Prompt:          "answer this",
		Contexts:        []ContextBlock{{Source: "selected-chat", Label: "Selected", Text: "hello"}},
		MaxOutputTokens: 17,
	})

	require.NoError(t, err)
	require.Equal(t, "returned-model", resp.Model)
	require.Equal(t, "provider draft", resp.Text)
	require.Equal(t, "Bearer sk-test", gotAuth)
	require.Equal(t, "test-model", got["model"])
	require.Equal(t, float64(17), got["max_tokens"])
	require.Equal(t, false, got["stream"])
	require.NotContains(t, string(mustJSON(t, got)), "providerBaseURL")
}

func TestOpenAIClientRejectsUnsafeURLsAndSanitizesProviderFailures(t *testing.T) {
	for _, rawURL := range []string{
		"http://example.com/v1",
		"ftp://localhost/v1",
		"https://",
	} {
		t.Run(rawURL, func(t *testing.T) {
			cfg := testConfig()
			cfg.ProviderBaseURL = rawURL
			_, err := NewOpenAIClient(cfg)
			require.Error(t, err)
		})
	}

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret raw body sk-test", http.StatusInternalServerError)
	}))
	defer provider.Close()

	cfg := testConfig()
	cfg.ProviderBaseURL = provider.URL + "/v1"
	client, err := NewOpenAIClient(cfg)
	require.NoError(t, err)

	_, err = client.Chat(context.Background(), ChatRequest{Action: "ask", Prompt: "secret", MaxOutputTokens: 17})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret")
	require.NotContains(t, err.Error(), "sk-test")
}

func TestOpenAIClientEnforcesResponseSizeCap(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 16))
	}))
	defer provider.Close()

	cfg := testConfig()
	cfg.ProviderBaseURL = provider.URL + "/v1"
	cfg.MaxResponseBytes = 8
	client, err := NewOpenAIClient(cfg)
	require.NoError(t, err)

	_, err = client.Chat(context.Background(), ChatRequest{Action: "ask", Prompt: "hi", MaxOutputTokens: 17})
	require.Error(t, err)
	require.Contains(t, err.Error(), "response_too_large")
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}
