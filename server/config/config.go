package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

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
	CertFile          string `default:"" envconfig:"CERT_FILE"`
	KeyFile           string `default:"" envconfig:"KEY_FILE"`
	TokensDir         string `default:"/tmp" envconfig:"TOKENS_DIR"`
	TokensSalt        string `envconfig:"TOKENS_SALT"`
	ServerLogFile     string `default:"/tmp/server.log" envconfig:"LOG_FILE"`
	StorageQuotaKB    int64  `default:"1024" envconfig:"STORAGE_QUOTA_KB"` // 1MB
	UnlimitedQuotaIDs string `envconfig:"UNLIMITED_QUOTA_IDS"`
}

func (c Config) APIHost() string { return hostOf(c.APIURL) }
func (c Config) AppHost() string { return hostOf(c.AppURL) }

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
