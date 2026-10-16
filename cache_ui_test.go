package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestSQLiteCacheAvoidsRepeatedDetailFetches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "cache.db")
	cache, err := openCacheAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("cache permissions: %v %v", info, err)
	}
	f := &fakeRunner{}
	c := Collector{Runner: f, Config: defaultConfig(), Cache: cache}
	first, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.SaveSnapshot(first); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := cache.LoadSnapshot()
	if err != nil || !ok || len(loaded.PRs) != 2 {
		t.Fatalf("saved snapshot: %+v %v %v", loaded, ok, err)
	}
	f.mu.Lock()
	f.calls = nil
	f.mu.Unlock()
	_, err = c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	calls := append([]string(nil), f.calls...)
	f.mu.Unlock()
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "gh search prs ") {
		t.Fatalf("cached refresh calls: %v", calls)
	}
	_, err = c.CollectForced(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	calls = append([]string(nil), f.calls...)
	f.mu.Unlock()
	if len(calls) <= 1 {
		t.Fatalf("forced refresh did not fetch details: %v", calls)
	}
}

func TestJiraDescriptionsAreComplete(t *testing.T) {
	plain := jiraDescription([]byte(`"First line\nSecond line"`))
	if plain != "First line\nSecond line" {
		t.Fatalf("plain: %q", plain)
	}
	adf := []byte(`{"type":"doc","content":[{"type":"heading","content":[{"type":"text","text":"Context"}]},{"type":"paragraph","content":[{"type":"text","text":"Full description"},{"type":"hardBreak"},{"type":"text","text":"last line"}]},{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"first item"}]}]}]}]}`)
	got := jiraDescription(adf)
	for _, part := range []string{"Context", "Full description", "last line", "- first item"} {
		if !strings.Contains(got, part) {
			t.Fatalf("missing %q in %q", part, got)
		}
	}
	issue, err := parseIssue([]byte(`{"key":"AI-42","fields":{"summary":"A ticket","description":`+string(adf)+`}}`), "AI-42")
	if err != nil || issue.Description != got {
		t.Fatalf("issue description: %+v %v", issue, err)
	}
	table := jiraDescription([]byte(`{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"left"}]}]},{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"right"}]}]}]}]}]}`))
	if !strings.Contains(table, "left | right") {
		t.Fatalf("table description: %q", table)
	}
}

func TestCommentsAndFullDetailNavigation(t *testing.T) {
	m := newModel(Collector{Config: defaultConfig()}, WatchState{})
	m.width, m.height = 100, 24
	m.snapshot = Snapshot{FetchedAt: time.Now(), PRs: []PR{{ID: "a/b#1", Repo: "a/b", Number: 1, Title: "Long PR", Body: strings.Repeat("An entire paragraph with useful details. ", 30), Issues: []Issue{{Key: "AI-42", Summary: "Ticket", Description: "Beginning\n" + strings.Repeat("Jira details. ", 50) + "\nThe end of the Jira description."}}}}}
	press := func(key rune) { next, _ := m.Update(tea.KeyPressMsg{Code: key, Text: string(key)}); m = next.(model) }
	press('d')
	if !strings.Contains(m.View().Content, "An entire paragraph") {
		t.Fatal("PR description pane is missing")
	}
	press('i')
	if !strings.Contains(m.View().Content, "Beginning") {
		t.Fatal("Jira description pane is missing")
	}
	for i := 0; i < 100; i++ {
		press(']')
	}
	if !strings.Contains(m.View().Content, "The end of the Jira description") {
		t.Fatal("cannot scroll to complete Jira description")
	}
	m.commentsByPR["a/b#1"] = []Comment{{Author: "alice", Body: "First comment"}, {Author: "bob", Body: "Last review", Kind: "review"}}
	m.detailMode = 2
	m.detailOffset = 0
	if !strings.Contains(m.View().Content, "First comment") {
		t.Fatal("first comment missing")
	}
	press('n')
	if !strings.Contains(m.View().Content, "Last review") {
		t.Fatal("next comment missing")
	}
	for _, line := range strings.Split(m.View().Content, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("wrapped line too wide: %d", lipgloss.Width(line))
		}
	}
}

func TestPaginatedComments(t *testing.T) {
	items, err := parseInlineComments([]byte(`[[{"user":{"login":"alice"},"body":"first","created_at":"2026-10-16T01:00:00Z","path":"main.go","line":12}],[{"user":{"login":"bob"},"body":"second","created_at":"2026-10-16T02:00:00Z"}]]`))
	if err != nil || len(items) != 2 || items[0].Path != "main.go" || items[0].Line != 12 || items[1].Body != "second" {
		t.Fatalf("comments: %+v %v", items, err)
	}
}

func TestCommentsFetchAndCache(t *testing.T) {
	cache, err := openCacheAt(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	f := &fakeRunner{}
	c := Collector{Runner: f, Config: defaultConfig(), Cache: cache}
	pr := PR{ID: "acme/api#12", Repo: "acme/api", Number: 12}
	comments, err := c.LoadComments(context.Background(), pr, false)
	if err != nil || len(comments) != 3 || comments[0].Path != "main.go" || comments[1].Kind != "review" || comments[2].Author != "alice" {
		t.Fatalf("comments: %+v %v", comments, err)
	}
	f.mu.Lock()
	f.calls = nil
	f.mu.Unlock()
	comments, err = c.LoadComments(context.Background(), pr, false)
	if err != nil || len(comments) != 3 {
		t.Fatalf("cached comments: %+v %v", comments, err)
	}
	f.mu.Lock()
	calls := append([]string(nil), f.calls...)
	f.mu.Unlock()
	if len(calls) != 0 {
		t.Fatalf("cache made API calls: %v", calls)
	}
}

func TestHiddenHTMLCommentIsNotShown(t *testing.T) {
	got := commentDisplayBody("<!-- GitHub bot metadata -->\nVisible review feedback")
	if got != "Visible review feedback" {
		t.Fatalf("visible body: %q", got)
	}
}

func TestNarrowViewFitsTerminal(t *testing.T) {
	m := newModel(Collector{Config: defaultConfig()}, WatchState{})
	m.width, m.height = 70, 20
	m.snapshot = Snapshot{PRs: []PR{{ID: "a/b#1", Repo: "a/b", Number: 1, Title: strings.Repeat("Long title ", 30), Body: strings.Repeat("Long body ", 30)}}}
	for _, mode := range []int{0, 1, 2, 3, 4} {
		m.detailMode = mode
		view := m.View().Content
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > m.width {
				t.Fatalf("mode %d line width %d exceeds %d", mode, lipgloss.Width(line), m.width)
			}
		}
		if lipgloss.Height(view) > m.height {
			t.Fatalf("mode %d view height exceeds terminal", mode)
		}
	}
}
