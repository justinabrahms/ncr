package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCacheDir(t *testing.T) {
	// NCR_CACHE_DIR override wins.
	t.Run("env override", func(t *testing.T) {
		t.Setenv("NCR_CACHE_DIR", "/some/custom/cache")
		if got := resolveCacheDir(); got != "/some/custom/cache" {
			t.Fatalf("resolveCacheDir() = %q, want /some/custom/cache", got)
		}
	})

	// With no override, default to os.UserCacheDir()/ncr rather than the CWD.
	t.Run("user cache dir default", func(t *testing.T) {
		t.Setenv("NCR_CACHE_DIR", "")
		want := ".ncr-cache"
		if base, err := os.UserCacheDir(); err == nil && base != "" {
			want = filepath.Join(base, "ncr")
		}
		got := resolveCacheDir()
		if got != want {
			t.Fatalf("resolveCacheDir() = %q, want %q", got, want)
		}
		if got == ".ncr-cache" && want != ".ncr-cache" {
			t.Fatalf("resolveCacheDir() defaulted to CWD .ncr-cache instead of user cache dir")
		}
	})
}
