package ui

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

func TestDownloadRunOffersShell(t *testing.T) {
	d := newDownload(map[string]gerrit.FetchInfo{"ssh": {Commands: map[string]string{"Checkout": "echo checked-out"}}})
	var captured *exec.Cmd
	d.exec = func(cmd *exec.Cmd, _ tea.ExecCallback) tea.Cmd {
		captured = cmd
		return nil
	}
	d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if captured == nil {
		t.Fatal("no command built")
	}
	captured.Env = append(captured.Environ(), "SHELL=sh")
	captured.Stdin = strings.NewReader("s\necho in-shell\nexit\n")
	out, err := captured.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{"$ echo checked-out", "checked-out", "s for a shell", "in-shell"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	captured2 := exec.Command(captured.Path, captured.Args[1:]...)
	captured2.Stdin = strings.NewReader("\n")
	out, err = captured2.CombinedOutput()
	if err != nil || strings.Contains(string(out), "in-shell") {
		t.Fatalf("enter should return without a shell: %v\n%s", err, out)
	}
}

func TestDownloadTempRepo(t *testing.T) {
	d := newDownload(map[string]gerrit.FetchInfo{"ssh": {Commands: map[string]string{
		"Checkout":    "git fetch https://example.invalid/p refs/changes/1/1/1 && git checkout FETCH_HEAD",
		"Cherry Pick": "git cherry-pick FETCH_HEAD",
	}}})
	var captured *exec.Cmd
	d.exec = func(cmd *exec.Cmd, _ tea.ExecCallback) tea.Cmd {
		captured = cmd
		return nil
	}
	d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if captured == nil || captured.Dir == "" {
		t.Fatal("expected a command in a temp dir")
	}
	script := captured.Args[2]
	if !strings.Contains(script, "git init -q && git fetch --depth 2 https://example.invalid/p") {
		t.Fatalf("script: %s", script)
	}
	if !strings.Contains(script, `exec "${SHELL:-sh}"`) || strings.Contains(script, "Press any key") {
		t.Fatalf("temp repo must open a shell directly: %s", script)
	}
	if !strings.HasPrefix(captured.Dir, os.TempDir()) {
		t.Fatalf("dir %q not temporary", captured.Dir)
	}
	os.RemoveAll(captured.Dir)

	captured = nil
	d.cursor = 0
	d.names = []string{"Cherry Pick"}
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if captured != nil {
		t.Fatal("cherry-pick must not run in a temp repo")
	}
	if msg, ok := cmd().(downloadDoneMsg); !ok || !strings.Contains(msg.notice, "existing clone") {
		t.Fatalf("expected a notice, got %v", msg)
	}
}
