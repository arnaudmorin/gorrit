package ui

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/arnaudmorin/gorrit/internal/gerrit"
)

// chainPos places a change in its relation chain; size is 0 when the change
// stands alone.
type chainPos struct {
	base  int // change number at the bottom, names the chain
	index int // 1 at the bottom
	size  int
}

type chainEntry struct {
	updated time.Time
	pos     chainPos
}

type chainsLoadedMsg struct {
	entries map[int]chainEntry
}

var chainColors = []lipgloss.AdaptiveColor{cSection, cBot, cYellow, cCyan}

func (p chainPos) tag() string {
	return fmt.Sprintf("%d/%d", p.index, p.size)
}

func (p chainPos) style() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(chainColors[p.base%len(chainColors)])
}

// loadChains looks up the relation chain of the changes not in known or
// updated since. One answer covers every listed change of that chain.
func loadChains(client *gerrit.Client, changes []gerrit.ChangeInfo, known map[int]chainEntry) tea.Cmd {
	var todo []gerrit.ChangeInfo
	updated := map[int]time.Time{}
	for _, c := range changes {
		if e, ok := known[c.Number]; ok && e.updated.Equal(c.Updated.Time) {
			continue
		}
		todo = append(todo, c)
		updated[c.Number] = c.Updated.Time
	}
	if len(todo) == 0 {
		return nil
	}
	return func() tea.Msg {
		var (
			wg  sync.WaitGroup
			mu  sync.Mutex
			out = map[int]chainEntry{}
			sem = make(chan struct{}, 8)
		)
		for _, c := range todo {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				mu.Lock()
				_, done := out[c.Number]
				mu.Unlock()
				if done {
					return
				}
				related, err := client.GetRelated(context.Background(), gerrit.ChangeID(c.Project, c.Number), gerrit.RevisionCurrent)
				if err != nil {
					return
				}
				mu.Lock()
				defer mu.Unlock()
				out[c.Number] = chainEntry{updated: updated[c.Number]}
				for i, r := range related {
					if t, listed := updated[r.ChangeNumber]; listed {
						base := related[len(related)-1].ChangeNumber
						out[r.ChangeNumber] = chainEntry{updated: t, pos: chainPos{base: base, index: len(related) - i, size: len(related)}}
					}
				}
			}()
		}
		wg.Wait()
		return chainsLoadedMsg{entries: out}
	}
}

// threadChains groups the members of a chain together, top first, and orders
// the groups by their latest update. The cursor stays on the same change.
func (m *listModel) threadChains() {
	if len(m.changes) == 0 {
		return
	}
	more := m.hasMore()
	selected := m.changes[m.cursor].Number
	group := func(c gerrit.ChangeInfo) int {
		if p := m.chains[c.Number].pos; p.size > 1 {
			return p.base
		}
		return -c.Number
	}
	latest := map[int]time.Time{}
	first := map[int]int{}
	for i, c := range m.changes {
		g := group(c)
		if c.Updated.After(latest[g]) {
			latest[g] = c.Updated.Time
		}
		if _, ok := first[g]; !ok {
			first[g] = i
		}
	}
	slices.SortStableFunc(m.changes, func(a, b gerrit.ChangeInfo) int {
		ga, gb := group(a), group(b)
		if ga != gb {
			if c := latest[gb].Compare(latest[ga]); c != 0 {
				return c
			}
			return cmp.Compare(first[ga], first[gb])
		}
		return cmp.Compare(m.chains[b.Number].pos.index, m.chains[a.Number].pos.index)
	})
	for i := range m.changes {
		m.changes[i].MoreChanges = false
		if m.changes[i].Number == selected {
			m.cursor = i
		}
	}
	m.changes[len(m.changes)-1].MoreChanges = more
}
