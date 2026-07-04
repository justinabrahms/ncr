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

// serve hosts the review page + /api/* comment routes on a free localhost port
// and blocks until interrupted. See docs/design-review-comments.md.
func serve(html []byte, rs *reviewServer, open bool) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String() + "/"
	srv := &http.Server{Handler: rs.handler(html)}
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
