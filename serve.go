package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Phase 1 of the review-comments feature (docs/design-review-comments.md): serve
// the rendered page over localhost instead of writing a file. The comment queue +
// submit API land in later phases; for now this just hosts the HTML and blocks
// until Ctrl-C.

// reviewHandler serves the rendered review page. Kept separate so it's testable
// and so later phases can hang the /api/* routes off the same mux.
func reviewHandler(html []byte) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(html)
	})
	return mux
}

// serve hosts the page on a free localhost port and blocks until interrupted.
func serve(html []byte, open bool) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String() + "/"
	srv := &http.Server{Handler: reviewHandler(html)}
	go func() { _ = srv.Serve(ln) }()

	logf("serving %s  (Ctrl-C to stop)", url)
	if open {
		openTarget(url)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	logf("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
