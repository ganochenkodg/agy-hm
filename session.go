package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Message represents a single user input or planner response in a session.
type Message struct {
	Role         string
	Content      string
	LowerContent string
}

// Session holds metadata and messages for a single conversation.
type Session struct {
	ID           string
	Title        string
	LowerTitle   string
	UpdatedAt    time.Time
	StepCount    int
	FirstUserMsg string
	LastAgentMsg string
	Messages     []Message
}

type transcriptStep struct {
	Type      string `json:"type"`
	CreatedAt string `json:"created_at"`
	Content   string `json:"content"`
}

// loadSessions reads all sessions from the antigravity-cli data directory.
func loadSessions(appDataDir string) ([]Session, error) {
	titles, err := loadTitles(filepath.Join(appDataDir, "conversation_summaries.db"))
	if err != nil {
		titles = make(map[string]string)
	}
	brainDir := filepath.Join(appDataDir, "brain")

	entries, err := os.ReadDir(brainDir)
	if err != nil {
		return nil, fmt.Errorf("read brain dir: %w", err)
	}

	var sessions []Session
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		transcriptPath := filepath.Join(brainDir, id, ".system_generated", "logs", "transcript.jsonl")
		s, err := parseTranscript(id, transcriptPath, titles[id])
		if err != nil || len(s.Messages) == 0 {
			continue
		}
		sessions = append(sessions, s)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

// loadTitles queries conversation_summaries.db for auto-generated session titles using pure-Go SQLite.
func loadTitles(dbPath string) (map[string]string, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open summaries db: %w", err)
	}
	defer db.Close()

	const query = `SELECT conversation_id, CASE WHEN title != '' THEN title ELSE preview END FROM conversation_summaries`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query summaries db: %w", err)
	}
	defer rows.Close()

	titles := make(map[string]string)
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			continue
		}
		if title = strings.TrimSpace(title); title != "" {
			titles[id] = title
		}
	}
	if err := rows.Err(); err != nil {
		return titles, fmt.Errorf("iterate summaries rows: %w", err)
	}
	return titles, nil
}

// parseTranscript extracts user inputs, planner responses, and timestamps from transcript.jsonl.
func parseTranscript(id, path, dbTitle string) (Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return Session{}, fmt.Errorf("stat transcript: %w", err)
	}

	s := Session{
		ID:        id,
		Title:     dbTitle,
		UpdatedAt: info.ModTime(),
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		s.StepCount++
		var step transcriptStep
		if err := json.Unmarshal(scanner.Bytes(), &step); err != nil {
			continue
		}
		if t, err := time.Parse(time.RFC3339, step.CreatedAt); err == nil {
			s.UpdatedAt = t
		}

		switch step.Type {
		case "USER_INPUT":
			text := cleanUserInput(step.Content)
			if text == "" {
				continue
			}
			if s.FirstUserMsg == "" {
				s.FirstUserMsg = text
			}
			s.Messages = append(s.Messages, Message{
				Role:         "User",
				Content:      text,
				LowerContent: strings.ToLower(text),
			})
		case "PLANNER_RESPONSE":
			text := sanitizeText(step.Content)
			if text == "" {
				continue
			}
			s.LastAgentMsg = text
			s.Messages = append(s.Messages, Message{
				Role:         "AGY",
				Content:      text,
				LowerContent: strings.ToLower(text),
			})
		}
	}

	if s.Title == "" {
		s.Title = firstLine(s.FirstUserMsg)
	}
	s.LowerTitle = strings.ToLower(s.Title)
	return s, scanner.Err()
}

// cleanUserInput strips <USER_REQUEST> wrapper tags and normalizes whitespace.
func cleanUserInput(raw string) string {
	if idx := strings.Index(raw, "<USER_REQUEST>"); idx != -1 {
		raw = raw[idx+len("<USER_REQUEST>"):]
	}
	if idx := strings.Index(raw, "</USER_REQUEST>"); idx != -1 {
		raw = raw[:idx]
	}
	return sanitizeText(raw)
}

// sanitizeText trims whitespace, strips carriage returns, and expands tabs to spaces.
func sanitizeText(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\t", "  ")
	return strings.TrimSpace(s)
}

// firstLine returns the first line of a string.
func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}
