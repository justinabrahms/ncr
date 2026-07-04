package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
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
	defaultMaxToks = 16000
)

var (
	includeRe     = regexp.MustCompile(`\{\{include:\s*([^}]+)\}\}`)
	placeholderRe = regexp.MustCompile(`\{\{([^}]+)\}\}`)
	headingRe     = regexp.MustCompile(`(?m)^##\s+(\w+)\s*$`)
	fenceRe       = regexp.MustCompile("(?s)```[a-zA-Z]*\\n(.*?)```")
)

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

	var fb strings.Builder
	for p, t := range files {
		fmt.Fprintf(&fb, "=== %s ===\n%s\n\n", p, t)
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

func runModel(system, user, model string, maxTokens int) ([]byte, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set")
	}
	reqBody, _ := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"system": []map[string]any{{
			"type": "text", "text": system,
			"cache_control": map[string]string{"type": "ephemeral"},
		}},
		"messages": []map[string]any{{"role": "user", "content": user}},
	})
	req, err := http.NewRequest("POST", anthropicURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", anthropicVers)
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("anthropic API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	var sb strings.Builder
	for _, c := range parsed.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	return extractJSON(sb.String())
}

// extractJSON pulls the first balanced {...} object out of a model response.
func extractJSON(text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		text = strings.Trim(text, "`\n ")
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		}
	}
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return nil, fmt.Errorf("no JSON object in model response")
	}
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
				return []byte(text[start : i+1]), nil
			}
		}
	}
	return nil, fmt.Errorf("unterminated JSON object in model response")
}
