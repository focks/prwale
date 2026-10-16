package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type fakeRunner struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	command := name + " " + strings.Join(args, " ")
	f.mu.Lock()
	f.calls = append(f.calls, command)
	f.mu.Unlock()
	switch {
	case strings.HasPrefix(command, "gh search prs "):
		return []byte(`[{"number":12,"title":"AI-42 improve sync","body":"See AI-42","url":"https://github.com/acme/api/pull/12","updatedAt":"2026-10-16T00:00:00Z","commentsCount":2,"repository":{"nameWithOwner":"acme/api"}},{"number":13,"title":"other fix","body":"Jira: AI-42","url":"https://github.com/acme/web/pull/13","updatedAt":"2026-10-16T00:00:00Z","repository":{"nameWithOwner":"acme/web"}}]`), nil
	case strings.HasPrefix(command, "gh pr view 12 "):
		return []byte(`{"headRefName":"feature_AI-42_sync","baseRefName":"main","reviewDecision":"REVIEW_REQUIRED","statusCheckRollup":[{"__typename":"CheckRun","status":"COMPLETED","conclusion":"SUCCESS"},{"__typename":"StatusContext","state":"PENDING"}]}`), nil
	case strings.HasPrefix(command, "gh pr view 13 "):
		return []byte(`{"headRefName":"fix-other","baseRefName":"main","statusCheckRollup":[]}`), nil
	case command == "gh api -H X-GitHub-Api-Version: 2026-03-10 --paginate repos/acme/api/stacks?per_page=100":
		return []byte(`[{"number":5,"open":true,"base":{"ref":"main"},"pull_requests":[{"number":12,"state":"open","head":{"ref":"feature_AI-42_sync"}},{"number":20,"state":"open","draft":true,"head":{"ref":"teammate"}}]}]`), nil
	case command == "jira project list":
		return []byte("KEY\tNAME\tTYPE\nAI\tEngineering\tclassic\n"), nil
	case command == "jira issue view AI-42 --raw":
		return []byte(`{"key":"AI-42","self":"https://acme.atlassian.net/rest/api/3/issue/42","fields":{"summary":"Sync reliability","status":{"name":"In Progress"},"assignee":{"displayName":"Dev"},"updated":"2026-10-16T01:00:00Z"}}`), nil
	case strings.HasPrefix(command, "gh api repos/acme/api/issues/12/comments?"):
		return []byte(`[[{"user":{"login":"alice"},"body":"Conversation note","created_at":"2026-10-16T01:00:00Z"}]]`), nil
	case strings.HasPrefix(command, "gh api repos/acme/api/pulls/12/reviews?"):
		return []byte(`[[{"user":{"login":"bob"},"body":"Looks good","submitted_at":"2026-10-16T02:00:00Z","state":"APPROVED"}]]`), nil
	case strings.HasPrefix(command, "gh api repos/acme/api/pulls/12/comments?"):
		return []byte(`[[{"user":{"login":"carol"},"body":"Inline note","created_at":"2026-10-16T03:00:00Z","path":"main.go","line":22}]]`), nil
	}
	return nil, errors.New("unexpected command: " + command)
}

func TestCollectorLinksAndDeduplicatesJira(t *testing.T) {
	f := &fakeRunner{}
	c := Collector{Runner: f, Config: defaultConfig()}
	s, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.PRs) != 2 || len(s.PRs[0].Issues) != 1 || s.PRs[0].Issues[0].Status != "In Progress" {
		t.Fatalf("unexpected snapshot: %+v", s)
	}
	if s.PRs[0].Checks != "1 pending · 1 passed" {
		t.Fatalf("checks: %q", s.PRs[0].Checks)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, call := range f.calls {
		if call == "jira issue view AI-42 --raw" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("Jira fetched %d times", count)
	}
}

func TestDetailUsesFullPRBody(t *testing.T) {
	pr := PR{Body: "short", Title: "No key"}
	if err := parseDetail([]byte(`{"body":"Full description with Jira: AI-42\nAnd another paragraph.","headRefName":"feature"}`), &pr); err != nil {
		t.Fatal(err)
	}
	if pr.Body != "Full description with Jira: AI-42\nAnd another paragraph." || !slices.Equal(pr.JiraKeys, []string{"AI-42"}) {
		t.Fatalf("full PR body: %+v", pr)
	}
}

func TestFindNumberOnlyReturnsExactAuthoredPRs(t *testing.T) {
	f := &fakeRunner{}
	c := Collector{Runner: f, Config: defaultConfig()}
	matches, truncated, err := c.FindNumber(context.Background(), 12)
	if err != nil || truncated || len(matches) != 1 || matches[0].ID != "acme/api#12" {
		t.Fatalf("matches: %+v, truncated=%v, err=%v", matches, truncated, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 1 || !strings.Contains(f.calls[0], "--author @me") || !strings.Contains(f.calls[0], "--limit 1000") {
		t.Fatalf("calls: %v", f.calls)
	}
}

func TestKeyExtraction(t *testing.T) {
	got := keysFromPR("feature_AI-42_sync", "AI-42 / DBX-19, prod-1", "example STEP-1\nJira: aai-99\nhttps://acme.atlassian.net/browse/AI-42")
	want := []string{"AI-42", "DBX-19", "AAI-99"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestProjectListParsing(t *testing.T) {
	keys, err := parseProjectKeys([]byte("KEY\tNAME\tTYPE\nAI\tEngineering\tclassic\nMAR\tMarketing\tnext-gen\n"))
	if err != nil || !keys["AI"] || !keys["MAR"] || keys["KEY"] {
		t.Fatalf("projects: %v, %v", keys, err)
	}
}

func TestChangeDetectionAndFailedIssue(t *testing.T) {
	base := Snapshot{FetchedAt: time.Now(), PRs: []PR{{ID: "acme/api#12", Title: "Sync", HeadSHA: "old", Review: "not reviewed", Checks: "1 pending", Comments: 1, Issues: []Issue{{Key: "AI-42", Status: "To Do", Updated: "old"}}}}}
	state := compareSnapshot(&base, WatchState{Version: 1, PRs: map[string]prState{}})
	if len(state.RecentEvents) != 0 {
		t.Fatal("first sync should only establish baseline")
	}
	next := Snapshot{FetchedAt: time.Now(), PRs: []PR{{ID: "acme/api#12", Title: "Sync", HeadSHA: "new", Review: "APPROVED", ReviewCount: 1, Checks: "1 passed", Comments: 3, Issues: []Issue{{Key: "AI-42", Status: "In Progress", Updated: "new"}}}}}
	state = compareSnapshot(&next, state)
	if len(next.PRs[0].Changes) != 6 {
		t.Fatalf("changes: %v", next.PRs[0].Changes)
	}
	failed := Snapshot{FetchedAt: time.Now(), PRs: []PR{{ID: "acme/api#12", Title: "Sync", DetailError: "offline", Issues: []Issue{{Key: "AI-42", Error: "offline"}}}}}
	state2 := compareSnapshot(&failed, state)
	if len(failed.PRs[0].Changes) != 0 {
		t.Fatalf("false change on failure: %v", failed.PRs[0].Changes)
	}
	if state2.PRs["acme/api#12"].Issues["AI-42"].Status != "In Progress" {
		t.Fatal("lost last known Jira status")
	}
	truncated := Snapshot{FetchedAt: time.Now(), Truncated: true}
	state3 := compareSnapshot(&truncated, state2)
	if _, ok := state3.PRs["acme/api#12"]; !ok {
		t.Fatal("capped search must not mark an unseen PR closed")
	}
}

func TestSearchAndNavigation(t *testing.T) {
	m := newModel(Collector{Config: defaultConfig()}, WatchState{Version: 1, PRs: map[string]prState{}})
	m.snapshot = Snapshot{PRs: []PR{{ID: "one#1", Repo: "one", Number: 1, Title: "Alpha"}, {ID: "two#2", Repo: "two", Number: 2, Title: "Beta", Issues: []Issue{{Key: "AI-42", Summary: "Search me"}}}}}
	press := func(code rune) {
		key := tea.KeyPressMsg{Code: code}
		if code >= 32 && code < 127 {
			key.Text = string(code)
		}
		next, _ := m.Update(key)
		m = next.(model)
	}
	press('j')
	if m.selected != 1 {
		t.Fatalf("selected %d", m.selected)
	}
	press('/')
	for _, r := range "AI-42" {
		press(r)
	}
	if len(m.visible()) != 1 || m.visible()[0] != 1 {
		t.Fatalf("search results %v", m.visible())
	}
	if !strings.Contains(m.View().Content, "AI-42") {
		t.Fatal("search prompt absent")
	}
	press(tea.KeyEsc)
	if m.query != "" {
		t.Fatal("Esc did not clear search")
	}
	press('#')
	press('2')
	if len(m.visible()) != 1 || m.visible()[0] != 1 {
		t.Fatalf("number results %v", m.visible())
	}
	press(tea.KeyEnter)
	if !m.numberExact {
		t.Fatal("Enter did not make number exact")
	}
	press(tea.KeyEsc)
	if m.numberQuery != "" {
		t.Fatal("Esc did not clear number")
	}
}

func TestInitPreservesConfigurationAndPrivatePermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	t.Setenv("PRWALE_HOME", dir)
	_, path, created, err := initConfig()
	if err != nil || !created {
		t.Fatalf("init: %v, created=%v", err, created)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("config mode: %v, %v", info, err)
	}
	if info, err = os.Stat(dir); err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("dir mode: %v, %v", info, err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"poll_seconds":30,"max_prs":10,"command_timeout_seconds":10}`), 0600); err != nil {
		t.Fatal(err)
	}
	c, _, created, err := initConfig()
	if err != nil || created || c.PollSeconds != 30 || c.CommentsTTLSeconds != 300 || c.JiraTTLSeconds != 300 {
		t.Fatalf("existing config overwritten: %+v %v %v", c, created, err)
	}
}

func TestWideViewFitsTerminal(t *testing.T) {
	m := newModel(Collector{Config: defaultConfig()}, WatchState{Version: 1, PRs: map[string]prState{}})
	m.width, m.height = 100, 28
	m.snapshot = Snapshot{PRs: []PR{{ID: "a/b#1", Repo: "a/b", Number: 1, Title: "A useful PR", Checks: "1 passed"}}}
	for _, line := range strings.Split(m.View().Content, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("line width %d exceeds %d: %q", lipgloss.Width(line), m.width, line)
		}
	}
	if got := lipgloss.Height(m.View().Content); got > m.height {
		t.Fatalf("view height %d exceeds %d", got, m.height)
	}
}
