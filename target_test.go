package main

import "testing"

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name     string
		pos      []string
		wantRepo string
		wantPR   int
		wantErr  bool
	}{
		{
			name:     "PR URL",
			pos:      []string{"https://github.com/owner/repo/pull/812"},
			wantRepo: "owner/repo",
			wantPR:   812,
		},
		{
			name:     "PR URL with trailing path",
			pos:      []string{"https://github.com/owner/repo/pull/812/files"},
			wantRepo: "owner/repo",
			wantPR:   812,
		},
		{
			name:     "bare number infers repo",
			pos:      []string{"812"},
			wantRepo: "",
			wantPR:   812,
		},
		{
			name:     "owner/name and number",
			pos:      []string{"owner/name", "812"},
			wantRepo: "owner/name",
			wantPR:   812,
		},
		{
			name:    "no args",
			pos:     nil,
			wantErr: true,
		},
		{
			name:    "single non-numeric non-URL",
			pos:     []string{"owner/name"},
			wantErr: true,
		},
		{
			name:    "non-github URL",
			pos:     []string{"https://example.com/owner/repo/pull/812"},
			wantErr: true,
		},
		{
			name:    "zero PR number",
			pos:     []string{"0"},
			wantErr: true,
		},
		{
			name:    "invalid PR number in two-arg form",
			pos:     []string{"owner/name", "notanumber"},
			wantErr: true,
		},
		{
			name:    "too many args",
			pos:     []string{"a", "1", "extra"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, pr, err := parseTarget(tt.pos)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseTarget(%q) = (%q, %d, nil); want error", tt.pos, repo, pr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTarget(%q) unexpected error: %v", tt.pos, err)
			}
			if repo != tt.wantRepo || pr != tt.wantPR {
				t.Fatalf("parseTarget(%q) = (%q, %d); want (%q, %d)", tt.pos, repo, pr, tt.wantRepo, tt.wantPR)
			}
		})
	}
}

func TestParsePRURL(t *testing.T) {
	if r, n, ok := parsePRURL("https://github.com/octo/cat/pull/7"); !ok || r != "octo/cat" || n != 7 {
		t.Fatalf("parsePRURL octo/cat/pull/7 = (%q, %d, %v)", r, n, ok)
	}
	if _, _, ok := parsePRURL("https://github.com/octo/cat/issues/7"); ok {
		t.Fatalf("parsePRURL should reject issue URLs")
	}
	if _, _, ok := parsePRURL("not a url at all ::"); ok {
		t.Fatalf("parsePRURL should reject garbage")
	}
}
