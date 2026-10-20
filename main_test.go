package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanUserInput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "wrapped request with tabs and carriage returns, want cleaned string",
			raw:  "prefix\n<USER_REQUEST>\n\thello\tworld\r\n</USER_REQUEST>\nsuffix",
			want: "hello  world",
		},
		{
			name: "plain text without tags, want trimmed string",
			raw:  "  simple prompt\n",
			want: "simple prompt",
		},
		{
			name: "empty request tags, want empty string",
			raw:  "<USER_REQUEST>   \n\t  </USER_REQUEST>",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanUserInput(tc.raw); got != tc.want {
				t.Errorf("cleanUserInput(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestParseTranscript(t *testing.T) {
	tmpDir := t.TempDir()
	validPath := filepath.Join(tmpDir, "valid.jsonl")
	validContent := strings.Join([]string{
		`{"type":"USER_INPUT","created_at":"2026-10-18T10:00:00Z","content":"<USER_REQUEST>\nFirst line\nSecond line\n</USER_REQUEST>"}`,
		`not valid json line`,
		`{"type":"GENERIC","created_at":"2026-10-18T10:01:00Z","content":"tool output"}`,
		`{"type":"PLANNER_RESPONSE","created_at":"2026-10-18T10:02:00Z","content":"First reply"}`,
		`{"type":"PLANNER_RESPONSE","created_at":"2026-10-18T10:03:00Z","content":"   "}`,
		`{"type":"PLANNER_RESPONSE","created_at":"2026-10-18T10:04:00Z","content":"Final reply"}`,
	}, "\n")
	if err := os.WriteFile(validPath, []byte(validContent), 0600); err != nil {
		t.Fatalf("write valid transcript: %v", err)
	}

	tests := []struct {
		name      string
		path      string
		dbTitle   string
		wantTitle string
		wantFirst string
		wantLast  string
		wantMsgs  int
		wantErr   error
	}{
		{
			name:      "valid transcript without dbTitle, want fallback title from first line",
			path:      validPath,
			dbTitle:   "",
			wantTitle: "First line",
			wantFirst: "First line\nSecond line",
			wantLast:  "Final reply",
			wantMsgs:  3,
			wantErr:   nil,
		},
		{
			name:      "valid transcript with dbTitle, want dbTitle preserved",
			path:      validPath,
			dbTitle:   "Custom Title",
			wantTitle: "Custom Title",
			wantFirst: "First line\nSecond line",
			wantLast:  "Final reply",
			wantMsgs:  3,
			wantErr:   nil,
		},
		{
			name:    "non-existent file path, want os.ErrNotExist",
			path:    filepath.Join(tmpDir, "missing.jsonl"),
			wantErr: os.ErrNotExist,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTranscript("id-1", tc.path, tc.dbTitle)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("parseTranscript() error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}
			if got.Title != tc.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tc.wantTitle)
			}
			if got.FirstUserMsg != tc.wantFirst {
				t.Errorf("FirstUserMsg = %q, want %q", got.FirstUserMsg, tc.wantFirst)
			}
			if got.LastAgentMsg != tc.wantLast {
				t.Errorf("LastAgentMsg = %q, want %q", got.LastAgentMsg, tc.wantLast)
			}
			if len(got.Messages) != tc.wantMsgs {
				t.Errorf("len(Messages) = %d, want %d", len(got.Messages), tc.wantMsgs)
			}
		})
	}
}

func TestFormatBlockAndHighlight(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		width    int
		maxLines int
		query    string
		wantSub  string
	}{
		{
			name:     "multibyte unicode İ before match, want match highlighted without panic",
			text:     "İstanbul İİİ scalibr extractor support",
			width:    30,
			maxLines: 3,
			query:    "scalibr",
			wantSub:  "scalibr",
		},
		{
			name:     "narrow width 1 and long text, want truncated output without panic",
			text:     "hello world",
			width:    1,
			maxLines: 2,
			query:    "",
			wantSub:  "h",
		},
		{
			name:     "empty text, want (empty) placeholder",
			text:     "",
			width:    20,
			maxLines: 3,
			query:    "",
			wantSub:  "(empty)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatBlock(tc.text, tc.width, tc.maxLines, tc.query)
			if !strings.Contains(got, tc.wantSub) {
				t.Errorf("formatBlock() = %q, want substring %q", got, tc.wantSub)
			}
		})
	}
}
