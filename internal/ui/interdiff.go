package ui

import (
	"context"
	"fmt"
	"sync"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

const diffContext = 3

// loadInterdiff fetches the current patch set against patch set base, one
// file diff at a time since the patch endpoint has no base.
func loadInterdiff(ctx context.Context, client *gerrit.Client, id string, base int) ([]diffFile, error) {
	infos, err := client.ListFiles(ctx, id, gerrit.RevisionCurrent, base)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range sortedKeys(infos) {
		if !magicPaths[p] {
			paths = append(paths, p)
		}
	}
	files := make([]diffFile, len(paths))
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
		sem      = make(chan struct{}, 8)
	)
	for i, p := range paths {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			d, err := client.GetDiff(ctx, id, gerrit.RevisionCurrent, p, base)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", p, err)
				}
				mu.Unlock()
				return
			}
			files[i] = fileFromDiff(p, infos[p], d)
		}()
	}
	wg.Wait()
	return files, firstErr
}

// fileFromDiff cuts a whole-file Gerrit diff into hunks with diffContext
// lines of context.
func fileFromDiff(path string, info gerrit.FileInfo, d *gerrit.DiffInfo) diffFile {
	f := diffFile{path: path, oldPath: info.OldPath, status: info.Status, binary: d.Binary}
	if f.status == "" {
		f.status = "M"
	}
	if f.oldPath == "" {
		f.oldPath = path
	}
	var all []diffLine
	gap := map[int]bool{}
	oldLine, newLine := 1, 1
	for _, c := range d.Content {
		if c.Skip > 0 {
			gap[len(all)] = true
			oldLine += c.Skip
			newLine += c.Skip
			continue
		}
		for _, t := range c.AB {
			all = append(all, diffLine{kind: ' ', old: oldLine, new: newLine, text: t})
			oldLine++
			newLine++
		}
		for _, t := range c.A {
			all = append(all, diffLine{kind: '-', old: oldLine, text: t})
			oldLine++
		}
		for _, t := range c.B {
			all = append(all, diffLine{kind: '+', new: newLine, text: t})
			newLine++
		}
	}
	keep := make([]bool, len(all))
	for i, l := range all {
		if l.kind == ' ' {
			continue
		}
		keep[i] = true
		for j := i - 1; j >= max(i-diffContext, 0) && !gap[j+1]; j-- {
			keep[j] = true
		}
		for j := i + 1; j <= min(i+diffContext, len(all)-1) && !gap[j]; j++ {
			keep[j] = true
		}
	}
	for i := 0; i < len(all); {
		if !keep[i] {
			i++
			continue
		}
		j := i + 1
		for j < len(all) && keep[j] && !gap[j] {
			j++
		}
		f.lines = append(f.lines, hunkHeader(all[i:j]))
		f.lines = append(f.lines, all[i:j]...)
		i = j
	}
	return f
}

func hunkHeader(lines []diffLine) diffLine {
	oldStart, newStart, oldCount, newCount := 0, 0, 0, 0
	for _, l := range lines {
		if l.old > 0 {
			if oldStart == 0 {
				oldStart = l.old
			}
			oldCount++
		}
		if l.new > 0 {
			if newStart == 0 {
				newStart = l.new
			}
			newCount++
		}
	}
	return diffLine{kind: '@', text: fmt.Sprintf("@@ -%d,%d +%d,%d @@", oldStart, oldCount, newStart, newCount)}
}
