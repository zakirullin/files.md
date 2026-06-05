// Package webserver provides a server for habits tracking functionality through Telegram miniapps.
// SSLs certificates are handled automatically via LetsEncrypt.
package sync

import (
	"compress/gzip"
	"crypto/tls"
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

	"github.com/zakirullin/files.md/server/config"
	"github.com/zakirullin/files.md/server/fs"
	"github.com/zakirullin/files.md/server/habits"
	"github.com/zakirullin/files.md/server/journal"
	"github.com/zakirullin/files.md/server/llm"
	"github.com/zakirullin/files.md/server/userconfig"
)

type gzipResponseWriter struct {
	http.ResponseWriter
	gz *gzip.Writer
}

func (w gzipResponseWriter) Write(b []byte) (int, error) { return w.gz.Write(b) }

// gzipMiddleware streams the response through gzip when the client advertises
// gzip support. No-op otherwise.
func gzipMiddleware(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			h(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		h(gzipResponseWriter{ResponseWriter: w, gz: gz}, r)
	}
}

// Serve TODO release graceful shutdown etc
// All directories paths are absolute.
func Serve(apiHost, appHost, certDir, logFilename string) {
	// Logger is used for ssl/connection errors.
	// For regular errors we still use slog.
	serverLogger := newLogger(logFilename)

	serverLogger.Printf("Resolved hosts: api_host=%q app_host=%q cert_dir=%q", apiHost, appHost, certDir)

	// For local environment.
	// TODO make it more explicit
	if certDir == "" {
		srv := &http.Server{
			Addr:    ":8080",
			Handler: router(serverLogger),
		}

		serverLogger.Printf("Starting HTTP server on %s", srv.Addr)
		err := srv.ListenAndServe()
		if err != nil {
			panic(err)
		}
		return
	}

	// This will also launch :80 http server that would pass ACME challenges or redirects to :443.
	autocert := certServer(serverLogger, certDir, apiHost, appHost)
	tlsConfig := &tls.Config{
		GetCertificate:   autocert.GetCertificate,
		CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256},
	}

	srv := &http.Server{
		Addr:         ":443",
		TLSConfig:    tlsConfig,
		IdleTimeout:  2 * time.Minute,
		ReadTimeout:  30 * time.Second, // Otherwise we get net::ERR_HTTP2_PROTOCOL_ERROR (RST_STREAM) errors on slow clients (I personally experienced it in South America on syncMedia upload)
		WriteTimeout: 2 * time.Minute,  // For slow files like inbox.wasm.
		ErrorLog:     serverLogger,
	}
	srv.Handler = router(serverLogger)

	serverLogger.Printf("Starting HTTPS server on %s (api_host=%q app_host=%q cert_dir=%q)", srv.Addr, apiHost, appHost, certDir)
	err := srv.ListenAndServeTLS("", "") // Key and cert provided automatically by autocert
	if err != nil {
		panic(err)
	}
}

func router(serverLogger *log.Logger) *http.ServeMux {
	r := http.NewServeMux()

	// TODO add hashing or secrets
	// TODO before release habits_v2 => habits
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Log Range requests
		if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
			serverLogger.Printf("🔍 Range request: %s %s - Range: %s", r.Method, r.URL.Path, rangeHeader)
			serverLogger.Printf("📱 User-Agent: %s", r.Header.Get("User-Agent"))
		}

		// Serving the PWA app
		if appHost := config.ServerCfg.AppHost(); appHost != "" && r.Host == appHost {
			http.FileServer(http.Dir("./web")).ServeHTTP(w, r)
			return
		}

		http.NotFound(w, r)
	})

	// Sync API is on only when API_URL is not empty.
	if config.ServerCfg.APIHost() != "" {
		r.HandleFunc("/syncFilenames", corsMiddleware(panicMiddleware(tokenMiddleware(gzipMiddleware(SyncFilenames)))))
		r.HandleFunc("/syncFile", corsMiddleware(panicMiddleware(tokenMiddleware(gzipMiddleware(SyncFile)))))
		r.HandleFunc("/syncMediaFilenames", corsMiddleware(panicMiddleware(tokenMiddleware(gzipMiddleware(SyncMediaFilenames)))))
		r.HandleFunc("/syncMediaFile", corsMiddleware(panicMiddleware(tokenMiddleware(gzipMiddleware(SyncMediaFile)))))
		r.HandleFunc("/issuePermanentToken", corsMiddleware(panicMiddleware(IssueToken)))
		llmHandler := llm.NewHandler(llm.FromServerConfig(config.ServerCfg))
		r.HandleFunc("/llmStatus", corsMiddleware(panicMiddleware(llm.AuthErrorJSON(http.HandlerFunc(tokenMiddleware(llmHandler.Status))).ServeHTTP)))
		r.HandleFunc("/llmChat", corsMiddleware(panicMiddleware(llm.AuthErrorJSON(http.HandlerFunc(tokenMiddleware(llmHandler.Chat))).ServeHTTP)))

		// Deprecated due to cryptic names :) Will be removed soon.
		r.HandleFunc("/syncTexts", corsMiddleware(panicMiddleware(tokenMiddleware(gzipMiddleware(SyncFilenames)))))
		r.HandleFunc("/syncText", corsMiddleware(panicMiddleware(tokenMiddleware(gzipMiddleware(SyncFile)))))
		r.HandleFunc("/syncMedias", corsMiddleware(panicMiddleware(tokenMiddleware(gzipMiddleware(SyncMediaFilenames)))))
		r.HandleFunc("/syncMedia", corsMiddleware(panicMiddleware(tokenMiddleware(gzipMiddleware(SyncMediaFile)))))
		r.HandleFunc("/token", corsMiddleware(panicMiddleware(IssueToken)))
	}

	// For now it is possible to see other user's habits, but is it a big deal?
	// TODO use X-Telegram-Init-Data header to verify requests
	r.HandleFunc("GET /habits_v2/{userID}", func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
		if err != nil {
			serverLogger.Printf("failed to parse userID for habits: %v", err)
			http.Error(w, "can't parse userID", http.StatusBadRequest)
			return
		}

		userFS, err := fs.NewUserFS(userID)
		if err != nil {
			serverLogger.Printf("failed to init userFS: %v", err)
			http.Error(w, "can't init userFS", http.StatusInternalServerError)
			return
		}

		str, err := habits.Render(userID, userFS)
		if err != nil {
			serverLogger.Printf("failed to render habits: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, err = w.Write(str)
		if err != nil {
			serverLogger.Printf("failed to write habits response: %v", err)
		}
	})

	// TODO use X-Telegram-Init-Data header to verify requests
	r.HandleFunc("POST /habits_v2/{userID}/{habitName}/{yearDay}/{status}", func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
		if err != nil {
			serverLogger.Printf("failed to parse userID: %v", err)
			http.Error(w, "can't parse userID", http.StatusBadRequest)
			return
		}

		yearDay, err := strconv.ParseInt(r.PathValue("yearDay"), 10, 32)
		if err != nil {
			serverLogger.Printf("failed to parse yearDay: %v", err)
			http.Error(w, "can't parse yearDay", http.StatusBadRequest)
			return
		}

		status, err := strconv.ParseInt(r.PathValue("status"), 10, 32)
		if err != nil {
			serverLogger.Printf("failed to parse status: %v", err)
			http.Error(w, "can't parse status", http.StatusBadRequest)
			return
		}

		habitName := r.PathValue("habitName")

		userFS, err := fs.NewUserFS(userID)
		if err != nil {
			serverLogger.Printf("failed to init user fs: %v", err)
			http.Error(w, "can't init user fs", http.StatusInternalServerError)
			return
		}

		userHabits, err := habits.Habits(userFS, time.Now().Year())
		if err != nil {
			serverLogger.Printf("failed to read habits: %v", err)
			http.Error(w, "can't read habits", http.StatusInternalServerError)
			return
		}

		if _, ok := userHabits[habitName]; !ok {
			userHabits[habitName] = make(habits.Year)
		}
		userHabits[habitName][int(yearDay)] = int(status)
		err = habits.Write(userFS, time.Now().Year(), userHabits)
		if err != nil {
			serverLogger.Printf("failed to write habits: %v", err)
			http.Error(w, "can't write habits", http.StatusInternalServerError)
			return
		}

		emoji := habits.Emoji(userFS, habitName)
		if habitName == habits.MoodHabit {
			if int(status) < len(habits.MoodEmojis) {
				emoji = habits.MoodEmojis[status]
			}
		}

		userConf := userconfig.NewConfig(userFS, userID, config.ServerCfg.ConfigFilename)
		err = journal.AddEmoji(userFS, emoji, userConf.Timezone())
		if err != nil {
			serverLogger.Printf("failed to write habit emoji to journal: %v", err)
			http.Error(w, "can't write habit emoji to journal", http.StatusInternalServerError)
			return
		}

		record := fmt.Sprintf("%s %s", emoji, habitName)
		err = journal.AddRecord(userFS, record, userConf.Timezone())
		if err != nil {
			serverLogger.Printf("failed to write habit to journal: %v", err)
			http.Error(w, "can't write habit to journal", http.StatusInternalServerError)
			return
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
		origin := r.Header.Get("Origin")
		if origin == config.ServerCfg.AppURL {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization, X-CSRF-OneTimeToken, Version")
		w.Header().Set("Vary", "Origin")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}
