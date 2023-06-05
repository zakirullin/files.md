package main

import (
	"fmt"
	"os"
	"time"

	"github.com/alicebob/miniredis/v2"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"github.com/spf13/afero"
	"golang.org/x/exp/slog"

	"zakirullin/dumpbot/internal"
	"zakirullin/dumpbot/internal/db"
	"zakirullin/dumpbot/internal/fs"
	"zakirullin/dumpbot/internal/i18n"
	"zakirullin/dumpbot/internal/sched"
	"zakirullin/dumpbot/pkg/tg"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr))
	slog.SetDefault(log)

	err := godotenv.Load()
	if err != nil {
		panic(fmt.Sprintf("Error loading .env file: %s\n", err))
	}

	err = i18n.LoadLangFile("assets/i18n/ru.json")
	if err != nil {
		panic(fmt.Sprintf("Error loading i18n: %s\n", err))
	}
	err = i18n.LoadEmojiFile("assets/emoji.json")
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
		for {
			select {
			case <-ticker.C:
				moveDueTasksToToday(redis, tg)
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
					fmt.Printf("Bot's panic: %s\n", err)
				}
			}()

			u := tg.NewUpd(upd)
			userID := u.UserID()
			fsys, err := fs.NewFS(userID, afero.NewOsFs())
			if err != nil {
				fmt.Printf("Bot's error: can't create FS: %s\n", err)
				return
			}
			bot := internal.NewBot(userID, telegram, fsys, db.NewDB(redis))
			if err := bot.Reply(u); err != nil {
				fmt.Printf("Bot's error: %s\n", err)
			}
		}(upd)
	}
}

func moveDueTasksToToday(redis *miniredis.Miniredis, tg *tg.TG) {
	ids, err := fs.AllUserIDs()
	if err != nil {
		fmt.Printf("moveDueTasksForToday: %s\n", err)
	}

	for _, id := range ids {
		database := db.NewDB(redis)
		sch, err := database.Schedule(id)
		if err != nil {
			fmt.Printf("moveDueTasksForToday: can't get sch: %s", err)
			return
		}

		fsys, err := fs.NewFS(id, afero.NewOsFs())
		if err != nil {
			fmt.Printf("moveDueTasksForToday: can't create FS: %s", err)
			return
		}
		for filename, cron := range sch {
			if time.Now().Unix() >= cron.RunAt {
				err = moveTaskToToday(filename, fsys)
				if err != nil {
					slog.Error(fmt.Sprintf("moveDueTasksForToday: can't move: %s", err))
				}

				if len(cron.Cron) != 0 {
					err = database.AddToSchedule(id, filename, sched.Next(cron.Cron), cron.Cron)
					if err != nil {
						fmt.Printf("err")
					}

					continue
				}

				err = database.DelFromSchedule(id, filename)
				if err != nil {
					fmt.Printf("err")
				}
			}
		}
	}
}

func moveTaskToToday(filename string, fsys *fs.FS) error {
	dirsToLookFor := []string{fs.DirLater, fs.DirTrash}
	for _, dir := range dirsToLookFor {
		filenames, err := fsys.FilesAndDirs(dir)
		fmt.Printf("%v\n", filenames)
		if err != nil {
			return fmt.Errorf("moveTaskForToday: %w", err)
		}

		for _, f := range filenames {
			if f.Name == filename {
				err = fsys.Rename(dir, filename, fs.DirToday, filename)
				if err != nil {
					return fmt.Errorf("moveTaskForToday: can't rename: %w", err)
				}
			}
		}
	}

	return nil
}
