package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

var ErrProviderTimeout = errors.New("provider_timeout")

type OpenAIClient struct {
	cfg      Config
	endpoint string
	client   *http.Client
}

func NewOpenAIClient(cfg Config) (*OpenAIClient, error) {
	cfg = cfg.withDefaults()
	if err := validateProviderURL(cfg.ProviderBaseURL); err != nil {
		return nil, err
	}
	endpoint, err := chatCompletionsEndpoint(cfg.ProviderBaseURL)
	if err != nil {
		return nil, err
	}

	return &OpenAIClient{
		cfg:      cfg,
		endpoint: endpoint,
		client: &http.Client{
			Timeout: cfg.Timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *OpenAIClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	body, err := json.Marshal(openAIRequest{
		Model:     c.cfg.Model,
		Messages:  providerMessages(req),
		MaxTokens: req.MaxOutputTokens,
		Stream:    false,
	})
	if err != nil {
		return ChatResponse{}, fmt.Errorf("provider_request_encode")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("provider_request_build")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		if isTimeout(err) {
			return ChatResponse{}, ErrProviderTimeout
		}
		return ChatResponse{}, fmt.Errorf("provider_request_failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ChatResponse{}, fmt.Errorf("provider_status_%d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, c.cfg.MaxResponseBytes+1))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("provider_response_read")
	}
	if int64(len(data)) > c.cfg.MaxResponseBytes {
		return ChatResponse{}, fmt.Errorf("response_too_large")
	}

	var decoded openAIResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return ChatResponse{}, fmt.Errorf("provider_response_decode")
	}
	if len(decoded.Choices) == 0 || decoded.Choices[0].Message.Content == "" {
		return ChatResponse{}, fmt.Errorf("provider_response_empty")
	}

	return ChatResponse{
		Model: decoded.Model,
		Text:  decoded.Choices[0].Message.Content,
	}, nil
}

type openAIRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens"`
	Stream    bool            `json:"stream"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func providerMessages(req ChatRequest) []openAIMessage {
	var b strings.Builder
	b.WriteString("Action: ")
	b.WriteString(req.Action)
	b.WriteString("\n\nPrompt:\n")
	b.WriteString(req.Prompt)
	for i, ctx := range req.Contexts {
		b.WriteString("\n\nContext ")
		b.WriteString(fmt.Sprintf("%d", i+1))
		b.WriteString(" (source: ")
		b.WriteString(ctx.Source)
		if ctx.Label != "" {
			b.WriteString(", label: ")
			b.WriteString(ctx.Label)
		}
		if ctx.Path != "" {
			b.WriteString(", path: ")
			b.WriteString(ctx.Path)
		}
		b.WriteString("):\n")
		b.WriteString(ctx.Text)
	}

	return []openAIMessage{
		{
			Role:    "system",
			Content: "You are an assistant inside Files.md. Return concise draft Markdown only. Do not mutate files.",
		},
		{
			Role:    "user",
			Content: b.String(),
		},
	}
}

func validateProviderURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid_provider_url")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if isLocalOrPrivateHost(u.Hostname()) {
			return nil
		}
		return fmt.Errorf("invalid_provider_url")
	default:
		return fmt.Errorf("invalid_provider_url")
	}
}

func chatCompletionsEndpoint(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid_provider_url")
	}
	path := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(path, "/chat/completions") {
		path += "/chat/completions"
	}
	u.Path = path
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func isLocalOrPrivateHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
