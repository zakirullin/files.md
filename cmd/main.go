package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/alicebob/miniredis/v2"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"github.com/spf13/afero"
	"golang.org/x/exp/slog"

	"zakirullin/stuffbot/i18n"
	"zakirullin/stuffbot/internal"
	"zakirullin/stuffbot/internal/db"
	"zakirullin/stuffbot/internal/fs"
	"zakirullin/stuffbot/internal/sched/worker"
	"zakirullin/stuffbot/internal/userconfig"
	"zakirullin/stuffbot/pkg/tg"
)

func main() {
	opts := slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	logger := slog.New(opts.NewTextHandler(os.Stderr))
	slog.SetDefault(logger)

	err := godotenv.Load()
	if err != nil {
		panic(fmt.Sprintf("Error loading .env file: %s\n", err))
	}
	conf, err := internal.LoadConfig()
	if err != nil {
		panic(fmt.Sprintf("Error loading conf: %s\n", err))
	}

	// TODO move to config
	err = i18n.LoadLangFile("i18n/ru.json")
	if err != nil {
		panic(fmt.Sprintf("Error loading i18n: %s\n", err))
	}
	// TODO move to config
	err = i18n.LoadEmojiFile("i18n/emojis.json")
	if err != nil {
		panic(fmt.Sprintf("Error loading emoji: %s\n", err))
	}

	api, err := tgbotapi.NewBotAPI(conf.BotAPIToken)
	if err != nil {
		panic(fmt.Sprintf("Can't create TG api: %s\n", err))
	}
	telegram := tg.NewTG(api)

	redis, err := miniredis.Run()
	if err != nil {
		panic(fmt.Sprintf("Can't create Redis: %s\n", err))
	}
	defer redis.Close()

	// Workers
	ticker := time.NewTicker(5 * time.Second)
	quit := make(chan struct{})
	defer func(quit chan struct{}) {
		close(quit)
	}(quit)

	go func(redis *miniredis.Miniredis, tg *tg.TG) {
		fsBackend := afero.NewOsFs()
		for {
			select {
			case <-ticker.C:
				err := worker.MoveDueTasksToToday(conf.StoragePath, conf.ConfigFilename, fsBackend)
				if err != nil {
					fmt.Printf("Worker's error: %s\n", err)
				}
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}(redis, telegram)

	// Service
	tgConfig := tgbotapi.NewUpdate(0)
	tgConfig.Timeout = 60
	updates := api.GetUpdatesChan(tgConfig)
	for upd := range updates {
		go func(upd tgbotapi.Update) {
			defer func() {
				err := recover()
				if err != nil {
					slog.Error("Bot panic", "err", err)
				}
			}()

			var updJSON []byte
			updJSON, _ = json.Marshal(upd)
			slog.Debug("Bot update: ", "upd", updJSON)

			u := tg.NewUpd(upd)
			userID := u.UserID()
			userPath := fs.UserPath(conf.StoragePath, userID)
			userFS, err := fs.NewFS(userPath, afero.NewOsFs())
			if err != nil {
				slog.Error("Bot error: can't create fs", "err", err)
				return
			}
			err = userFS.CreateUserDirs()
			if err != nil {
				slog.Error("Bot error: can't create user dirs", "err", err)
				return
			}

			userconf := userconfig.NewConfig()
			userconfPath := userFS.Path("", conf.ConfigFilename)
			err = userconf.LoadOrCreate(userconfPath)
			if err != nil {
				slog.Error("Bot error: can't get or create conf", "err", err)
				return
			}
			defer func() {
				err = userconf.Save(userconfPath)
				slog.Error("Bot error: can't save userconfig", "err", err)
			}()

			bot := internal.NewBot(userID, telegram, userFS, db.NewDB(redis), userconf)

			if err := bot.Reply(u); err != nil {
				slog.Error("Bot error", "err", err)
			}
		}(upd)
	}
}
