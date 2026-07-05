package main

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// The plan step blocks on a single model call that can take 30–60s+ on a first
// (uncached) run. Without feedback that silence reads as a hang. spinner draws
// an animated "message (Ns)" line on stderr and ticks the elapsed seconds while
// the call is in flight, then clears the line when it returns (success or error).
// It only draws when stderr is a real terminal, so piped/redirected output stays
// clean (no escape codes, no partial lines).

// isTerminal reports whether f is an interactive terminal (a character device).
// Kept dependency-free: a char-device check is enough to gate the spinner and
// avoids pulling in golang.org/x/term for one call.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

type spinner struct {
	w       io.Writer
	msg     string
	done    chan struct{}
	stopped chan struct{}
	once    sync.Once
}

// startSpinner begins drawing on w when tty is true; otherwise it's a no-op that
// writes nothing (so callers can pass isTerminal(os.Stderr) unconditionally). The
// returned spinner's Stop must be called to clear the line and join the goroutine.
func startSpinner(w io.Writer, tty bool, msg string) *spinner {
	s := &spinner{
		w:       w,
		msg:     msg,
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	if !tty {
		close(s.stopped) // nothing running; Stop returns immediately
		return s
	}
	go s.run()
	return s
}

func (s *spinner) run() {
	defer close(s.stopped)
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	start := time.Now()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	i := 0
	draw := func() {
		secs := int(time.Since(start).Seconds())
		// \r returns to column 0, \033[K clears to end of line.
		fmt.Fprintf(s.w, "\r\033[K%c %s (%ds)", frames[i%len(frames)], s.msg, secs)
		i++
	}
	draw()
	for {
		select {
		case <-s.done:
			fmt.Fprint(s.w, "\r\033[K") // clear the spinner line on exit
			return
		case <-ticker.C:
			draw()
		}
	}
}

// Stop halts the spinner, clears its line, and waits for the goroutine to exit.
// Idempotent and safe to call once via defer.
func (s *spinner) Stop() {
	s.once.Do(func() { close(s.done) })
	<-s.stopped
}
