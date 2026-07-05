package main

import (
	"bytes"
	"testing"
)

// A non-TTY spinner must write nothing — piped/redirected output stays clean.
func TestSpinnerNoTTYWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	sp := startSpinner(&buf, false, "working")
	sp.Stop()
	if buf.Len() != 0 {
		t.Fatalf("non-tty spinner wrote %q, want nothing", buf.String())
	}
}

// Stop is idempotent and must not panic on a repeat call.
func TestSpinnerStopIdempotent(t *testing.T) {
	var buf bytes.Buffer
	sp := startSpinner(&buf, false, "working")
	sp.Stop()
	sp.Stop()
}
