// files.md lite launcher.
//
// Single-binary wrapper around the PWA: embeds the web/ directory, serves
// it on localhost, and opens either a native WebView2 window (default on
// Win10/11) or the system default browser. No backend, no bot, no sync —
// the web app falls back to local-only mode via the `lastServerOk`
// short-circuit in files.js.
//
// Use the `--browser` flag to force browser mode.
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/jchv/go-webview2"
)

//go:embed all:web
var webFS embed.FS

func main() {
	browserMode := flag.Bool("browser", false, "open default browser instead of native window")
	flag.Parse()

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	url := fmt.Sprintf("http://localhost:%d", ln.Addr().(*net.TCPAddr).Port)
	fmt.Printf("files.md running at %s\n", url)

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	var w webview2.WebView
	if !*browserMode {
		w = webview2.NewWithOptions(webview2.WebViewOptions{
			AutoFocus: true,
			WindowOptions: webview2.WindowOptions{
				Title:  "files.md",
				Width:  1200,
				Height: 800,
			},
		})
	}

	if w != nil {
		// Window mode: Run() blocks until the window is closed.
		defer w.Destroy()
		w.Navigate(url)
		w.Run()
	} else {
		// Browser mode: forced by --browser, or WebView2 runtime unavailable.
		if *browserMode {
			fmt.Println("Browser mode (--browser)")
		} else {
			fmt.Println("WebView2 runtime not detected, opening default browser…")
		}
		fmt.Println("Close this window or press Ctrl+C to quit.")
		if err := openBrowser(url); err != nil {
			log.Printf("could not open browser: %v (open %s manually)", err, url)
		}
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		fmt.Println("\nshutting down…")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
