package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/zakirullin/files.md/server/config"
)

const UserIDContextKey = "userID"

type Config struct {
	ProviderBaseURL       string
	Model                 string
	APIKey                string
	AppURL                string
	Timeout               time.Duration
	MaxBodyBytes          int64
	MaxPromptChars        int
	MaxContextBlocks      int
	MaxContextBytes       int64
	MaxResponseBytes      int64
	MaxOutputTokens       int
	UserRequestsPerMinute int
	IPRequestsPerMinute   int
	UserConcurrency       int
	UserDailyQuota        int
}

func DefaultConfig() Config {
	return Config{
		Timeout:               30 * time.Second,
		MaxBodyBytes:          256 << 10,
		MaxPromptChars:        8000,
		MaxContextBlocks:      8,
		MaxContextBytes:       128 << 10,
		MaxResponseBytes:      256 << 10,
		MaxOutputTokens:       1024,
		UserRequestsPerMinute: 10,
		IPRequestsPerMinute:   60,
		UserConcurrency:       2,
		UserDailyQuota:        100,
	}
}

func FromServerConfig(c config.Config) Config {
	cfg := DefaultConfig()
	cfg.ProviderBaseURL = c.LLMProviderBaseURL
	cfg.Model = c.LLMModel
	cfg.APIKey = c.LLMAPIKey
	cfg.AppURL = c.AppURL
	cfg.Timeout = c.LLMTimeout()
	cfg.MaxBodyBytes = c.LLMMaxBodyBytes
	cfg.MaxPromptChars = c.LLMMaxPromptChars
	cfg.MaxContextBlocks = c.LLMMaxContextBlocks
	cfg.MaxContextBytes = c.LLMMaxContextBytes
	cfg.MaxResponseBytes = c.LLMMaxResponseBytes
	cfg.MaxOutputTokens = c.LLMMaxOutputTokens
	cfg.UserRequestsPerMinute = c.LLMUserRatePerMinute
	cfg.IPRequestsPerMinute = c.LLMIPRatePerMinute
	cfg.UserConcurrency = c.LLMUserConcurrency
	cfg.UserDailyQuota = c.LLMUserDailyQuota
	return cfg.withDefaults()
}

func (c Config) withDefaults() Config {
	defaults := DefaultConfig()
	if c.Timeout <= 0 {
		c.Timeout = defaults.Timeout
	}
	if c.MaxBodyBytes <= 0 {
		c.MaxBodyBytes = defaults.MaxBodyBytes
	}
	if c.MaxPromptChars <= 0 {
		c.MaxPromptChars = defaults.MaxPromptChars
	}
	if c.MaxContextBlocks <= 0 {
		c.MaxContextBlocks = defaults.MaxContextBlocks
	}
	if c.MaxContextBytes <= 0 {
		c.MaxContextBytes = defaults.MaxContextBytes
	}
	if c.MaxResponseBytes <= 0 {
		c.MaxResponseBytes = defaults.MaxResponseBytes
	}
	if c.MaxOutputTokens <= 0 {
		c.MaxOutputTokens = defaults.MaxOutputTokens
	}
	if c.UserRequestsPerMinute <= 0 {
		c.UserRequestsPerMinute = defaults.UserRequestsPerMinute
	}
	if c.IPRequestsPerMinute <= 0 {
		c.IPRequestsPerMinute = defaults.IPRequestsPerMinute
	}
	if c.UserConcurrency <= 0 {
		c.UserConcurrency = defaults.UserConcurrency
	}
	if c.UserDailyQuota <= 0 {
		c.UserDailyQuota = defaults.UserDailyQuota
	}
	return c
}

type ContextBlock struct {
	Source string `json:"source"`
	Label  string `json:"label,omitempty"`
	Path   string `json:"path,omitempty"`
	Text   string `json:"text"`
}

type ChatRequest struct {
	Action          string
	Prompt          string
	Contexts        []ContextBlock
	MaxOutputTokens int
	ProviderBaseURL string
}

type ChatResponse struct {
	Model string
	Text  string
}

type Provider interface {
	Chat(context.Context, ChatRequest) (ChatResponse, error)
}

type Handler struct {
	cfg       Config
	provider  Provider
	now       func() time.Time
	requestID func() string
	limits    *limits
}

type Option func(*Handler)

func WithProvider(provider Provider) Option {
	return func(h *Handler) {
		h.provider = provider
	}
}

func WithNow(now func() time.Time) Option {
	return func(h *Handler) {
		h.now = now
	}
}

func WithRequestID(requestID func() string) Option {
	return func(h *Handler) {
		h.requestID = requestID
	}
}

func NewHandler(cfg Config, opts ...Option) *Handler {
	cfg = cfg.withDefaults()
	h := &Handler{
		cfg:       cfg,
		now:       time.Now,
		requestID: newRequestID,
		limits:    newLimits(),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

type statusResponse struct {
	Status    string `json:"status"`
	Available bool   `json:"available"`
	Model     string `json:"model,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type chatResponse struct {
	Status    string `json:"status"`
	RequestID string `json:"requestId"`
	Model     string `json:"model"`
	Text      string `json:"text"`
}

type errorResponse struct {
	Status  string `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuthenticated(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid LLM status request.")
		return
	}

	reason := h.unavailableReason()
	if reason != "" {
		writeJSON(w, http.StatusOK, statusResponse{
			Status:    "ok",
			Available: false,
			Reason:    reason,
		})
		return
	}

	writeJSON(w, http.StatusOK, statusResponse{
		Status:    "ok",
		Available: true,
		Model:     h.cfg.Model,
	})
}

type chatPayload struct {
	Action          string         `json:"action"`
	Prompt          string         `json:"prompt"`
	Contexts        []ContextBlock `json:"contexts"`
	ClientRequestID string         `json:"clientRequestId,omitempty"`
}

func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid LLM chat request.")
		return
	}
	if !h.trustedOrigin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "The request origin is not allowed.")
		return
	}
	if reason := h.unavailableReason(); reason != "" {
		writeError(w, http.StatusBadGateway, "unavailable", "LLM assistance is not available.")
		return
	}

	payload, ok := h.decodePayload(w, r)
	if !ok {
		return
	}
	if !h.validatePayload(w, payload) {
		return
	}

	release, ok := h.limits.reserve(userID, clientIP(r), h.now(), h.cfg)
	if !ok {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "LLM request limit reached.")
		return
	}
	defer release()

	provider, err := h.providerClient()
	if err != nil {
		writeError(w, http.StatusBadGateway, "provider_error", "LLM provider request failed.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.Timeout)
	defer cancel()

	resp, err := provider.Chat(ctx, ChatRequest{
		Action:          payload.Action,
		Prompt:          payload.Prompt,
		Contexts:        payload.Contexts,
		MaxOutputTokens: h.cfg.MaxOutputTokens,
		ProviderBaseURL: h.cfg.ProviderBaseURL,
	})
	if err != nil {
		if isTimeout(err) {
			writeError(w, http.StatusGatewayTimeout, "provider_timeout", "LLM provider timed out.")
			return
		}
		writeError(w, http.StatusBadGateway, "provider_error", "LLM provider request failed.")
		return
	}
	if int64(len(resp.Text)) > h.cfg.MaxResponseBytes {
		writeError(w, http.StatusBadGateway, "provider_error", "LLM provider request failed.")
		return
	}

	writeJSON(w, http.StatusOK, chatResponse{
		Status:    "ok",
		RequestID: h.requestID(),
		Model:     safeModel(resp.Model, h.cfg.Model),
		Text:      resp.Text,
	})
}

func (h *Handler) requireAuthenticated(w http.ResponseWriter, r *http.Request) bool {
	_, ok := h.userID(w, r)
	return ok
}

func (h *Handler) userID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	userID, ok := r.Context().Value(UserIDContextKey).(int64)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication is required.")
		return 0, false
	}
	return userID, true
}

func (h *Handler) unavailableReason() string {
	if strings.TrimSpace(h.cfg.ProviderBaseURL) == "" ||
		strings.TrimSpace(h.cfg.Model) == "" ||
		strings.TrimSpace(h.cfg.APIKey) == "" {
		return "not_configured"
	}
	if err := validateProviderURL(h.cfg.ProviderBaseURL); err != nil {
		return "invalid_provider_url"
	}
	return ""
}

func (h *Handler) decodePayload(w http.ResponseWriter, r *http.Request) (chatPayload, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, h.cfg.MaxBodyBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid LLM request body.")
		return chatPayload{}, false
	}
	if int64(len(body)) > h.cfg.MaxBodyBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "LLM request is too large.")
		return chatPayload{}, false
	}
	var payload chatPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid LLM request body.")
		return chatPayload{}, false
	}
	return payload, true
}

func (h *Handler) validatePayload(w http.ResponseWriter, payload chatPayload) bool {
	if !validAction(payload.Action) || strings.TrimSpace(payload.Prompt) == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid LLM request.")
		return false
	}
	if len([]rune(payload.Prompt)) > h.cfg.MaxPromptChars {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "LLM request is too large.")
		return false
	}
	if len(payload.Contexts) > h.cfg.MaxContextBlocks {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "LLM request is too large.")
		return false
	}
	var totalContextBytes int64
	for _, ctx := range payload.Contexts {
		if ctx.Source != "selected-chat" && ctx.Source != "current-file" {
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid LLM context.")
			return false
		}
		totalContextBytes += int64(len(ctx.Text))
		if totalContextBytes > h.cfg.MaxContextBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "LLM request is too large.")
			return false
		}
	}
	return true
}

func validAction(action string) bool {
	switch action {
	case "summarize", "rewrite", "draft", "ask":
		return true
	default:
		return false
	}
}

func (h *Handler) trustedOrigin(r *http.Request) bool {
	appOrigin := originOf(h.cfg.AppURL)
	if appOrigin == "" {
		return false
	}
	if origin := originOf(r.Header.Get("Origin")); origin != "" {
		return origin == appOrigin
	}
	if referer := originOf(r.Header.Get("Referer")); referer != "" {
		return referer == appOrigin
	}
	return false
}

func originOf(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

func (h *Handler) providerClient() (Provider, error) {
	if h.provider != nil {
		return h.provider, nil
	}
	return NewOpenAIClient(h.cfg)
}

func safeModel(providerModel, configuredModel string) string {
	if providerModel != "" {
		return providerModel
	}
	return configuredModel
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{
		Status:  "error",
		Code:    code,
		Message: message,
	})
}

func newRequestID() string {
	return fmt.Sprintf("llm-%d-%08x", time.Now().UnixNano(), rand.Uint32())
}

type captureResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *captureResponseWriter) Header() http.Header {
	return w.header
}

func (w *captureResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *captureResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}

func AuthErrorJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture := &captureResponseWriter{header: make(http.Header)}
		next.ServeHTTP(capture, r)
		status := capture.status
		if status == 0 {
			status = http.StatusOK
		}
		if status == http.StatusUnauthorized {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication is required.")
			return
		}
		if status == http.StatusTooManyRequests {
			writeError(w, http.StatusTooManyRequests, "rate_limited", "Request limit reached.")
			return
		}
		for k, values := range capture.header {
			for _, value := range values {
				w.Header().Add(k, value)
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write(capture.body.Bytes())
	})
}

type windowCounter struct {
	start time.Time
	count int
}

type dailyCounter struct {
	day   string
	count int
}

type limits struct {
	mu      sync.Mutex
	userMin map[int64]windowCounter
	ipMin   map[string]windowCounter
	daily   map[int64]dailyCounter
	active  map[int64]int
}

func newLimits() *limits {
	return &limits{
		userMin: make(map[int64]windowCounter),
		ipMin:   make(map[string]windowCounter),
		daily:   make(map[int64]dailyCounter),
		active:  make(map[int64]int),
	}
}

func (l *limits) reserve(userID int64, ip string, now time.Time, cfg Config) (func(), bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if cfg.UserConcurrency > 0 && l.active[userID] >= cfg.UserConcurrency {
		return nil, false
	}
	if !allowMinute(l.userMin, userID, now, cfg.UserRequestsPerMinute) {
		return nil, false
	}
	if !allowMinute(l.ipMin, ip, now, cfg.IPRequestsPerMinute) {
		return nil, false
	}
	day := now.Format("2006-01-02")
	daily := l.daily[userID]
	if daily.day != day {
		daily = dailyCounter{day: day}
	}
	if cfg.UserDailyQuota > 0 && daily.count >= cfg.UserDailyQuota {
		return nil, false
	}

	incrementMinute(l.userMin, userID, now)
	incrementMinute(l.ipMin, ip, now)
	daily.count++
	l.daily[userID] = daily
	l.active[userID]++

	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.active[userID] > 0 {
			l.active[userID]--
		}
	}, true
}

func allowMinute[K comparable](counters map[K]windowCounter, key K, now time.Time, limit int) bool {
	if limit <= 0 {
		return true
	}
	counter := counters[key]
	if counter.start.IsZero() || now.Sub(counter.start) >= time.Minute {
		return true
	}
	return counter.count < limit
}

func incrementMinute[K comparable](counters map[K]windowCounter, key K, now time.Time) {
	counter := counters[key]
	if counter.start.IsZero() || now.Sub(counter.start) >= time.Minute {
		counter = windowCounter{start: now}
	}
	counter.count++
	counters[key] = counter
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	if r.RemoteAddr == "" {
		return "unknown"
	}
	return r.RemoteAddr
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrProviderTimeout) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
