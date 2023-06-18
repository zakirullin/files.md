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

	"zakirullin/dumpbot/i18n"
	"zakirullin/dumpbot/internal"
	"zakirullin/dumpbot/internal/db"
	"zakirullin/dumpbot/internal/fs"
	"zakirullin/dumpbot/internal/sched/worker"
	"zakirullin/dumpbot/internal/userconfig"
	"zakirullin/dumpbot/pkg/tg"
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

	err = i18n.LoadLangFile("i18n/ru.json")
	if err != nil {
		panic(fmt.Sprintf("Error loading i18n: %s\n", err))
	}
	err = i18n.LoadEmojiFile("i18n/emojis.json")
	if err != nil {
		panic(fmt.Sprintf("Error loading emoji: %s\n", err))
	}

	api, err := tgbotapi.NewBotAPI(os.Getenv("BOT_API_TOKEN"))
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
				err := worker.MoveDueTasksToToday(redis, fsBackend)
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
	config := tgbotapi.NewUpdate(0)
	config.Timeout = 60
	updates := api.GetUpdatesChan(config)
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
			fsys, err := fs.NewFS(userID, afero.NewOsFs())
			if err != nil {
				slog.Error("Bot error: can't create FS", "err", err)
				return
			}

			conf := userconfig.NewConfig()
			// TODO paths to envs
			configPath := "cmd/testdata/config.json"
			err = conf.LoadOrCreate(configPath)
			if err != nil {
				slog.Error("Bot error: can't get or create config", "err", err)
				return
			}
			defer conf.Save(configPath)

			bot := internal.NewBot(userID, telegram, fsys, db.NewDB(redis), conf)

			if err := bot.Reply(u); err != nil {
				slog.Error("Bot error", "err", err)
			}
		}(upd)
	}
}
