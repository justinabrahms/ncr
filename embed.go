package main

import "embed"

// Prompt templates are embedded so the binary is self-contained.
//
//go:embed prompts
var promptsFS embed.FS
