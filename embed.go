package main

import "embed"

// Prompt templates are embedded so the binary is self-contained.
//
//go:embed prompts
var promptsFS embed.FS

// Interactive review UI assets (served in `ncr serve` mode).
//
//go:embed web/review.js
var reviewJS []byte

//go:embed web/review.css
var reviewCSS []byte
