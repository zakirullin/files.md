package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/crypto/acme/autocert"

	"zakirullin/stuffbot/config"
	"zakirullin/stuffbot/i18n"
	"zakirullin/stuffbot/internal/fs"
	"zakirullin/stuffbot/internal/habits"
	"zakirullin/stuffbot/internal/journal"
	"zakirullin/stuffbot/internal/userconfig"
	"zakirullin/stuffbot/pkg/txt"
)

// TODO release graceful shutdown etc
func habitsServer(habitsHost, habitsCertsPath string) {
	autocertManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(habitsHost),
		Cache:      autocert.DirCache(habitsCertsPath),
	}

	// Listen for HTTP requests on port 80 in a new goroutine. Use
	// autocertManager.HTTPHandler(nil) as the handler. This will send ACME
	// "http-01" challenge responses as necessary, and 302 redirect all other
	// requests to HTTPS.
	go func() {
		srv := &http.Server{
			Addr:         ":80",
			Handler:      autocertManager.HTTPHandler(nil),
			IdleTimeout:  time.Minute,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
		}

		err := srv.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}()

	// Configure the TLS config to use the autocertManager.GetCertificate function.
	tlsConfig := &tls.Config{
		GetCertificate:   autocertManager.GetCertificate,
		CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256},
	}

	router := http.NewServeMux()
	setupRouter(router)
	srv := &http.Server{
		Addr:         ":443",
		Handler:      router,
		TLSConfig:    tlsConfig,
		IdleTimeout:  time.Minute,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	err := srv.ListenAndServeTLS("", "") // Key and cert provided automatically by autocert
	if err != nil {
		panic(err)
	}
}

func setupRouter(router *http.ServeMux) {
	// TODO add hashing or secrets
	// TODO before release habits_v2 => habits
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<body style="text-align: center; background-color: #FFFCF0; color: #100F0F; padding: 10px; margin: 0; font-size: 1.4em; font-family: -apple-system, BlinkMacSystemFont, 'Inter', 'IBM Plex Sans', 'Segoe UI', Helvetica, Arial, sans-serif;">Files made to last</body>`))
	})

	router.HandleFunc("GET /habits_v2/{userID}", func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
		if err != nil {
			w.Write([]byte("can't parse userID"))
		}

		userPath := path.Join(config.BotCfg.StoragePath, txt.I64(userID))
		userFS, err := fs.NewFS(userPath, afero.NewOsFs())
		if err != nil {
			w.Write([]byte("can't init userFS"))
		}

		str, err := habits.Render(userID, userFS)
		if err != nil {
			w.Write([]byte(err.Error()))
		}
		w.Write(str)
	})

	router.HandleFunc("POST /habits_v2/{userID}/{habitName}/{yearDay}/{status}", func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
		if err != nil {
			w.Write([]byte("can't parse userID"))
		}

		yearDay, err := strconv.ParseInt(r.PathValue("yearDay"), 10, 32)
		if err != nil {
			w.Write([]byte("can't parse yearDay"))
		}

		status, err := strconv.ParseInt(r.PathValue("status"), 10, 32)
		if err != nil {
			w.Write([]byte("can't parse status"))
		}

		habitName := r.PathValue("habitName")

		userPath := path.Join(config.BotCfg.StoragePath, txt.I64(userID))
		userFS, err := fs.NewFS(userPath, afero.NewOsFs())
		if err != nil {
			w.Write([]byte("can't init user fs"))
		}

		userHabits, err := habits.Habits(userFS, time.Now().Year())
		if err != nil {
			w.Write([]byte("can't read habits"))
		}

		if _, ok := userHabits[habitName]; !ok {
			userHabits[habitName] = make(habits.Year)
		}
		userHabits[habitName][int(yearDay)] = int(status)
		err = habits.Write(userFS, time.Now().Year(), userHabits)
		if err != nil {
			w.Write([]byte("can't write habits"))
		}

		var emoji string
		if habitName == habits.MoodHabit {
			if int(status) < len(habits.MoodEmojis) {
				emoji = habits.MoodEmojis[status]
			}
		} else {
			emoji, _ = userFS.Read(fs.DirHabits, fs.Filename(habitName))
			if emoji == "" {
				emoji = i18n.Emoji(habitName)
			}
			if emoji == "" {
				emoji = "⚡️"
			}
		}

		userConf := userconfig.NewConfig(userFS, userID, config.BotCfg.ConfigFilename)
		err = journal.AddEmoji(userFS, emoji, userConf.Timezone())
		if err != nil {
			w.Write([]byte("can't write habit emoji to journal"))
		}

		record := fmt.Sprintf("%s %s", emoji, habitName)
		err = journal.AddRecord(userFS, record, userConf.Timezone())
		if err != nil {
			w.Write([]byte("can't write habit to journal"))
		}
	})
}
