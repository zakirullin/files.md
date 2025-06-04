// Package webserver provides a server for habits tracking functionality through Telegram miniapps.
// SSLs certificates are handled automatically via LetsEncrypt.
package server

import (
	_ "embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"zakirullin/stuffbot/config"
	"zakirullin/stuffbot/internal/fs"
	"zakirullin/stuffbot/internal/habits"
	"zakirullin/stuffbot/internal/journal"
	"zakirullin/stuffbot/internal/userconfig"
)

// TODO release graceful shutdown etc
func Serve(apiHost, appHost, certDir, logFilename, token string) {
	// TODO fix
	AuthToken = token

	logger := newLogger(logFilename)
	srv := ssl(logger, certDir, apiHost, appHost)
	srv.Handler = newRouter(logger)

	err := srv.ListenAndServeTLS("", "") // Key and cert provided automatically by autocert
	if err != nil {
		panic(err)
	}
}

func newRouter(logger *log.Logger) *http.ServeMux {
	r := http.NewServeMux()
	// TODO add hashing or secrets
	// TODO before release habits_v2 => habits
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Serving the PWA app
		host := r.Host
		if strings.HasPrefix(host, "app.") {
			if r.URL.Path == "" || r.URL.Path == "/" {
				http.ServeFile(w, r, "./web/app.html")
				return
			}

			http.FileServer(http.Dir("./web")).ServeHTTP(w, r)
			return
		}

		http.NotFound(w, r)
	})

	r.HandleFunc("GET /habits_v2/{userID}", func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
		if err != nil {
			logger.Printf("failed to parse userID for habits: %v", err)
			_, _ = w.Write([]byte("can't parse userID"))
		}

		userFS, err := fs.NewUserFS(userID)
		if err != nil {
			logger.Printf("failed to init userFS: %v", err)
			_, _ = w.Write([]byte("can't init userFS"))
		}

		str, err := habits.Render(userID, userFS)
		if err != nil {
			logger.Printf("failed to render habits: %v", err)
			_, _ = w.Write([]byte(err.Error()))
		}
		_, err = w.Write(str)
		if err != nil {
			logger.Printf("failed to write habits response: %v", err)
		}
	})

	r.HandleFunc("POST /habits_v2/{userID}/{habitName}/{yearDay}/{status}", func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
		if err != nil {
			logger.Printf("failed to parse userID: %v", err)
			_, _ = w.Write([]byte("can't parse userID"))
		}

		yearDay, err := strconv.ParseInt(r.PathValue("yearDay"), 10, 32)
		if err != nil {
			logger.Printf("failed to parse yearDay: %v", err)
			_, _ = w.Write([]byte("can't parse yearDay"))
		}

		status, err := strconv.ParseInt(r.PathValue("status"), 10, 32)
		if err != nil {
			logger.Printf("failed to parse status: %v", err)
			_, _ = w.Write([]byte("can't parse status"))
		}

		habitName := r.PathValue("habitName")

		userFS, err := fs.NewUserFS(userID)
		if err != nil {
			logger.Printf("failed to init user fs: %v", err)
			_, _ = w.Write([]byte("can't init user fs"))
		}

		userHabits, err := habits.Habits(userFS, time.Now().Year())
		if err != nil {
			logger.Printf("failed to read habits: %v", err)
			_, _ = w.Write([]byte("can't read habits"))
		}

		if _, ok := userHabits[habitName]; !ok {
			userHabits[habitName] = make(habits.Year)
		}
		userHabits[habitName][int(yearDay)] = int(status)
		err = habits.Write(userFS, time.Now().Year(), userHabits)
		if err != nil {
			logger.Printf("failed to write habits: %v", err)
			_, _ = w.Write([]byte("can't write habits"))
		}

		emoji := habits.Emoji(userFS, habitName)
		if habitName == habits.MoodHabit {
			if int(status) < len(habits.MoodEmojis) {
				emoji = habits.MoodEmojis[status]
			}
		}

		userConf := userconfig.NewConfig(userFS, userID, config.BotCfg.ConfigFilename)
		err = journal.AddEmoji(userFS, emoji, userConf.Timezone())
		if err != nil {
			logger.Printf("failed to write habit emoji to journal: %v", err)
			_, _ = w.Write([]byte("can't write habit emoji to journal"))
		}

		record := fmt.Sprintf("%s %s", emoji, habitName)
		err = journal.AddRecord(userFS, record, userConf.Timezone())
		if err != nil {
			logger.Printf("failed to write habit to journal: %v", err)
			_, _ = w.Write([]byte("can't write habit to journal"))
		}
	})

	r.HandleFunc("/syncTexts", corsMiddleware(authMiddleware(SyncTexts)))
	r.HandleFunc("/syncText", corsMiddleware(authMiddleware(SyncText)))
	r.HandleFunc("/syncMedias", corsMiddleware(authMiddleware(SyncAllMedia)))
	r.HandleFunc("/syncMedia", corsMiddleware(authMiddleware(SyncMedia)))

	return r
}

func newLogger(logFilename string) *log.Logger {
	logFile, err := os.OpenFile(logFilename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		log.Fatalf("Server: failed to open log file: %v", err)
	}
	defer logFile.Close()

	return log.New(logFile, "Server Error: ", log.Ldate|log.Ltime|log.Lshortfile)
}
