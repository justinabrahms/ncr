package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// LLM plan step — port of ncr/plan.py. Loads the embedded single-shot prompt,
// fills placeholders, and calls the Anthropic Messages API (prompt caching on the
// stable system prompt). The reconciler owns completeness, so JSON parsing is
// tolerant. build_prompt is split out so its hash can key the cache.

const (
	defaultModel   = "claude-sonnet-4-6"
	anthropicURL   = "https://api.anthropic.com/v1/messages"
	anthropicVers  = "2023-06-01"
	defaultMaxToks = 32000
	schemaVersion  = "toolv4" // bump when the tool/request shape changes (salts the cache key)
)

var (
	includeRe     = regexp.MustCompile(`\{\{include:\s*([^}]+)\}\}`)
	placeholderRe = regexp.MustCompile(`\{\{([^}]+)\}\}`)
	headingRe     = regexp.MustCompile(`(?m)^##\s+(\w+)\s*$`)
	fenceRe       = regexp.MustCompile("(?s)```[a-zA-Z]*\\n(.*?)```")
	jsonFenceRe   = regexp.MustCompile("(?s)```(?:json)?\\s*\\n(.*?)```")
)

// resolveMaxTokens picks the model's max_tokens ceiling: an explicit --max-tokens
// flag (>0) wins, then NCR_MAX_TOKENS, then defaultMaxToks. Non-positive or
// unparseable values are ignored so a bad env var can't silently zero the budget.
func resolveMaxTokens(flagVal int, envVal string) int {
	if flagVal > 0 {
		return flagVal
	}
	if n, err := strconv.Atoi(strings.TrimSpace(envVal)); err == nil && n > 0 {
		return n
	}
	return defaultMaxToks
}

func readPrompt(name string) string {
	b, err := promptsFS.ReadFile("prompts/" + name)
	if err != nil {
		return ""
	}
	return string(b)
}

func resolveIncludes(text string) string {
	return includeRe.ReplaceAllStringFunc(text, func(m string) string {
		sub := includeRe.FindStringSubmatch(m)
		return readPrompt(strings.TrimSpace(sub[1]))
	})
}

// section returns the body under "## <heading>" up to the next "## ".
func section(raw, heading string) string {
	locs := headingRe.FindAllStringSubmatchIndex(raw, -1)
	for i, loc := range locs {
		if raw[loc[2]:loc[3]] != heading {
			continue
		}
		start := loc[1]
		end := len(raw)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		return raw[start:end]
	}
	return ""
}

func loadPrompt(name string) (system, user string) {
	raw := readPrompt(name)
	system = strings.TrimSpace(resolveIncludes(section(raw, "System")))
	userBody := section(raw, "User")
	if m := fenceRe.FindStringSubmatch(userBody); m != nil {
		user = strings.TrimSpace(m[1])
	} else {
		user = strings.TrimSpace(userBody)
	}
	return system, user
}

func renderUser(tmpl string, vars map[string]string) string {
	return placeholderRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		key := strings.TrimSpace(placeholderRe.FindStringSubmatch(m)[1])
		if v, ok := vars[key]; ok {
			return v
		}
		return m
	})
}

type slimBlock struct {
	BlockID    string `json:"blockId"`
	Path       string `json:"path"`
	ChangeType string `json:"changeType"`
	OldStart   *int   `json:"oldStart"`
	OldLines   int    `json:"oldLines"`
	NewStart   *int   `json:"newStart"`
	NewLines   int    `json:"newLines"`
	Header     string `json:"header"`
	Text       string `json:"text"`
	Sha        string `json:"sha"`
}

// buildPrompt returns the exact (system, user) strings sent to the model. The
// model doesn't need display-only context lines, so they're dropped here (keeps
// the prompt — and thus the cache key — stable as the renderer evolves).
func buildPrompt(index Index, files map[string]string, comments []Comment, meta Meta) (string, string) {
	system, userTmpl := loadPrompt("00-single-shot.md")

	slim := struct {
		Blocks   []slimBlock `json:"blocks"`
		BlockIDs []string    `json:"blockIds"`
	}{BlockIDs: index.BlockIDs}
	for _, b := range index.Blocks {
		slim.Blocks = append(slim.Blocks, slimBlock{
			b.BlockID, b.Path, b.ChangeType, b.OldStart, b.OldLines,
			b.NewStart, b.NewLines, b.Header, b.Text, b.Sha,
		})
	}
	slimJSON, _ := json.MarshalIndent(slim, "", " ")

	// Sort file paths for a stable prompt — Go map iteration is randomized, and an
	// unstable order would change the prompt hash (the cache key) every run.
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var fb strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&fb, "=== %s ===\n%s\n\n", p, files[p])
	}
	filesTxt := strings.TrimSpace(fb.String())
	if filesTxt == "" {
		filesTxt = "(not provided)"
	}
	commentsJSON, _ := json.Marshal(comments)

	user := renderUser(userTmpl, map[string]string{
		"prTitle":       meta.Title,
		"prNumber":      itoaOrEmpty(meta.Number),
		"prDescription": meta.Body,
		"blockIndex":    string(slimJSON),
		"files":         filesTxt,
		"comments":      string(commentsJSON),
	})
	return system, user
}

func itoaOrEmpty(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d", n)
}

// planTool builds the forced tool the model must call. The block-id field is an
// enum of this PR's actual ids, so the model can't reference a block that doesn't
// exist; layer is constrained to 0–6. Forcing the tool means the response is
// structured JSON (the tool input) rather than free-form prose we have to scrape.
func planTool(index Index) map[string]any {
	unit := map[string]any{
		"type":     "object",
		"required": []string{"blocks", "summary"},
		"properties": map[string]any{
			"blocks": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string", "enum": index.BlockIDs},
				"description": "block ids from the index this unit covers. Group blocks by concern: a unit may span several functions, types, or files if they advance one idea. Prefer fewer, larger concern-units over one-per-function. Every block id must appear in exactly one unit.",
			},
			"symbol":      map[string]any{"type": "string", "description": "a short label for this unit's concern — a symbol name when it's one function/type, else a few-word description of the shared concern (may span files)"},
			"layer":       map[string]any{"type": "integer", "enum": []int{0, 1, 2, 3, 4, 5, 6}},
			"layerReason": map[string]any{"type": "string"},
			"summary":     map[string]any{"type": "string", "description": "one short line — the point/intent of this change, NOT a restatement of the diff (the reader sees the diff). A few words if the code is self-evident."},
			"detail":      map[string]any{"type": "string", "description": "optional; include ONLY when it adds something the diff doesn't show — a reason, a non-obvious consequence. Omit for self-evident changes (most of the time)."},
		},
	}
	chapter := map[string]any{
		"type":     "object",
		"required": []string{"title", "changeUnits"},
		"properties": map[string]any{
			"title":       map[string]any{"type": "string", "description": "the chapter's theme — a capability like 'POST /orders — place an order', or a concern like 'Line-level completeness accounting'. Never a bare filename."},
			"summary":     map[string]any{"type": "string"},
			"changeUnits": map[string]any{"type": "array", "items": unit},
		},
	}
	schema := map[string]any{
		"type":     "object",
		"required": []string{"overview", "chapters"},
		"properties": map[string]any{
			"title":    map[string]any{"type": "string"},
			"overview": map[string]any{"type": "string", "description": "2–4 sentences: what the PR does and the suggested reading path"},
			"chapters": map[string]any{"type": "array", "items": chapter, "description": "outside-in reading order; each a coherent story — a capability (call-path) or, for refactors/tooling, a shared theme. Never one chapter per file."},
			"orphans": map[string]any{
				"type":        "array",
				"description": "changed units with no in-diff caller, grouped by layer",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"layer":       map[string]any{"type": "integer"},
						"changeUnits": map[string]any{"type": "array", "items": unit},
					},
				},
			},
		},
	}
	return map[string]any{
		"name":         "submit_reading_plan",
		"description":  "Submit the outside-in reading plan. Every block id in the index must appear in exactly one unit's blocks.",
		"input_schema": schema,
	}
}

// runModel calls the Messages API forcing planTool, and returns the tool input
// (structured JSON). Falls back to text extraction only if no tool_use block is
// present (shouldn't happen with tool_choice, but be defensive).
func runModel(system, user, model string, maxTokens int, tool map[string]any) ([]byte, Usage, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, Usage{}, fmt.Errorf("ANTHROPIC_API_KEY is not set")
	}
	reqMap := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"system": []map[string]any{{
			"type": "text", "text": system,
			"cache_control": map[string]string{"type": "ephemeral"},
		}},
		"messages": []map[string]any{{"role": "user", "content": user}},
	}
	if tool != nil {
		reqMap["tools"] = []any{tool}
		reqMap["tool_choice"] = map[string]any{"type": "tool", "name": tool["name"]}
	}
	reqBody, _ := json.Marshal(reqMap)

	req, err := http.NewRequest("POST", anthropicURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, Usage{}, err
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", anthropicVers)
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, Usage{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, Usage{}, fmt.Errorf("anthropic API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return parseModelResponse(body, maxTokens)
}

// parseModelResponse pulls the plan JSON out of a Messages API response: the
// forced tool_use input when present, else text (via extractJSON). It also
// returns token usage for cost reporting.
func parseModelResponse(body []byte, maxTokens int) ([]byte, Usage, error) {
	var parsed struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      Usage  `json:"usage"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, Usage{}, err
	}
	if parsed.StopReason == "max_tokens" {
		return nil, parsed.Usage, fmt.Errorf("model response hit max_tokens (%d) — the plan was truncated; raise the ceiling with --max-tokens (or NCR_MAX_TOKENS)", maxTokens)
	}
	for _, c := range parsed.Content {
		if c.Type == "tool_use" && len(c.Input) > 0 {
			return c.Input, parsed.Usage, nil // already clean structured JSON
		}
	}
	var sb strings.Builder
	for _, c := range parsed.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	if sb.Len() == 0 {
		return nil, parsed.Usage, fmt.Errorf("model returned no tool_use and no text")
	}
	b, err := extractJSON(sb.String())
	return b, parsed.Usage, err
}

// extractJSON is a defensive fallback for text responses: it returns the largest
// balanced {...} that is valid JSON (preferring a fenced ```json block), so prose
// or an incidental `{ code snippet }` before the real object can't fool it.
func extractJSON(text string) ([]byte, error) {
	if m := jsonFenceRe.FindStringSubmatch(text); m != nil {
		if c := strings.TrimSpace(m[1]); json.Valid([]byte(c)) {
			return []byte(c), nil
		}
	}
	var best []byte
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		if end := matchBrace(text, i); end > i {
			if c := text[i : end+1]; len(c) > len(best) && json.Valid([]byte(c)) {
				best = []byte(c)
			}
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no valid JSON object in model response")
	}
	return best, nil
}

// matchBrace returns the index of the '}' closing the '{' at start (string-aware).
func matchBrace(text string, start int) int {
	depth, inStr, esc := 0, false, false
	for i := start; i < len(text); i++ {
		c := text[i]
		switch {
		case inStr:
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
		case c == '"':
			inStr = true
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
