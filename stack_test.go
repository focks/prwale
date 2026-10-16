package main

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestParseNativeStacks(t *testing.T) {
	data := []byte(`[{"id":1,"number":42,"open":true,"base":{"ref":"main"},"pull_requests":[{"number":10,"state":"closed","merged_at":"2026-10-10T00:00:00Z","head":{"ref":"a"}},{"number":11,"state":"open","draft":false,"head":{"ref":"b"}}]},{"number":43,"open":false,"base":{"ref":"main"},"pull_requests":[{"number":12,"state":"open","head":{"ref":"c"}}]}][]`)
	stacks, err := parseStacks("acme/api", data)
	if err != nil || len(stacks) != 1 {
		t.Fatalf("stacks: %+v %v", stacks, err)
	}
	s := stacks[0]
	if !s.Native || s.Number != 42 || s.Base != "main" || s.Entries[0].State != "merged" || s.Position(11) != 2 {
		t.Fatalf("stack: %+v", s)
	}
}

func TestInferStacksFromBranchChains(t *testing.T) {
	now := time.Now()
	prs := []PR{
		{ID: "a/r#3", Repo: "a/r", Number: 3, Branch: "top", BaseRef: "middle", UpdatedAt: now},
		{ID: "a/r#1", Repo: "a/r", Number: 1, Branch: "bottom", BaseRef: "main", UpdatedAt: now},
		{ID: "a/r#2", Repo: "a/r", Number: 2, Branch: "middle", BaseRef: "bottom", UpdatedAt: now},
		{ID: "a/r#9", Repo: "a/r", Number: 9, Branch: "solo", BaseRef: "development", UpdatedAt: now},
		{ID: "b/r#1", Repo: "b/r", Number: 1, Branch: "x", BaseRef: "bottom", UpdatedAt: now}, // other repo: not a child
	}
	attachStacks(prs, nil)
	for _, pr := range prs {
		switch pr.Number {
		case 1, 2, 3:
			if pr.Repo == "a/r" && (pr.Stack == nil || pr.Stack.Native || pr.Stack.Base != "main" || pr.Stack.Position(pr.Number) != pr.Number || len(pr.Stack.Entries) != 3) {
				t.Fatalf("PR %d stack: %+v", pr.Number, pr.Stack)
			}
		case 9:
			if pr.Stack != nil {
				t.Fatal("a lone PR on a non-main base is not a stack")
			}
		}
	}
	if prs[4].Stack != nil {
		t.Fatal("cross-repo branch name matched")
	}
}

func TestNativeStackWinsOverInferred(t *testing.T) {
	prs := []PR{
		{ID: "a/r#1", Repo: "a/r", Number: 1, Title: "base", Branch: "b1", BaseRef: "main"},
		{ID: "a/r#2", Repo: "a/r", Number: 2, Title: "top", Branch: "b2", BaseRef: "b1"},
	}
	native := map[string][]Stack{"a/r": {{Key: "a/r#stack7", Number: 7, Native: true, Base: "main", Entries: []StackEntry{{Number: 1, State: "open"}, {Number: 2, State: "open"}, {Number: 5, State: "open", Head: "teammate"}}}}}
	attachStacks(prs, native)
	if prs[1].Stack == nil || !prs[1].Stack.Native || len(prs[1].Stack.Entries) != 3 || prs[1].Stack.Entries[1].Title != "top" {
		t.Fatalf("native stack: %+v", prs[1].Stack)
	}
}

func stackModel() model {
	m := newModel(Collector{Config: defaultConfig()}, WatchState{})
	m.width, m.height = 140, 40
	m.loading = false
	prs := []PR{
		{ID: "a/r#1", Repo: "a/r", Number: 1, Title: "feat: fixtures", Branch: "b1", BaseRef: "main", Checks: "2 failed · 3 passed", UpdatedAt: time.Now()},
		{ID: "a/r#2", Repo: "a/r", Number: 2, Title: "feat: contract", Branch: "b2", BaseRef: "b1", Checks: "5 passed", Review: "REVIEW_REQUIRED", UpdatedAt: time.Now()},
		{ID: "a/r#3", Repo: "a/r", Number: 3, Title: "fix: unrelated", Branch: "c", BaseRef: "main", Checks: "5 passed", UpdatedAt: time.Now()},
	}
	attachStacks(prs, nil)
	m.snapshot = Snapshot{FetchedAt: time.Now(), PRs: prs}
	return m
}

func TestStacksQueueAndMap(t *testing.T) {
	m := stackModel()
	press := func(key string) {
		code := []rune(key)[0]
		next, _ := m.Update(tea.KeyPressMsg{Code: code, Text: key})
		m = next.(model)
	}
	press("5")
	if got := len(m.visible()); got != 2 {
		t.Fatalf("stacks queue shows %d PRs", got)
	}
	if top, _ := m.chosen(); top.Number != 2 {
		t.Fatalf("stack not listed top-down: first is #%d", top.Number)
	}
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"Stacks 2", "STACK", "→ main", "stack 2/2", "◂ this", "blocked below by #1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	press("{")
	if pr, _ := m.chosen(); pr.Number != 1 {
		t.Fatalf("{ did not move down the stack: #%d", pr.Number)
	}
	press("}")
	if pr, _ := m.chosen(); pr.Number != 2 {
		t.Fatalf("} did not move up the stack: #%d", pr.Number)
	}
	for _, line := range strings.Split(m.View().Content, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("line too wide: %d", lipgloss.Width(line))
		}
	}
}

func TestStackMoveWidensFilter(t *testing.T) {
	m := stackModel()
	m.filter = 1 // attention: only #1 visible
	m.selected = 0
	if pr, _ := m.chosen(); pr.Number != 1 {
		t.Fatalf("setup: #%d", pr.Number)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: '}', Text: "}"})
	m = next.(model)
	if pr, _ := m.chosen(); pr.Number != 2 || m.filter != 0 {
		t.Fatalf("} should widen to all and select #2: #%d filter %d", pr.Number, m.filter)
	}
}

func TestCollectorAttachesNativeStacks(t *testing.T) {
	t.Setenv("PRWALE_HOME", t.TempDir())
	c := Collector{Runner: &fakeRunner{}, Config: defaultConfig()}
	snap, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, pr := range snap.PRs {
		if pr.Number == 12 && (pr.Stack == nil || !pr.Stack.Native || pr.Stack.Number != 5 || pr.Stack.Position(12) != 1 || len(pr.Stack.Entries) != 2) {
			t.Fatalf("#12 stack: %+v", pr.Stack)
		}
		if pr.Number == 13 && pr.Stack != nil {
			t.Fatalf("#13 should have no stack (API unavailable for acme/web): %+v", pr.Stack)
		}
	}
}
