package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "get home dir: %v\n", err)
		os.Exit(1)
	}

	cmdName, appDataDir := detectAgentDir(home)

	versionCh := make(chan string, 1)
	go func() {
		versionCh <- fetchAgentVersion(cmdName)
	}()

	sessions, err := loadSessions(appDataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load sessions: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(newModel(sessions, <-versionCh), tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "run tui: %v\n", err)
		os.Exit(1)
	}

	m, ok := finalModel.(model)
	if !ok || m.selectedID == "" {
		return
	}

	binPath, err := findAgentBin(cmdName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "find %s binary: %v\n", cmdName, err)
		os.Exit(1)
	}

	if err := syscall.Exec(binPath, []string{binPath, "--conversation", m.selectedID}, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "exec %s: %v\n", cmdName, err)
		os.Exit(1)
	}
}

// detectAgentDir selects jetski if ~/.gemini/jetski exists, otherwise falls back to agy.
func detectAgentDir(home string) (cmdName, appDataDir string) {
	jetskiDir := filepath.Join(home, ".gemini", "jetski")
	if info, err := os.Stat(jetskiDir); err == nil && info.IsDir() {
		return "jetski", jetskiDir
	}
	return "agy", filepath.Join(home, ".gemini", "antigravity-cli")
}

// shortcut: only checks standard gLinux/gMac jetski paths, parse shell rc if custom alias paths are needed
func findAgentBin(cmdName string) (string, error) {
	if cmdName == "jetski" {
		for _, p := range []string{"/google/bin/releases/jetski-devs/tools/cli", "/usr/local/bin/jetski"} {
			if bin, err := exec.LookPath(p); err == nil {
				return bin, nil
			}
		}
	}
	return exec.LookPath(cmdName)
}

// fetchAgentVersion runs <cmdName> --version and formats the version string.
func fetchAgentVersion(cmdName string) string {
	binPath, err := findAgentBin(cmdName)
	if err != nil {
		return cmdName
	}
	out, err := exec.Command(binPath, "--version").Output()
	if err != nil {
		return cmdName
	}
	v := firstLine(string(out))
	if v == "" {
		return cmdName
	}
	if strings.HasPrefix(strings.ToLower(v), cmdName) {
		return v
	}
	return cmdName + " " + v
}
