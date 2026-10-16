package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// TestRenderPreview writes ANSI frames of the TUI over fixture data so a
// design change can be looked at without GitHub or Jira. It only runs when
// PRWALE_PREVIEW_DIR is set:
//
//	PRWALE_PREVIEW_DIR=/tmp/prwale-preview go test -run TestRenderPreview .
//	cat /tmp/prwale-preview/160x44_all_overview.ans
//
// See skills/prwale-tui-review for turning frames into screenshots.
func TestRenderPreview(t *testing.T) {
	dir := os.Getenv("PRWALE_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set PRWALE_PREVIEW_DIR to write preview frames")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ago := func(h float64) time.Time { return now.Add(-time.Duration(h * float64(time.Hour))) }
	issue := Issue{Key: "DEMO-42", Status: "To Do", Summary: "Improve the import pipeline and add reference examples", Assignee: "Example Developer", Priority: "Medium",
		Description: "## Context\nThe import pipeline needs **reference examples**.\n\n- [x] contract tests\n- [ ] golden snapshots\n\n| Window | Factor |\n| --- | --- |\n| 30 day | 365 / 29 |"}
	body := "<!-- template -->\n## What / How\nPins the existing `build_import_report` contract with a local fixture. See [DEMO-42](https://example.atlassian.net/browse/DEMO-42).\n\n- Independent of #7214\n  - fixtures in `tests/fixtures/`\n\n> [!NOTE]\n> The `365 / 29` factor is pinned on purpose.\n\n```python\nassert report.factor(30) == 365 / 29\n```\n"
	prs := []PR{
		{ID: "acme/service#7214", Repo: "acme/service", Number: 7214, Title: "feat: [DEMO-42] add import report", Branch: "report", BaseRef: "main", Checks: "1 failed · 15 passed", Review: "REVIEW_REQUIRED", MergeState: "BLOCKED", Comments: 9, UpdatedAt: ago(8), JiraKeys: []string{"DEMO-42"}, Issues: []Issue{issue}, Body: body},
		{ID: "acme/service#7234", Repo: "acme/service", Number: 7234, Title: "test: [DEMO-42] pin import report contract", Branch: "report-tests", BaseRef: "report", Checks: "2 failed · 16 passed · 1 pending", Review: "REVIEW_REQUIRED", MergeState: "BLOCKED", Comments: 3, UpdatedAt: ago(6), JiraKeys: []string{"DEMO-42"}, Issues: []Issue{issue}, Body: body, Changes: []string{"1 new comment(s)"}},
		{ID: "acme/tools#222", Repo: "acme/tools", Number: 222, Title: "upkeep: update retry policy", MergeState: "DIRTY", Checks: "4 passed", Comments: 2, UpdatedAt: ago(48)},
		{ID: "acme/service#7181", Repo: "acme/service", Number: 7181, Title: "fix: [DEMO-51] preserve cursor on refresh", Checks: "6 passed", Review: "CHANGES_REQUESTED", Comments: 8, UpdatedAt: ago(20), JiraKeys: []string{"DEMO-51"}, Issues: []Issue{{Key: "DEMO-51", Status: "Code Review"}}},
		{ID: "acme/infra#1087", Repo: "acme/infra", Number: 1087, Title: "chore: raise the preview slot limit from 5 to 15", Checks: "3 pending · 4 passed", Review: "REVIEW_REQUIRED", Comments: 1, UpdatedAt: ago(200)},
		{ID: "acme/service#7133", Repo: "acme/service", Number: 7133, Title: "docs: add deployment guide", Draft: true, Checks: "8 passed", UpdatedAt: ago(120)},
		{ID: "acme/web#3301", Repo: "acme/web", Number: 3301, Title: "feat: [DEMO-61] add status sparkline", Checks: "22 passed", Review: "APPROVED", MergeState: "CLEAN", Comments: 4, UpdatedAt: ago(30), JiraKeys: []string{"DEMO-61"}},
	}
	attachStacks(prs, nil)
	comments := map[string][]Comment{prs[1].ID: {
		{Author: "reviewer", Kind: "inline", Path: "tests/test_report.py", Line: 88, Body: "Can we also pin the **29-day** window?", CreatedAt: ago(3)},
		{Author: "contributor", Kind: "review", Body: "Done in `a1b2c3`.", CreatedAt: ago(2)},
	}}
	for _, size := range [][2]int{{200, 56}, {120, 40}, {80, 30}} {
		for _, filter := range []int{0, 4} {
			for mode, name := range []string{"overview", "description", "comments", "jira", "activity"} {
				m := newModel(Collector{Config: defaultConfig()}, WatchState{RecentEvents: []Event{{At: ago(1), PRID: prs[1].ID, Message: "1 new comment(s)"}}})
				m.width, m.height = size[0], size[1]
				m.snapshot = Snapshot{FetchedAt: now, PRs: prs}
				m.loading, m.now, m.nextSync = false, now, now.Add(90*time.Second)
				m.message = fmt.Sprintf("Synced %d open PRs", len(prs))
				m.filter, m.detailMode, m.commentsByPR = filter, mode, comments
				m.clamp()
				for j, i := range m.visible() {
					if m.snapshot.PRs[i].Number == 7234 {
						m.selected = j
					}
				}
				queue := map[int]string{0: "all", 4: "stacks"}[filter]
				file := filepath.Join(dir, strconv.Itoa(size[0])+"x"+strconv.Itoa(size[1])+"_"+queue+"_"+name+".ans")
				if err := os.WriteFile(file, []byte(m.View().Content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	t.Logf("wrote preview frames to %s", dir)
}
