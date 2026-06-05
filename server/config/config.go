package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// APIURL / AppURL carry the full scheme+host (e.g. "https://api.files.md").
// Hostnames are derived from them on demand via APIHost()/AppHost().
type Config struct {
	WorkingDir        string
	StorageDir        string `default:"./storage"  envconfig:"STORAGE_DIR"`
	BotAPIToken       string `required:"false" envconfig:"BOT_API_TOKEN"`
	ConfigFilename    string `default:"config.json"`
	APIURL            string `default:"" envconfig:"API_URL"`
	AppURL            string `default:"" envconfig:"APP_URL"`
	ServerCertDir     string `default:"/tmp" envconfig:"CERT_DIR"`
	TokensDir         string `default:"/tmp" envconfig:"TOKENS_DIR"`
	TokensSalt        string `envconfig:"TOKENS_SALT"`
	ServerLogFile     string `default:"/tmp/server.log" envconfig:"LOG_FILE"`
	StorageQuotaKB    int64  `default:"1024" envconfig:"STORAGE_QUOTA_KB"` // 1MB
	UnlimitedQuotaIDs string `envconfig:"UNLIMITED_QUOTA_IDS"`

	LLMProviderBaseURL   string `envconfig:"LLM_PROVIDER_BASE_URL"`
	LLMModel             string `envconfig:"LLM_MODEL"`
	LLMAPIKey            string `envconfig:"LLM_API_KEY"`
	LLMTimeoutSeconds    int    `default:"30" envconfig:"LLM_TIMEOUT_SECONDS"`
	LLMMaxBodyBytes      int64  `default:"262144" envconfig:"LLM_MAX_BODY_BYTES"`
	LLMMaxPromptChars    int    `default:"8000" envconfig:"LLM_MAX_PROMPT_CHARS"`
	LLMMaxContextBlocks  int    `default:"8" envconfig:"LLM_MAX_CONTEXT_BLOCKS"`
	LLMMaxContextBytes   int64  `default:"131072" envconfig:"LLM_MAX_CONTEXT_BYTES"`
	LLMMaxResponseBytes  int64  `default:"262144" envconfig:"LLM_MAX_RESPONSE_BYTES"`
	LLMMaxOutputTokens   int    `default:"1024" envconfig:"LLM_MAX_OUTPUT_TOKENS"`
	LLMUserRatePerMinute int    `default:"10" envconfig:"LLM_USER_RATE_PER_MINUTE"`
	LLMIPRatePerMinute   int    `default:"60" envconfig:"LLM_IP_RATE_PER_MINUTE"`
	LLMUserConcurrency   int    `default:"2" envconfig:"LLM_USER_CONCURRENCY"`
	LLMUserDailyQuota    int    `default:"100" envconfig:"LLM_USER_DAILY_QUOTA"`
}

func (c Config) APIHost() string { return hostOf(c.APIURL) }
func (c Config) AppHost() string { return hostOf(c.AppURL) }
func (c Config) LLMTimeout() time.Duration {
	if c.LLMTimeoutSeconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.LLMTimeoutSeconds) * time.Second
}

func hostOf(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}

var ServerCfg Config

func LoadBotConfig() error {
	if err := envconfig.Process("", &ServerCfg); err != nil {
		return fmt.Errorf("can't load config: %w", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("config can't get working directory: %w", err)
	}
	ServerCfg.WorkingDir = wd

	if !filepath.IsAbs(ServerCfg.StorageDir) {
		ServerCfg.StorageDir = filepath.Join(wd, ServerCfg.StorageDir)
	}

	return nil
}
