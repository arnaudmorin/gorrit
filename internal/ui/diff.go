package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

type openDiffMsg struct{ id string }

type diffReadyMsg struct {
	cmd     *exec.Cmd
	cleanup func()
	err     error
}

type diffDoneMsg struct{ err error }

var magicPaths = map[string]bool{"/COMMIT_MSG": true, "/MERGE_LIST": true, "/PATCHSET_LEVEL": true}

// prepareDiff downloads what the configured command needs into a temp
// directory and returns the command ready to run from there.
func prepareDiff(client *gerrit.Client, id, command string) tea.Cmd {
	return func() tea.Msg {
		dir, err := os.MkdirTemp("", "gorrit-")
		if err != nil {
			return diffReadyMsg{err: err}
		}
		cleanup := func() { os.RemoveAll(dir) }
		cmd, err := buildDiffCommand(context.Background(), client, id, command, dir)
		if err != nil {
			cleanup()
			return diffReadyMsg{err: err}
		}
		return diffReadyMsg{cmd: cmd, cleanup: cleanup}
	}
}

func buildDiffCommand(ctx context.Context, client *gerrit.Client, id, command, dir string) (*exec.Cmd, error) {
	needTrees := strings.Contains(command, "{old}") || strings.Contains(command, "{new}")
	needPatch := strings.Contains(command, "{patch}")
	needNotes := strings.Contains(command, "{notes}")
	pipePatch := !needTrees && !needPatch

	if needNotes {
		if err := writeNotes(ctx, client, id, filepath.Join(dir, "notes.json")); err != nil {
			return nil, err
		}
	}

	if needTrees {
		if err := fetchTrees(ctx, client, id, dir); err != nil {
			return nil, err
		}
	}
	var patch []byte
	if needPatch || pipePatch {
		var err error
		if patch, err = client.GetPatch(ctx, id, gerrit.RevisionCurrent); err != nil {
			return nil, err
		}
		if needPatch {
			if err := os.WriteFile(filepath.Join(dir, "change.patch"), patch, 0o600); err != nil {
				return nil, err
			}
		}
	}

	shell := strings.NewReplacer("{old}", "a", "{new}", "b", "{patch}", "change.patch", "{notes}", "notes.json").Replace(command)
	cmd := exec.Command("sh", "-c", shell)
	cmd.Dir = dir
	if pipePatch {
		cmd.Stdin = strings.NewReader(string(patch))
	}
	return cmd, nil
}

// writeNotes stores the inline comments of the current patch set as a hunk
// agent-context sidecar.
func writeNotes(ctx context.Context, client *gerrit.Client, id, dest string) error {
	change, err := client.GetChange(ctx, id)
	if err != nil {
		return err
	}
	comments, err := client.ListComments(ctx, id)
	if err != nil {
		return err
	}
	raw, err := marshalNotes(hunkNotes(change, comments, change.CurrentRevisionNumber))
	if err != nil {
		return err
	}
	return os.WriteFile(dest, raw, 0o600)
}

// fetchTrees writes the changed files before (a/) and after (b/) the revision.
func fetchTrees(ctx context.Context, client *gerrit.Client, id, dir string) error {
	files, err := client.ListFiles(ctx, id, gerrit.RevisionCurrent)
	if err != nil {
		return err
	}
	type job struct {
		path   string
		parent int
		dest   string
	}
	var jobs []job
	for path, info := range files {
		if magicPaths[path] {
			continue
		}
		oldPath := path
		if info.OldPath != "" {
			oldPath = info.OldPath
		}
		if info.Status != "A" {
			jobs = append(jobs, job{oldPath, 1, filepath.Join(dir, "a", filepath.FromSlash(oldPath))})
		}
		if info.Status != "D" {
			jobs = append(jobs, job{path, 0, filepath.Join(dir, "b", filepath.FromSlash(path))})
		}
	}
	for _, side := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(dir, side), 0o700); err != nil {
			return err
		}
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
		sem      = make(chan struct{}, 8)
	)
	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			content, ok, err := client.GetFileContent(ctx, id, gerrit.RevisionCurrent, j.path, j.parent)
			if err == nil && ok {
				if err = os.MkdirAll(filepath.Dir(j.dest), 0o700); err == nil {
					err = os.WriteFile(j.dest, content, 0o600)
				}
			}
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", j.path, err)
				}
				mu.Unlock()
			}
		}(j)
	}
	wg.Wait()
	return firstErr
}
