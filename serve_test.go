package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReviewHandlerServesHTML(t *testing.T) {
	html := []byte("<!doctype html><title>ncr</title>")
	srv := httptest.NewServer(reviewHandler(html))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("content-type = %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != string(html) {
		t.Fatalf("body mismatch")
	}
}

func TestReviewHandler404(t *testing.T) {
	srv := httptest.NewServer(reviewHandler([]byte("x")))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/nope")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
