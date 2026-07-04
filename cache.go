package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Content-addressed cache — port of ncr/cache.py. Lets you iterate on presentation
// without re-spending API credits: the plan is keyed by a hash of the exact
// prompt, so an unchanged PR + prompt is a hit and makes no API call. Ingest is
// cached by repo#pr. Dir: $NCR_CACHE_DIR or ./.ncr-cache; --refresh busts it.

var cacheSanitize = regexp.MustCompile(`[^A-Za-z0-9._#-]`)

func cacheDir() string {
	d := os.Getenv("NCR_CACHE_DIR")
	if d == "" {
		d = ".ncr-cache"
	}
	_ = os.MkdirAll(d, 0o755)
	return d
}

func cacheDigest(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:])[:16]
}

func cachePath(name string) string {
	return filepath.Join(cacheDir(), cacheSanitize.ReplaceAllString(name, "_")+".json")
}

func cacheLoad(name string) ([]byte, bool) {
	b, err := os.ReadFile(cachePath(name))
	if err != nil {
		return nil, false
	}
	return b, true
}

func cacheSave(name string, data []byte) error {
	return os.WriteFile(cachePath(name), data, 0o644)
}
