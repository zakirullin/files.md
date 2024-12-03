package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"github.com/spf13/afero"

	"zakirullin/stuffbot/config"
	"zakirullin/stuffbot/i18n"
	"zakirullin/stuffbot/internal"
	"zakirullin/stuffbot/internal/db"
	"zakirullin/stuffbot/internal/fs"
	"zakirullin/stuffbot/internal/sched/worker"
	"zakirullin/stuffbot/internal/userconfig"
	"zakirullin/stuffbot/internal/web"
	"zakirullin/stuffbot/pkg/tg"
	"zakirullin/stuffbot/pkg/txt"
)

func processUserUpdates(updates <-chan tgbotapi.Update, telegram *tg.TG, infolog *slog.Logger) {
	for update := range updates {
		upd := tg.NewTGUpd(update)
		userID := upd.UserID()

		var updJSON []byte
		updJSON, _ = json.Marshal(update)
		infolog.Info("Bot update: ", "update", string(updJSON))

		storagePath := config.BotCfg.StorageDir
		storagePath, err := filepath.Abs(storagePath)
		userPath := path.Join(storagePath, txt.I64(userID))
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

		bot := internal.NewBot(userID, telegram, userFS, db.NewDB(userID), userconf)
		if err := bot.Answer(upd); err != nil {
			slog.Error("Bot error", "err", err)
		}
	}
}

func main() {
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
				err := worker.MoveDueTasks(config.BotCfg.StorageDir, config.BotCfg.ConfigFilename, fsBackend, telegram)
				if err != nil {
					fmt.Printf("Worker's error: %s\n", err)
				}
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}(telegram)

	// TODO apphost?
	// Launch habits server if needed
	if config.BotCfg.HabitsHost != "" {
		go web.Serve(
			config.BotCfg.HabitsHost,
			config.BotCfg.AppHost,
			config.BotCfg.ServerCertDir,
			config.BotCfg.ServerLogFile,
		)
	}

	infolog := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Main bot loop.
	// Loop through updates from Telegram and process them in separate per-user goroutines.
	userChannels := make(map[int64]chan tgbotapi.Update)
	tgConfig := tgbotapi.NewUpdate(0)
	tgConfig.Timeout = 60 // TODO release, check if it's enough
	updates := api.GetUpdatesChan(tgConfig)
	for update := range updates {
		upd := tg.NewTGUpd(update)
		userID := upd.UserID()

		userCh, exists := userChannels[userID]
		if !exists {
			userCh = make(chan tgbotapi.Update, 100)
			userChannels[userID] = userCh
			// Start per-user worker if none is running
			go func() {
				defer func() {
					err := recover()
					if err != nil {
						slog.Error("Bot panic", "err", err, "stacktrace", string(debug.Stack()))
					}
				}()

				processUserUpdates(userCh, telegram, infolog)
			}()
		}

		userCh <- update
	}
}
