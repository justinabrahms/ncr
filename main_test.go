package main

import "testing"

func TestResolveModel(t *testing.T) {
	// flag wins over env and default
	t.Run("flag beats env", func(t *testing.T) {
		t.Setenv("NCR_MODEL", "env-model")
		if got := resolveModel("flag-model"); got != "flag-model" {
			t.Fatalf("resolveModel = %q, want flag-model", got)
		}
	})
	// env is used when the flag is empty
	t.Run("env beats default", func(t *testing.T) {
		t.Setenv("NCR_MODEL", "env-model")
		if got := resolveModel(""); got != "env-model" {
			t.Fatalf("resolveModel = %q, want env-model", got)
		}
	})
	// default is used when neither flag nor env is set
	t.Run("default fallback", func(t *testing.T) {
		t.Setenv("NCR_MODEL", "")
		if got := resolveModel(""); got != defaultModel {
			t.Fatalf("resolveModel = %q, want %q", got, defaultModel)
		}
	})
}
