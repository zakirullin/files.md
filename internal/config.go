package internal

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"

	"zakirullin/stuffbot/internal/fs"
)

type Config struct {
	StoragePath    string `required:"true" envconfig:"STORAGE_PATH"`
	BotAPIToken    string `required:"true" envconfig:"BOT_API_TOKEN"`
	AdminUserID    string `required:"true" envconfig:"ADMIN_USER_ID"`
	ConfigFilename string `default:"config.json"`
}

func LoadConfig() (Config, error) {
	var cfg Config

	if err := envconfig.Process("", &cfg); err != nil {
		return Config{}, fmt.Errorf("can't load config: %w", err)
	}

	return cfg, nil
}

func shouldSplitChecklist(checklist string) bool {
	for _, unsplittableChecklist := range []string{fs.DirRead, fs.DirWatch} {
		if checklist == unsplittableChecklist {
			return false
		}
	}
	return true
}
