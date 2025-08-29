// Package webserver provides a server for habits tracking functionality through Telegram miniapps.
// SSLs certificates are handled automatically via LetsEncrypt.
package server

import (
	_ "embed"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"zakirullin/stuffbot/config"
	"zakirullin/stuffbot/internal/fs"
	"zakirullin/stuffbot/internal/habits"
	"zakirullin/stuffbot/internal/journal"
	"zakirullin/stuffbot/internal/userconfig"
)

// Serve TODO release graceful shutdown etc
// All directories paths are absolute.
func Serve(apiHost, appHost, certDir, logFilename, token, tokensDir string) {
	err := os.Setenv("GODEBUG", "http2debug=1")
	if err != nil {
		panic(err)
	}

	// Logger is used for ssl/connection errors.
	// For regular errors we still use slog.
	logger := newLogger(logFilename)
	srv := ssl(logger, certDir, apiHost, appHost)
	srv.Handler = newRouter(logger)

	// For local environment.
	// TODO make it more explicit
	if certDir == "" {
		srv := &http.Server{
			Addr:    ":8080",
			Handler: newRouter(logger),
		}

		logger.Printf("Starting HTTP server on %s", srv.Addr)
		err := srv.ListenAndServe()
		if err != nil {
			panic(err)
		}
		return
	}

	err = srv.ListenAndServeTLS("", "") // Key and cert provided automatically by autocert
	if err != nil {
		panic(err)
	}
}

func newRouter(logger *log.Logger) *http.ServeMux {
	r := http.NewServeMux()

	// TODO add hashing or secrets
	// TODO before release habits_v2 => habits
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Log Range requests
		if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
			log.Printf("🔍 Range request: %s %s - Range: %s", r.Method, r.URL.Path, rangeHeader)
			log.Printf("📱 User-Agent: %s", r.Header.Get("User-Agent"))
		}

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

	// TODO CHECK that user id belongs to oneTimeToken ID, or get userID by oneTimeToken id
	// TODO for further safety, remove * cors?
	r.HandleFunc("/syncTexts", panicMiddleware(corsMiddleware(tokenMiddleware(SyncTexts))))
	r.HandleFunc("/syncText", panicMiddleware(corsMiddleware(tokenMiddleware(SyncText))))
	r.HandleFunc("/syncMedias", panicMiddleware(corsMiddleware(tokenMiddleware(SyncMedias))))
	r.HandleFunc("/syncMedia", panicMiddleware(corsMiddleware(tokenMiddleware(SyncMedia))))
	r.HandleFunc("/token", panicMiddleware(corsMiddleware(IssueToken)))

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

	return r
}

func newLogger(logFilename string) *log.Logger {
	logFile, err := os.OpenFile(logFilename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		log.Fatalf("Server: failed to open log file: %v", err)
	}

	filteredWriter := &FilteredWriter{
		writer: logFile,
		ignorePatterns: []string{
			"TLS handshake error",
		},
	}

	return log.New(filteredWriter, "Server Error: ", log.Ldate|log.Ltime|log.Lshortfile)
}

type FilteredWriter struct {
	writer         io.Writer
	ignorePatterns []string
}

func (fw *FilteredWriter) Write(p []byte) (n int, err error) {
	message := string(p)
	for _, pattern := range fw.ignorePatterns {
		if strings.Contains(message, pattern) {
			return len(p), nil
		}
	}

	return fw.writer.Write(p)
}

func panicMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("Handler panic",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Any("panic", err),
					slog.String("stack", string(debug.Stack())),
				)

				if w.Header().Get("Content-Type") == "" {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"Internal server error"}`))
			}
		}()

		next(w, r)
	}
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization, X-CSRF-OneTimeToken, Version")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}
