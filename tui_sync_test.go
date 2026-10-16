package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestPeriodicSyncReschedulesAfterSuccessAndFailure(t *testing.T) {
	t.Setenv("PRWALE_HOME", t.TempDir())
	f := &fakeRunner{}
	c := Collector{Runner: f, Config: defaultConfig()}
	initial := Snapshot{FetchedAt: time.Now(), PRs: []PR{{ID: "acme/api#12", Repo: "acme/api", Number: 12, Title: "saved"}}}
	m := newModel(c, WatchState{Version: 1, PRs: map[string]prState{}}, initial)
	if m.loading || m.nextSync.IsZero() || m.Init() == nil {
		t.Fatal("saved snapshot did not schedule next sync")
	}
	loaded, cmd := m.Update(tickMsg{generation: m.syncGeneration})
	m = loaded.(model)
	if !m.loading || cmd == nil || !m.nextSync.IsZero() {
		t.Fatal("scheduled tick did not start refresh")
	}
	result := cmd()
	loaded, cmd = m.Update(result)
	m = loaded.(model)
	if m.loading || cmd == nil || m.nextSync.Before(time.Now()) || m.showingCache {
		t.Fatalf("successful sync not rescheduled: %+v", m)
	}
	staleGeneration := m.syncGeneration - 1
	loaded, staleCmd := m.Update(tickMsg{generation: staleGeneration})
	m = loaded.(model)
	if m.loading || staleCmd != nil {
		t.Fatal("obsolete timer started a second sync")
	}
	loaded, cmd = m.Update(tickMsg{generation: m.syncGeneration})
	m = loaded.(model)
	if !m.loading || cmd == nil {
		t.Fatal("second scheduled sync did not start")
	}
	loaded, cmd = m.Update(fetchedMsg{err: errors.New("offline")})
	m = loaded.(model)
	if m.loading || cmd == nil || m.nextSync.Before(time.Now()) || !strings.Contains(m.message, "Refresh failed") {
		t.Fatal("failed sync did not schedule retry")
	}
	if len(m.snapshot.PRs) == 0 {
		t.Fatal("failure discarded saved PRs")
	}
}

func TestCardsAndFilteredScrolling(t *testing.T) {
	m := newModel(Collector{Config: defaultConfig()}, WatchState{})
	m.width, m.height = 120, 28
	for i := 1; i <= 30; i++ {
		title := "other"
		if i%2 == 0 {
			title = "match"
		}
		m.snapshot.PRs = append(m.snapshot.PRs, PR{ID: fmt.Sprintf("acme/api#%d", i), Repo: "acme/api", Number: i, Title: title, Checks: "2 failed · 8 passed", JiraKeys: []string{"AI-42"}, Issues: []Issue{{Key: "AI-42", Status: "In Progress", Summary: "Important Jira context"}}})
	}
	m.query = "match"
	m.clamp()
	visible := m.visible()
	if len(visible) != 15 {
		t.Fatalf("filtered results: %d", len(visible))
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if cmd != nil {
		t.Fatal("unexpected navigation command")
	}
	m = updated.(model)
	if m.selected != 14 || m.offset == 0 {
		t.Fatalf("filtered scroll: selected=%d offset=%d", m.selected, m.offset)
	}
	selected, _ := m.chosen()
	if selected.Number%2 != 0 {
		t.Fatalf("selected PR fell outside filter: %+v", selected)
	}
	lines := strings.Join(m.listLines(50, m.height-5), "\n")
	if !strings.Contains(lines, "AI-42") || !strings.Contains(lines, fmt.Sprintf("#%d", selected.Number)) {
		t.Fatal("card did not show selected PR and Jira context")
	}
	updated, _ = m.Update(tea.MouseWheelMsg{X: 4, Y: 8, Button: tea.MouseWheelUp})
	m = updated.(model)
	if m.selected != 11 || m.query != "match" {
		t.Fatalf("wheel did not scroll filtered queue: selected=%d query=%q", m.selected, m.query)
	}
	for _, line := range strings.Split(m.View().Content, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("card overflows: %d", lipgloss.Width(line))
		}
	}
}

func TestJiraOpenFollowsSelectedIssue(t *testing.T) {
	m := newModel(Collector{Config: defaultConfig()}, WatchState{})
	m.snapshot.PRs = []PR{{ID: "a/b#1", Repo: "a/b", Number: 1, JiraKeys: []string{"AI-1", "AI-2"}, Issues: []Issue{{Key: "AI-1"}, {Key: "AI-2"}}}}
	if m.jiraToOpen() != "AI-1" {
		t.Fatal("default Jira target")
	}
	m.detailMode = 3
	m.jiraIndex = 1
	if m.jiraToOpen() != "AI-2" {
		t.Fatal("Jira pane did not target displayed issue")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'J', Text: "J"})
	if cmd == nil || next.(model).jiraToOpen() != "AI-2" {
		t.Fatal("J key did not prepare Jira open")
	}
}
