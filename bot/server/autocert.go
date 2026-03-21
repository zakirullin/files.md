package server

import (
	"log"
	"net/http"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

func certServer(logger *log.Logger, certDir string, hosts ...string) *autocert.Manager {
	autocertManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(hosts...),
		Cache:      autocert.DirCache(certDir),
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
			ErrorLog:     logger,
		}

		err := srv.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}()

	return &autocertManager
}
