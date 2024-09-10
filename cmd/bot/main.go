package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
	"github.com/spf13/afero"
	"golang.org/x/exp/slog"

	"zakirullin/stuffbot/config"
	"zakirullin/stuffbot/i18n"
	"zakirullin/stuffbot/internal"
	"zakirullin/stuffbot/internal/db"
	"zakirullin/stuffbot/internal/fs"
	"zakirullin/stuffbot/internal/sched/worker"
	"zakirullin/stuffbot/internal/userconfig"
	"zakirullin/stuffbot/pkg/tg"
	"zakirullin/stuffbot/pkg/txt"
)

func main() {
	opts := &tint.Options{
		Level: slog.LevelDebug,
	}
	logger := slog.New(tint.NewHandler(os.Stderr, opts))
	slog.SetDefault(logger)

	err := godotenv.Load()
	if err != nil {
		panic(fmt.Sprintf("Error loading .env file: %s\n", err))
	}
	err = config.LoadBotConfig()
	if err != nil {
		panic(fmt.Sprintf("Error loading cfg: %s\n", err))
	}

	// TODO move to embed
	err = i18n.LoadLangFile("i18n/ru.json")
	if err != nil {
		panic(fmt.Sprintf("Error loading i18n: %s\n", err))
	}

	api, err := tgbotapi.NewBotAPI(config.BotCfg.BotAPIToken)
	if err != nil {
		panic(fmt.Sprintf("Can't create FakeTG api: %s\n", err))
	}
	telegram := tg.NewTG(api)

	// Workers
	ticker := time.NewTicker(5 * time.Second)
	quit := make(chan struct{})
	defer func(quit chan struct{}) {
		close(quit)
	}(quit)

	// Due tasks scheduler
	go func(tg *tg.TG) {
		fsBackend := afero.NewOsFs()
		for {
			select {
			case <-ticker.C:
				err := worker.MoveDueTasks(config.BotCfg.StoragePath, config.BotCfg.ConfigFilename, fsBackend, telegram)
				if err != nil {
					fmt.Printf("Worker's error: %s\n", err)
				}
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}(telegram)

	// Launch habits server if needed
	if config.BotCfg.HabitsHost != "" {
		go habitsServer(config.BotCfg.HabitsHost, config.BotCfg.HabitsCertsPath)
	}

	// Main bot loop.
	// Listen for updates from user and process them in separate goroutines.
	tgConfig := tgbotapi.NewUpdate(0)
	tgConfig.Timeout = 60 // TODO release, check if it's enough
	updates := api.GetUpdatesChan(tgConfig)

	for upd := range updates {
		go func(upd tgbotapi.Update) {
			defer func() {
				err := recover()
				if err != nil {
					slog.Error("Bot panic", "err", err)
				}
			}()

			u := tg.NewUpd(upd)
			userID := u.UserID()

			var updJSON []byte
			updJSON, _ = json.Marshal(upd)
			slog.Debug("Bot update: ", "upd", string(updJSON))

			userPath := path.Join(config.BotCfg.StoragePath, txt.I64(userID))
			userFS, err := fs.NewFS(userPath, afero.NewOsFs())
			if err != nil {
				slog.Error("Bot error: can't create fs", "err", err)
				return
			}
			err = userFS.CreateDirsIfNotExist()
			if err != nil {
				slog.Error("Bot error: can't create user dirs", "err", err)
				return
			}

			confFilename := config.BotCfg.ConfigFilename
			userconf := userconfig.NewConfig(userFS, userID, confFilename)
			err = userconf.CreateDefaultIfNotExists()
			if err != nil {
				slog.Error("Bot error: can't create default user config", "err", err)
				return
			}

			bot := internal.NewBot(userID, telegram, userFS, db.NewDB(), userconf)
			if err := bot.Answer(u); err != nil {
				slog.Error("Bot error", "err", err)
			}
		}(upd)
	}
}
