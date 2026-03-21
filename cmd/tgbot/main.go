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

	bot "zakirullin/stuffbot/bot"
	"zakirullin/stuffbot/config"
	"zakirullin/stuffbot/i18n"
	"zakirullin/stuffbot/bot/db"
	"zakirullin/stuffbot/bot/fs"
	"zakirullin/stuffbot/bot/sched/worker"
	"zakirullin/stuffbot/bot/server"
	"zakirullin/stuffbot/bot/userconfig"
	"zakirullin/stuffbot/pkg/tg"
	"zakirullin/stuffbot/pkg/txt"
)

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
		panic(fmt.Sprintf("Can't create api: %s\n", err))
	}
	telegram := tg.NewTG(api)

	// Save all renames and deletes to an append-only log.
	fs.LogRename = server.LogRename
	fs.LogDelete = server.LogDelete
	// If today or inbox was changed in web app, we need to send the updated items to the bot.
	server.OnTodayUpdate = func(userID int64) { updateToday(telegram, userID) }

	// Due tasks scheduler
	ticker := time.NewTicker(5 * time.Second)
	quit := make(chan struct{})
	defer func(quit chan struct{}) {
		close(quit)
	}(quit)
	go func(tg *tg.TG) {
		fsBackend := afero.NewOsFs()
		for {
			select {
			case <-ticker.C:
				err := worker.MoveDueTasks(config.BotCfg.StorageDir, config.BotCfg.ConfigFilename, fsBackend, telegram)
				if err != nil {
					fmt.Printf("Worker's error: %s\n", err)
				}

				err = worker.RemoveCompletedChecklistItems(config.BotCfg.StorageDir, config.BotCfg.ConfigFilename, fsBackend)
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
	if config.BotCfg.ApiHost != "" {
		go server.Serve(
			config.BotCfg.ApiHost,
			config.BotCfg.AppHost,
			config.BotCfg.ServerCertDir,
			config.BotCfg.ServerLogFile,
		)
	}

	infolog := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Main bot loop.
	// Loop through updates from Telegram and process them sequentially in separate per-user goroutine.
	userChannels := make(map[int64]chan tgbotapi.Update)
	tgConfig := tgbotapi.NewUpdate(0)
	tgConfig.Timeout = 60 // TODO release, check if it's enough
	updates := api.GetUpdatesChan(tgConfig)
	for update := range updates {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("Bot panic", "update", update, "err", err, "stacktrace", string(debug.Stack()))
				}
			}()

			updJSON, _ := json.Marshal(update)
			infolog.Info("Bot update", "update", string(updJSON))

			var userID int64
			upd := tg.NewTGUpd(update)
			channelID, channelIDExists := upd.ChannelID()
			if channelIDExists {
				userID, err = telegram.ChannelCreatorID(channelID)
				if err != nil {
					slog.Error("Bot error: can't get channel creator ID", "upd", string(updJSON), "err", err)
				}
			} else {
				userID = upd.UserID()
			}

			userCh, channelIDExists := userChannels[userID]
			if !channelIDExists {
				userCh = make(chan tgbotapi.Update, 100)
				userChannels[userID] = userCh
				go supervisor(userID, userCh, telegram)
			}

			userCh <- update
		}()
	}
}

// Runs per-user worker that listens for updates.
// Restarts infinitely upon panics.
func supervisor(userID int64, updates <-chan tgbotapi.Update, telegram *tg.TG) {
	for {
		func() {
			defer func() {
				if err := recover(); err != nil {
					slog.Error("Bot panic", "userID", userID, "err", err, "stacktrace", string(debug.Stack()))
				}
			}()
			processUserUpdates(userID, updates, telegram)
		}()
		time.Sleep(time.Second)
		slog.Info("Restarting worker", "userID", userID)
	}
}

func processUserUpdates(userID int64, updates <-chan tgbotapi.Update, telegram *tg.TG) {
	for update := range updates {
		upd := tg.NewTGUpd(update)

		bot, err := newBot(telegram, userID)
		if err != nil {
			slog.Error("Bot error: can't create bot", "err", err)
			return
		}

		if err := bot.Reply(upd); err != nil {
			slog.Error("Bot error", "err", err)
		}
	}
}

func newBot(telegram *tg.TG, userID int64) (*bot.Bot, error) {
	storagePath := config.BotCfg.StorageDir
	storagePath, err := filepath.Abs(storagePath)
	userPath := path.Join(storagePath, txt.I64(userID))
	userFS, err := fs.NewFS(userPath, afero.NewOsFs())
	if err != nil {
		return nil, fmt.Errorf("can't create fs: %w", err)
	}
	err = userFS.CreateDirsIfNotExist()
	if err != nil {
		return nil, fmt.Errorf("can't create user dirs: %w", err)
	}

	confFilename := config.BotCfg.ConfigFilename
	userconf := userconfig.NewConfig(userFS, userID, confFilename)
	err = userconf.CreateDefaultIfNotExists()
	if err != nil {
		return nil, fmt.Errorf("can't create default user config: %w", err)
	}

	b := bot.NewBot(userID, telegram, userFS, db.NewDB(userID), userconf)

	return b, nil
}

func updateToday(telegram *tg.TG, userID int64) {
	bot, err := newBot(telegram, userID)
	if err != nil {
		slog.Error("Bot error: can't create bot", "err", err)
		return
	}

	err = bot.ShowToday(nil)
	if err != nil {
		slog.Error("Bot error: can't update today", "userID", userID, "err", err)
	}
}
