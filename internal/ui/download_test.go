package ui

import (
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
