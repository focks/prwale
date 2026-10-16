package main

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type fetchedMsg struct {
	snapshot Snapshot
	err      error
	force    bool
}
type tickMsg struct{ generation uint64 }
type pulseMsg struct{ at time.Time }
type openedMsg struct{ err error }
type commentsMsg struct {
	id       string
	comments []Comment
	err      error
}

type model struct {
	collector          Collector
	state              WatchState
	snapshot           Snapshot
	selected           int
	offset             int
	detailOffset       int
	query              string
	searching          bool
	numberQuery        string
	numberSearching    bool
	numberExact        bool
	filter             int // 0 all, 1 action, 2 changed, 3 missing Jira, 4 stacks
	events             bool
	detailMode         int // 0 overview, 1 PR description, 2 comments, 3 Jira description, 4 activity
	jiraIndex          int
	commentCursor      int
	commentsByPR       map[string][]Comment
	commentsLoadingFor string
	help               bool
	loading            bool
	showingCache       bool
	nextSync           time.Time
	syncGeneration     uint64
	now                time.Time
	cache              *renderCache
	message            string
	width              int
	height             int
}

func newModel(c Collector, state WatchState, initial ...Snapshot) model {
	m := model{collector: c, state: state, width: 100, height: 30, loading: true, commentsByPR: map[string][]Comment{}, now: time.Now(), cache: &renderCache{}}
	if len(initial) > 0 && !initial[0].FetchedAt.IsZero() {
		m.snapshot = initial[0]
		m.showingCache = true
		m.loading = time.Since(m.snapshot.FetchedAt) >= time.Duration(c.Config.PollSeconds)*time.Second
		if !m.loading {
			m.nextSync = m.snapshot.FetchedAt.Add(time.Duration(c.Config.PollSeconds) * time.Second)
		}
		m.message = "Showing saved data"
	}
	return m
}

func (m model) fetchCmd(force bool) tea.Cmd {
	return func() tea.Msg {
		var snapshot Snapshot
		var err error
		if force {
			snapshot, err = m.collector.CollectForced(context.Background())
		} else {
			snapshot, err = m.collector.Collect(context.Background())
		}
		return fetchedMsg{snapshot: snapshot, err: err, force: force}
	}
}

func (m model) commentsCmd(pr PR, force bool) tea.Cmd {
	return func() tea.Msg {
		comments, err := m.collector.LoadComments(context.Background(), pr, force)
		return commentsMsg{id: pr.ID, comments: comments, err: err}
	}
}

func (m model) tickCmd() tea.Cmd {
	delay := time.Until(m.nextSync)
	if delay < time.Millisecond {
		delay = time.Millisecond
	}
	generation := m.syncGeneration
	return tea.Tick(delay, func(time.Time) tea.Msg { return tickMsg{generation: generation} })
}

func pulseCmd() tea.Cmd {
	return tea.Tick(time.Second, func(at time.Time) tea.Msg { return pulseMsg{at: at} })
}

func (m model) Init() tea.Cmd {
	if m.showingCache && !m.loading {
		return tea.Batch(pulseCmd(), m.tickCmd())
	}
	return tea.Batch(pulseCmd(), m.fetchCmd(false))
}

func attentionReasons(pr PR) []string {
	var reasons []string
	if strings.Contains(pr.Checks, "failed") {
		reasons = append(reasons, "checks failing")
	}
	if pr.MergeState == "DIRTY" {
		reasons = append(reasons, "merge conflicts")
	}
	if pr.Review == "CHANGES_REQUESTED" {
		reasons = append(reasons, "review changes requested")
	}
	return reasons
}

func urgency(pr PR) int {
	if len(attentionReasons(pr)) > 0 {
		return 0
	}
	if len(pr.Changes) > 0 {
		return 1
	}
	if pr.Draft || pr.Review == "REVIEW_REQUIRED" || strings.Contains(pr.Checks, "pending") {
		return 2
	}
	return 3
}

func (m model) computeVisible() []int {
	var out []int
	query := strings.ToLower(strings.TrimSpace(m.query))
	for i, pr := range m.snapshot.PRs {
		if m.filter == 1 && len(attentionReasons(pr)) == 0 {
			continue
		}
		if m.filter == 2 && len(pr.Changes) == 0 {
			continue
		}
		if m.filter == 4 && pr.Stack == nil {
			continue
		}
		if m.filter == 3 && len(pr.JiraKeys) != 0 {
			continue
		}
		if query != "" {
			text := pr.Repo + " " + pr.Title + " " + pr.Branch + " " + pr.Body + " " + strconv.Itoa(pr.Number)
			for _, issue := range pr.Issues {
				text += " " + issue.Key + " " + issue.Summary + " " + issue.Status + " " + issue.Description
			}
			for _, comment := range m.commentsByPR[pr.ID] {
				text += " " + comment.Body + " " + comment.Author
			}
			if !strings.Contains(strings.ToLower(text), query) {
				continue
			}
		}
		if m.numberQuery != "" {
			number := strconv.Itoa(pr.Number)
			if m.numberExact && number != m.numberQuery {
				continue
			}
			if !m.numberExact && !strings.HasPrefix(number, m.numberQuery) {
				continue
			}
		}
		out = append(out, i)
	}
	rank := make(map[int]int, len(out))
	for _, i := range out {
		rank[i] = urgency(m.snapshot.PRs[i])
	}
	if m.filter == 4 {
		// Stacks view: stacks ordered by their most urgent member, each stack
		// listed top-down like the stack map on github.com.
		stackRank := map[string]int{}
		for _, i := range out {
			key := m.snapshot.PRs[i].Stack.Key
			if r, ok := stackRank[key]; !ok || rank[i] < r {
				stackRank[key] = rank[i]
			}
		}
		slices.SortStableFunc(out, func(a, b int) int {
			left, right := m.snapshot.PRs[a], m.snapshot.PRs[b]
			lk, rk := left.Stack.Key, right.Stack.Key
			if lk != rk {
				if stackRank[lk] != stackRank[rk] {
					return stackRank[lk] - stackRank[rk]
				}
				return strings.Compare(lk, rk)
			}
			return right.Stack.Position(right.Number) - left.Stack.Position(left.Number)
		})
		return out
	}
	slices.SortStableFunc(out, func(a, b int) int {
		left, right := m.snapshot.PRs[a], m.snapshot.PRs[b]
		if rank[a] != rank[b] {
			return rank[a] - rank[b]
		}
		if left.UpdatedAt.After(right.UpdatedAt) {
			return -1
		}
		if left.UpdatedAt.Before(right.UpdatedAt) {
			return 1
		}
		return strings.Compare(left.ID, right.ID)
	})
	return out
}

func (m *model) clamp() {
	n := len(m.visible())
	if n == 0 {
		m.selected, m.offset = 0, 0
		return
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= n {
		m.selected = n - 1
	}
	if m.offset > m.selected {
		m.offset = m.selected
	}
	capacity := m.listCapacity()
	if m.selected >= m.offset+capacity {
		m.offset = m.selected - capacity + 1
	}
	m.offset = min(m.offset, max(0, n-capacity))
}

func (m model) chosen() (PR, bool) {
	vis := m.visible()
	if len(vis) == 0 || m.selected >= len(vis) {
		return PR{}, false
	}
	return m.snapshot.PRs[vis[m.selected]], true
}

func (m model) jiraToOpen() string {
	pr, ok := m.chosen()
	if !ok {
		return ""
	}
	if len(pr.Issues) > 0 {
		index := 0
		if m.detailMode == 3 {
			index = min(m.jiraIndex, len(pr.Issues)-1)
		}
		return pr.Issues[index].Key
	}
	if len(pr.JiraKeys) > 0 {
		return pr.JiraKeys[0]
	}
	return ""
}

func (m *model) selectedCommentsCmd(previousID string) tea.Cmd {
	if m.detailMode == 2 {
		if pr, ok := m.chosen(); ok && pr.ID != previousID {
			m.commentCursor = 0
			m.detailOffset = 0
			m.commentsLoadingFor = pr.ID
			return m.commentsCmd(pr, false)
		}
	}
	return nil
}

func openCmd(name string, args ...string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
		if err != nil {
			err = fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
		}
		return openedMsg{err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clamp()
	case pulseMsg:
		m.now = msg.at
		return m, pulseCmd()
	case fetchedMsg:
		m.loading = false
		m.now = time.Now()
		m.nextSync = m.now.Add(time.Duration(m.collector.Config.PollSeconds) * time.Second)
		m.syncGeneration++
		next := m.tickCmd()
		if msg.err != nil {
			m.message = "Refresh failed: " + compactError(msg.err)
			return m, next
		}
		selectedID := ""
		previousComments, previousReviews := 0, 0
		if pr, ok := m.chosen(); ok {
			selectedID = pr.ID
			previousComments, previousReviews = pr.Comments, pr.ReviewCount
		}
		m.snapshot = msg.snapshot
		m.showingCache = false
		m.state = compareSnapshot(&m.snapshot, m.state)
		m.message = fmt.Sprintf("Synced %d open PRs", len(m.snapshot.PRs))
		if len(m.snapshot.Warnings) > 0 {
			m.message += fmt.Sprintf(" · %d warning(s)", len(m.snapshot.Warnings))
		}
		if err := saveState(m.state); err != nil {
			m.message = "Could not save state: " + compactError(err)
		} else if m.collector.Cache != nil {
			if err := m.collector.Cache.SaveSnapshot(m.snapshot); err != nil {
				m.message = "Could not save SQLite snapshot: " + compactError(err)
			}
		}
		for j, i := range m.visible() {
			if m.snapshot.PRs[i].ID == selectedID {
				m.selected = j
				break
			}
		}
		m.detailOffset = 0
		m.clamp()
		if m.detailMode == 2 {
			if pr, ok := m.chosen(); ok {
				m.commentsLoadingFor = pr.ID
				return m, tea.Batch(next, m.commentsCmd(pr, msg.force || pr.Comments != previousComments || pr.ReviewCount != previousReviews))
			}
		}
		return m, next
	case tickMsg:
		if msg.generation != m.syncGeneration || m.loading {
			return m, nil
		}
		m.loading = true
		m.nextSync = time.Time{}
		m.syncGeneration++
		return m, m.fetchCmd(false)
	case openedMsg:
		if msg.err != nil {
			m.message = "Open failed: " + compactError(msg.err)
		} else {
			m.message = "Opened in browser"
		}
	case commentsMsg:
		if msg.id == m.commentsLoadingFor {
			m.commentsLoadingFor = ""
		}
		if msg.comments != nil {
			m.commentsByPR[msg.id] = msg.comments
		} else if msg.err == nil {
			m.commentsByPR[msg.id] = []Comment{}
		}
		if msg.err != nil {
			m.message = compactError(msg.err)
		} else {
			m.message = fmt.Sprintf("Loaded %d comments and reviews", len(msg.comments))
		}
	case tea.MouseWheelMsg:
		delta := 0
		if msg.Button == tea.MouseWheelDown {
			delta = 3
		} else if msg.Button == tea.MouseWheelUp {
			delta = -3
		}
		if delta == 0 {
			return m, nil
		}
		if m.mouseInList(msg.X, msg.Y) {
			previous := ""
			if pr, ok := m.chosen(); ok {
				previous = pr.ID
			}
			m.selected += delta
			m.clamp()
			m.detailOffset = 0
			return m, m.selectedCommentsCmd(previous)
		}
		m.scrollDetail(delta)
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft && m.mouseInList(msg.X, msg.Y) {
			previous := ""
			if pr, ok := m.chosen(); ok {
				previous = pr.ID
			}
			if index := m.listIndexAt(msg.Y); index >= 0 {
				m.selected = index
				m.clamp()
				m.detailOffset = 0
				return m, m.selectedCommentsCmd(previous)
			}
		}
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.searching {
			beforeID := ""
			if pr, ok := m.chosen(); ok {
				beforeID = pr.ID
			}
			switch key {
			case "enter":
				m.searching = false
			case "esc":
				m.searching = false
				m.query = ""
				m.selected = 0
			case "backspace", "ctrl+h":
				runes := []rune(m.query)
				if len(runes) > 0 {
					m.query = string(runes[:len(runes)-1])
					m.selected = 0
				}
			case "ctrl+u":
				m.query = ""
				m.selected = 0
			default:
				if len([]rune(key)) == 1 && key != "\n" {
					m.query += key
					m.selected = 0
				}
			}
			m.clamp()
			return m, m.selectedCommentsCmd(beforeID)
		}
		if m.numberSearching {
			beforeID := ""
			if pr, ok := m.chosen(); ok {
				beforeID = pr.ID
			}
			switch key {
			case "enter":
				m.numberSearching = false
				m.numberExact = true
			case "esc":
				m.numberSearching = false
				m.numberQuery = ""
				m.numberExact = false
				m.selected = 0
			case "backspace", "ctrl+h":
				if len(m.numberQuery) > 0 {
					m.numberQuery = m.numberQuery[:len(m.numberQuery)-1]
					m.selected = 0
				}
			case "ctrl+u":
				m.numberQuery = ""
				m.selected = 0
			default:
				if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
					m.numberQuery += key
					m.numberExact = false
					m.selected = 0
				}
			}
			m.clamp()
			return m, m.selectedCommentsCmd(beforeID)
		}
		if m.help {
			m.help = false
			return m, nil
		}
		beforeID := ""
		if before, ok := m.chosen(); ok {
			beforeID = before.ID
		}
		switch key {
		case "q":
			return m, tea.Quit
		case "/":
			m.searching = true
			m.numberSearching = false
			m.numberQuery = ""
			m.numberExact = false
			m.filter = 0
			m.selected = 0
		case "#":
			m.numberSearching = true
			m.numberExact = false
			m.query = ""
			m.filter = 0
			m.selected = 0
		case "esc":
			m.query = ""
			m.numberQuery = ""
			m.numberExact = false
			m.filter = 0
			m.selected = 0
			m.clamp()
		case "j", "down":
			m.selected++
			m.detailOffset = 0
			m.clamp()
		case "k", "up":
			m.selected--
			m.detailOffset = 0
			m.clamp()
		case "g":
			m.selected = 0
			m.detailOffset = 0
			m.clamp()
		case "G":
			m.selected = len(m.visible()) - 1
			m.detailOffset = 0
			m.clamp()
		case "ctrl+d", "pgdown":
			m.selected += max(1, m.height/2)
			m.detailOffset = 0
			m.clamp()
		case "ctrl+u", "pgup":
			m.selected -= max(1, m.height/2)
			m.detailOffset = 0
			m.clamp()
		case "[":
			m.detailOffset = max(0, m.detailOffset-1)
		case "]":
			w, h := m.detailGeometry()
			m.detailOffset = min(m.detailOffset+1, max(0, len(m.detailContent(w))-h))
		case "ctrl+b":
			m.detailOffset = max(0, m.detailOffset-max(1, m.height/2))
		case "ctrl+f":
			w, h := m.detailGeometry()
			m.detailOffset = min(m.detailOffset+max(1, h/2), max(0, len(m.detailContent(w))-h))
		case "tab":
			m.filter = (m.filter + 1) % 5
			m.selected = 0
			m.offset = 0
			m.detailOffset = 0
			m.clamp()
		case "1", "2", "3", "4", "5":
			m.filter = int(key[0] - '1')
			m.selected = 0
			m.detailOffset = 0
			m.clamp()
		case "s":
			m.detailMode = 0
			m.detailOffset = 0
		case "d":
			m.detailMode = 1
			m.detailOffset = 0
		case "c":
			m.detailMode = 2
			m.detailOffset = 0
			m.commentCursor = 0
			if pr, ok := m.chosen(); ok {
				m.commentsLoadingFor = pr.ID
				return m, m.commentsCmd(pr, false)
			}
		case "i":
			m.detailMode = 3
			m.detailOffset = 0
			m.jiraIndex = 0
		case "I":
			if pr, ok := m.chosen(); ok && len(pr.Issues) > 0 {
				m.jiraIndex = (m.jiraIndex + 1) % len(pr.Issues)
				m.detailOffset = 0
				m.detailMode = 3
			}
		case "e":
			m.detailMode = 4
			m.detailOffset = 0
		case "n":
			if m.detailMode == 2 {
				if pr, ok := m.chosen(); ok {
					m.commentCursor = min(len(m.commentsByPR[pr.ID])-1, m.commentCursor+1)
					m.commentCursor = max(0, m.commentCursor)
					m.detailOffset = 0
				}
			}
		case "p":
			if m.detailMode == 2 {
				m.commentCursor = max(0, m.commentCursor-1)
				m.detailOffset = 0
			}
		case "?":
			m.help = true
		case "r":
			if !m.loading {
				m.loading = true
				m.nextSync = time.Time{}
				m.syncGeneration++
				return m, m.fetchCmd(true)
			}
		case "o", "enter":
			if pr, ok := m.chosen(); ok {
				return m, openCmd("gh", "pr", "view", strconv.Itoa(pr.Number), "-R", pr.Repo, "--web")
			}
		case "{", "}":
			if pr, ok := m.chosen(); ok && pr.Stack != nil {
				pos := pr.Stack.Position(pr.Number)
				target := pos - 1 // "{" moves down the stack, toward trunk
				if key == "}" {
					target = pos + 1
				}
				if target >= 1 && target <= len(pr.Stack.Entries) {
					if m.selectPR(fmt.Sprintf("%s#%d", pr.Repo, pr.Stack.Entries[target-1].Number)) {
						m.detailOffset = 0
					} else {
						m.message = fmt.Sprintf("#%d is not one of your open PRs", pr.Stack.Entries[target-1].Number)
					}
				}
			}
		case "J", "O":
			if key := m.jiraToOpen(); key != "" {
				return m, openCmd("jira", "open", key)
			}
		}
		return m, m.selectedCommentsCmd(beforeID)
	}
	return m, nil
}

// selectPR moves the selection to the PR with id, widening the queue to
// "all" when the current filter or search hides it.
func (m *model) selectPR(id string) bool {
	find := func() bool {
		for j, i := range m.visible() {
			if m.snapshot.PRs[i].ID == id {
				m.selected = j
				m.clamp()
				return true
			}
		}
		return false
	}
	if find() {
		return true
	}
	for _, pr := range m.snapshot.PRs {
		if pr.ID == id {
			m.filter, m.query, m.numberQuery, m.numberExact = 0, "", "", false
			return find()
		}
	}
	return false
}
