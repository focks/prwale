package main

import (
	"fmt"
	"image/color"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Palette. Every color is used for one meaning only:
// red = blocked on you, amber = waiting on others, green = clear,
// teal = changed since last sync, violet = Jira, accent = focus and navigation.
var (
	bright     = lipgloss.Color("#F2F4F8") // titles, selected text
	ink        = lipgloss.Color("#D3D9E3") // body text
	muted      = lipgloss.Color("#8E98AA") // secondary text
	subtle     = lipgloss.Color("#5F697B") // hints, labels, timestamps
	border     = lipgloss.Color("#384254") // rules and dividers
	selectedBG = lipgloss.Color("#243044") // selected row
	accent     = lipgloss.Color("#7AB8FF")
	green      = lipgloss.Color("#7FD6A0")
	amber      = lipgloss.Color("#F2C46D")
	red        = lipgloss.Color("#FF7A8A")
	violet     = lipgloss.Color("#B5A4FF")
	teal       = lipgloss.Color("#5FD4C8")
	onFill     = lipgloss.Color("#0F131A") // text on filled badges
)

const (
	wideBreakpoint = 88
	chromeTop      = 2 // header + rule
	chromeBottom   = 2 // rule + footer
)

// ── spans: styled text that keeps the row background intact ──────────────

type span struct {
	text  string
	style lipgloss.Style
}

func fg(text string, c color.Color) span {
	return span{text, lipgloss.NewStyle().Foreground(c)}
}
func bold(text string, c color.Color) span {
	return span{text, lipgloss.NewStyle().Foreground(c).Bold(true)}
}
func badge(text string, bg color.Color) span {
	return span{" " + text + " ", lipgloss.NewStyle().Foreground(onFill).Background(bg).Bold(true)}
}
func gap(n int) span { return span{strings.Repeat(" ", max(0, n)), lipgloss.NewStyle()} }

func spansWidth(spans []span) int {
	w := 0
	for _, s := range spans {
		w += lipgloss.Width(s.text)
	}
	return w
}

// renderSpans renders spans into at most width cells, truncating the last
// visible span with an ellipsis. bg (when non-nil) is applied to every span.
func renderSpans(spans []span, width int, bg color.Color) (string, int) {
	var b strings.Builder
	used := 0
	for _, s := range spans {
		if used >= width {
			break
		}
		text := s.text
		w := lipgloss.Width(text)
		if used+w > width {
			text = ansi.Truncate(text, width-used, "…")
			w = lipgloss.Width(text)
		}
		style := s.style
		if bg != nil {
			style = style.Background(bg)
		}
		b.WriteString(style.Render(text))
		used += w
	}
	return b.String(), used
}

// row lays out left spans and right spans on one line of exactly width cells.
func row(width int, bg color.Color, left, right []span) string {
	if width <= 0 {
		return ""
	}
	rw := spansWidth(right)
	if rw > width/2 {
		rw = width / 2
	}
	avail := width - rw
	if rw > 0 {
		avail--
	}
	l, lw := renderSpans(left, max(0, avail), bg)
	r, rwUsed := renderSpans(right, rw, bg)
	filler := lipgloss.NewStyle()
	if bg != nil {
		filler = filler.Background(bg)
	}
	return l + filler.Render(strings.Repeat(" ", max(0, width-lw-rwUsed))) + r
}

func rule(width int) string {
	return lipgloss.NewStyle().Foreground(border).Render(strings.Repeat("─", max(0, width)))
}

// section renders "TITLE ─────────" in the detail pane.
func section(title string, width int) string {
	head := lipgloss.NewStyle().Foreground(muted).Bold(true).Render(title)
	return head + " " + rule(width-lipgloss.Width(title)-1)
}

// ── legacy helpers kept for other callers ────────────────────────────────

func label(text string, c color.Color) string {
	return lipgloss.NewStyle().Foreground(c).Bold(true).Render(text)
}
func dim(text string) string { return lipgloss.NewStyle().Foreground(subtle).Render(text) }
func clip(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	return ansi.Truncate(s, max(1, width), "…")
}
func pad(s string, width int) string {
	w := lipgloss.Width(s)
	switch {
	case w == width:
		return s
	case w < width:
		return s + strings.Repeat(" ", width-w)
	case width <= 0:
		return ""
	}
	return ansi.Truncate(s, width, "…")
}

// ── PR semantics ─────────────────────────────────────────────────────────

func shortRepo(repo string) string {
	parts := strings.Split(repo, "/")
	return parts[len(parts)-1]
}

type checkPart struct {
	count int
	kind  string // failed, passed, pending
}

func parseChecks(checks string) []checkPart {
	var parts []checkPart
	for _, piece := range strings.Split(checks, " · ") {
		fields := strings.Fields(piece)
		if len(fields) != 2 {
			continue
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil || n == 0 {
			continue
		}
		parts = append(parts, checkPart{n, fields[1]})
	}
	return parts
}

func checkCount(pr PR, kind string) int {
	for _, p := range parseChecks(pr.Checks) {
		if p.kind == kind {
			return p.count
		}
	}
	return 0
}

// prSignal is the single glyph, color and short reason that best describes a PR.
func prSignal(pr PR) (string, color.Color, string) {
	switch {
	case pr.MergeState == "DIRTY":
		return "⊘", red, "merge conflict"
	case checkCount(pr, "failed") > 0:
		return "✗", red, fmt.Sprintf("%d failing", checkCount(pr, "failed"))
	case pr.Review == "CHANGES_REQUESTED":
		return "↺", red, "changes requested"
	case pr.DetailError != "":
		return "?", muted, "detail unavailable"
	case pr.Draft:
		return "◇", muted, "draft"
	case strings.Contains(pr.Checks, "pending"):
		return "◐", amber, fmt.Sprintf("%d running", checkCount(pr, "pending"))
	case pr.Review == "REVIEW_REQUIRED":
		return "○", amber, "awaiting review"
	case pr.Review == "APPROVED":
		return "✓", green, "approved"
	case strings.Contains(pr.Checks, "passed"):
		return "✓", green, "checks pass"
	}
	return "·", muted, "no CI"
}

// checkGlyph is kept for callers that only need the CI glyph.
func checkGlyph(pr PR) string { g, _, _ := prSignal(pr); return g }

type group struct {
	name   string
	color  color.Color
	key    string
	detail []span
}

// groupOf picks the list section a PR is shown under: its urgency, or in the
// Stacks queue, the stack it belongs to.
func (m model) groupOf(pr PR) group {
	if m.filter == 4 && pr.Stack != nil {
		s := pr.Stack
		detail := []span{fg(shortRepo(pr.Repo), muted)}
		if s.Native {
			detail = append(detail, fg(fmt.Sprintf("  #%d", s.Number), subtle))
		}
		detail = append(detail, fg("  → "+s.Base, subtle))
		return group{name: "STACK", color: bright, key: s.Key, detail: detail}
	}
	g := groups[urgency(pr)]
	g.key = g.name
	return g
}

var groups = []group{
	{name: "NEEDS ATTENTION", color: red},
	{name: "CHANGED", color: teal},
	{name: "WAITING", color: amber},
	{name: "QUIET", color: green},
}

func jiraStatusColor(status string) color.Color {
	s := strings.ToLower(status)
	switch {
	case strings.Contains(s, "done"), strings.Contains(s, "closed"), strings.Contains(s, "resolved"):
		return green
	case strings.Contains(s, "review"):
		return accent
	case strings.Contains(s, "progress"):
		return amber
	}
	return muted
}

var conventionalPrefix = regexp.MustCompile(`^[a-zA-Z]+(\([^)]*\))?!?:\s*`)

// titleSpans dims the conventional-commit prefix and drops the linked Jira
// key that is already shown on the second line.
func titleSpans(pr PR, c color.Color, weight bool) []span {
	title := pr.Title
	for _, key := range pr.JiraKeys {
		title = strings.ReplaceAll(title, "["+key+"]", "")
	}
	title = strings.Join(strings.Fields(title), " ")
	prefix := conventionalPrefix.FindString(title)
	rest := strings.TrimPrefix(title, prefix)
	main := fg(rest, c)
	if weight {
		main = bold(rest, c)
	}
	if prefix == "" {
		return []span{main}
	}
	return []span{fg(strings.TrimSpace(prefix)+" ", subtle), main}
}

func ageLabel(updated, now time.Time) string {
	if updated.IsZero() {
		return "—"
	}
	delta := now.Sub(updated)
	switch {
	case delta < time.Minute:
		return "now"
	case delta < time.Hour:
		return fmt.Sprintf("%dm", int(delta.Minutes()))
	case delta < 24*time.Hour:
		return fmt.Sprintf("%dh", int(delta.Hours()))
	case delta < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(delta.Hours()/24))
	}
	return updated.Local().Format("Jan 02")
}

func countdown(until, now time.Time) string {
	if until.IsZero() {
		return ""
	}
	seconds := max(0, int(until.Sub(now).Seconds()+0.999))
	if seconds >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", seconds/3600, (seconds%3600)/60, seconds%60)
	}
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

func overview(pr PR) (string, string, color.Color) {
	if reasons := attentionReasons(pr); len(reasons) > 0 {
		return "NEEDS ATTENTION", strings.Join(reasons, " · "), red
	}
	var waiting []string
	if pr.Draft {
		waiting = append(waiting, "draft")
	}
	if strings.Contains(pr.Checks, "pending") {
		waiting = append(waiting, "checks running")
	}
	if pr.Review == "REVIEW_REQUIRED" {
		waiting = append(waiting, "review required")
	}
	if len(waiting) > 0 {
		return "WAITING", strings.Join(waiting, " · "), amber
	}
	return "CLEAR", "nothing blocking in the fetched fields", green
}

// ── geometry ─────────────────────────────────────────────────────────────

func (m model) wide() bool { return m.width >= wideBreakpoint }

func (m model) bodyHeight() int { return max(1, m.height-chromeTop-chromeBottom) }

func (m model) leftWidth() int {
	return min(84, max(44, m.width*44/100))
}

func (m model) listHeight() int {
	if m.wide() {
		return m.bodyHeight()
	}
	return max(4, m.bodyHeight()*45/100)
}

// detailGeometry returns the content width and height of the detail pane,
// excluding its tab strip and side margins.
func (m model) detailGeometry() (int, int) {
	if m.wide() {
		return max(1, m.width-m.leftWidth()-1-4), max(1, m.bodyHeight()-3)
	}
	return max(1, m.width-4), max(1, m.bodyHeight()-m.listHeight()-1-3)
}

func (m model) listCapacity() int {
	if m.wide() {
		return max(1, (m.listHeight()-6)/2)
	}
	return max(1, m.listHeight()-4)
}

func (m model) mouseInList(x, y int) bool {
	if y < chromeTop || y >= chromeTop+m.listHeight() {
		return false
	}
	if m.wide() {
		return x < m.leftWidth()
	}
	return true
}

func (m model) listIndexAt(y int) int {
	width := m.width
	if m.wide() {
		width = m.leftWidth()
	}
	_, rows := m.listView(width, m.listHeight())
	line := y - chromeTop
	if line < 0 || line >= len(rows) {
		return -1
	}
	return rows[line]
}

func (m *model) scrollDetail(delta int) {
	w, h := m.detailGeometry()
	m.detailOffset = max(0, min(m.detailOffset+delta, max(0, len(m.detailContent(w))-h)))
}

// ── list pane ────────────────────────────────────────────────────────────

func (m model) listLines(width, height int) []string {
	lines, _ := m.listView(width, height)
	return lines
}

// listView returns the rendered list and, for each line, the visible index
// of the PR it belongs to (-1 for headers and spacing).
func (m model) buildListView(width, height int) ([]string, []int) {
	vis := m.visible()
	empty := func(text string) ([]string, []int) {
		return []string{"", row(width, nil, []span{gap(2), fg(text, subtle)}, nil)}, []int{-1, -1}
	}
	if len(vis) == 0 {
		switch {
		case m.loading && m.snapshot.FetchedAt.IsZero():
			return empty("Loading your open PRs…")
		case m.query != "":
			return empty("Nothing matches /" + m.query)
		case m.numberQuery != "":
			return empty("No loaded PR matches #" + m.numberQuery)
		case m.filter != 0:
			return empty("Nothing in this queue")
		}
		return empty("No open PRs")
	}
	groupCount := map[string]int{}
	for _, i := range vis {
		groupCount[m.groupOf(m.snapshot.PRs[i]).key]++
	}
	start := min(max(0, m.offset), len(vis)-1)
	start = min(start, m.selected)
	for {
		lines, rows, last := m.layoutList(vis, groupCount, start, width, height)
		if last >= m.selected || start >= m.selected {
			return lines, rows
		}
		start++
	}
}

func (m model) layoutList(vis []int, groupCount map[string]int, start, width, height int) ([]string, []int, int) {
	var lines []string
	var rows []int
	itemH := 1
	if m.wide() {
		itemH = 2
	}
	room := height - 1 // last line is the position indicator
	prevGroup := ""
	last := start - 1
	for i := start; i < len(vis); i++ {
		pr := m.snapshot.PRs[vis[i]]
		g := m.groupOf(pr)
		need := itemH
		header := g.key != prevGroup
		spacer := header && len(lines) > 0
		if header {
			need++
		}
		if spacer && len(lines)+need+1 > room {
			spacer = false // tight: drop the blank line before the group
		}
		if spacer {
			need++
		}
		if len(lines)+need > room {
			break
		}
		if header {
			if spacer {
				lines, rows = append(lines, ""), append(rows, -1)
			}
			lines = append(lines, row(width, nil, append([]span{gap(2), bold(g.name, g.color), gap(1)}, g.detail...), []span{fg(strconv.Itoa(groupCount[g.key]), subtle), gap(1)}))
			rows = append(rows, -1)
			prevGroup = g.key
		}
		for _, line := range m.listItem(pr, i == m.selected, width) {
			lines, rows = append(lines, line), append(rows, i)
		}
		last = i
	}
	for len(lines) < room {
		lines, rows = append(lines, ""), append(rows, -1)
	}
	if len(vis) > 0 {
		pos := fmt.Sprintf("%d–%d of %d", start+1, last+1, len(vis))
		if start == 0 && last == len(vis)-1 {
			pos = fmt.Sprintf("%d PRs", len(vis))
		}
		lines, rows = append(lines, row(width, nil, []span{gap(2), fg(pos, subtle)}, nil)), append(rows, -1)
	}
	return lines, rows, last
}

func (m model) listItem(pr PR, selected bool, width int) []string {
	glyph, stateColor, reason := prSignal(pr)
	var bg color.Color
	bar := gap(1)
	titleColor := ink
	if selected {
		bg = selectedBG
		bar = fg("▍", accent)
		titleColor = bright
	}
	right := []span{fg(ageLabel(pr.UpdatedAt, m.now), subtle)}
	if len(pr.Changes) > 0 {
		right = append([]span{fg("● ", teal)}, right...)
	}
	if !m.wide() {
		left := append([]span{bar, gap(1), bold(glyph, stateColor), gap(1), fg(fmt.Sprintf("#%d", pr.Number), subtle), gap(1)}, titleSpans(pr, titleColor, selected)...)
		if selected {
			left, right = liftSubtle(left), liftSubtle(right)
		}
		return []string{row(width, bg, left, right)}
	}
	first := append([]span{bar, gap(1), bold(glyph, stateColor), gap(2)}, titleSpans(pr, titleColor, selected)...)
	reasonColor := muted
	if stateColor == red {
		reasonColor = red
	}
	sep := fg("  ", subtle)
	second := []span{bar, gap(4), fg(fmt.Sprintf("#%d", pr.Number), subtle), sep, fg(reason, reasonColor), sep}
	if len(pr.JiraKeys) == 0 {
		second = append(second, fg("no jira", subtle))
	} else {
		second = append(second, fg(pr.JiraKeys[0], violet))
		if len(pr.Issues) > 0 && pr.Issues[0].Status != "" {
			second = append(second, gap(1), fg(pr.Issues[0].Status, jiraStatusColor(pr.Issues[0].Status)))
		}
		if len(pr.JiraKeys) > 1 {
			second = append(second, fg(fmt.Sprintf(" +%d", len(pr.JiraKeys)-1), subtle))
		}
	}
	if pr.Stack != nil {
		tagColor := muted
		if _, blocked := stackBlocker(pr, m.snapshot.PRs); blocked {
			tagColor = red
		}
		second = append(second, sep, fg(fmt.Sprintf("stack %d/%d", pr.Stack.Position(pr.Number), len(pr.Stack.Entries)), tagColor))
	}
	second = append(second, sep, fg(shortRepo(pr.Repo), muted))
	var meta []span
	if pr.Comments > 0 && spansWidth(second)+8 <= width {
		meta = append(meta, fg(fmt.Sprintf("%d cmt", pr.Comments), subtle))
	}
	if selected {
		first, right, second, meta = liftSubtle(first), liftSubtle(right), liftSubtle(second), liftSubtle(meta)
	}
	return []string{row(width, bg, first, right), row(width, bg, second, meta)}
}

// ── detail pane ──────────────────────────────────────────────────────────

var detailTabs = []struct{ key, name string }{
	{"s", "Overview"}, {"d", "Description"}, {"c", "Comments"}, {"i", "Jira"}, {"e", "Activity"},
}

func (m model) tabStrip(width int) []string {
	var spans []span
	var under strings.Builder
	under.WriteString(" ")
	used := 1
	spans = append(spans, gap(1))
	for i, tab := range detailTabs {
		name := tab.name
		if pr, ok := m.chosen(); ok {
			if i == 2 {
				if comments, loaded := m.commentsByPR[pr.ID]; loaded && len(comments) > 0 {
					name += " " + strconv.Itoa(len(comments))
				}
			}
			if i == 3 && len(pr.Issues) > 1 {
				name += " " + strconv.Itoa(len(pr.Issues))
			}
		}
		active := !m.help && m.detailMode == i
		label := fg(name, muted)
		if active {
			label = bold(name, bright)
		}
		cell := []span{fg(tab.key, subtle), gap(1), label, gap(3)}
		cellW := spansWidth(cell)
		if used+cellW > width {
			break
		}
		spans = append(spans, cell...)
		mark := lipgloss.NewStyle().Foreground(border).Render(strings.Repeat("─", cellW))
		if active {
			mark = lipgloss.NewStyle().Foreground(accent).Render(strings.Repeat("━", cellW-3)) + lipgloss.NewStyle().Foreground(border).Render("───")
		}
		under.WriteString(mark)
		used += cellW
	}
	var right []span
	if m.help {
		right = []span{bold("keys", accent), gap(1)}
	}
	underline := lipgloss.NewStyle().Foreground(border).Render("─") + under.String()[1:]
	underline += rule(width - lipgloss.Width(underline))
	return []string{row(width, nil, spans, right), clip(underline, width)}
}

func (m model) detailRows(width, height int) []string {
	lines := m.detailContent(width)
	start := min(m.detailOffset, max(0, len(lines)-height))
	start = max(0, start)
	end := min(len(lines), start+height)
	return lines[start:end]
}

func field(width int, name string, value []span) string {
	return row(width, nil, append([]span{fg(fmt.Sprintf("%-10s", name), subtle)}, value...), nil)
}

func wrapStyled(text string, width int, c color.Color, weight bool) []string {
	var out []string
	for _, line := range wrappedLines(text, width) {
		if weight {
			out = append(out, lipgloss.NewStyle().Foreground(c).Bold(true).Render(line))
		} else {
			out = append(out, lipgloss.NewStyle().Foreground(c).Render(line))
		}
	}
	return out
}

func wrappedLines(body string, width int) []string {
	if strings.TrimSpace(body) == "" {
		return []string{dim("No description provided.")}
	}
	var out []string
	for _, raw := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if raw == "" {
			out = append(out, "")
			continue
		}
		for _, line := range strings.Split(ansi.Wordwrap(raw, max(1, width), ""), "\n") {
			out = append(out, clip(line, width))
		}
	}
	return out
}

func bodyPreview(body string, width, limit int) []string {
	var lines []string
	for _, line := range markdownLines(body, width) {
		if strings.TrimSpace(ansi.Strip(line)) == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) >= limit {
			break
		}
	}
	return lines
}

func checksSpans(pr PR) []span {
	parts := parseChecks(pr.Checks)
	if len(parts) == 0 {
		return []span{fg("no checks reported", subtle)}
	}
	var out []span
	for i, p := range parts {
		if i > 0 {
			out = append(out, gap(3))
		}
		switch p.kind {
		case "failed":
			out = append(out, bold("✗", red), fg(fmt.Sprintf(" %d failed", p.count), red))
		case "passed":
			out = append(out, bold("✓", green), fg(fmt.Sprintf(" %d passed", p.count), ink))
		case "pending":
			out = append(out, bold("◐", amber), fg(fmt.Sprintf(" %d running", p.count), amber))
		default:
			out = append(out, fg(fmt.Sprintf("%d %s", p.count, p.kind), muted))
		}
	}
	return out
}

func reviewSpans(pr PR) []span {
	switch pr.Review {
	case "APPROVED":
		return []span{bold("✓", green), fg(" approved", green)}
	case "CHANGES_REQUESTED":
		return []span{bold("↺", red), fg(" changes requested", red)}
	case "REVIEW_REQUIRED":
		return []span{bold("○", amber), fg(" review required", amber)}
	case "":
		return []span{fg("—", subtle)}
	}
	return []span{fg(strings.ToLower(strings.ReplaceAll(pr.Review, "_", " ")), muted)}
}

func mergeSpans(pr PR) []span {
	switch pr.MergeState {
	case "DIRTY":
		return []span{bold("⊘", red), fg(" conflicts with base", red)}
	case "CLEAN":
		return []span{bold("✓", green), fg(" ready to merge", green)}
	case "BLOCKED":
		return []span{fg("blocked", muted), fg("  by required checks or review", subtle)}
	case "BEHIND":
		return []span{fg("behind base", amber)}
	case "UNSTABLE":
		return []span{fg("unstable", amber)}
	}
	return []span{fg(strings.ToLower(pr.MergeState), muted)}
}

func (m model) issueBlock(issue Issue, width int) []string {
	head := []span{bold(issue.Key, violet)}
	if issue.Status != "" {
		head = append(head, gap(2), fg("● ", jiraStatusColor(issue.Status)), fg(issue.Status, ink))
	}
	var right []span
	if issue.Priority != "" {
		right = append(right, fg(issue.Priority, subtle))
	}
	lines := []string{row(width, nil, head, right)}
	if issue.Summary != "" {
		lines = append(lines, wrapStyled(issue.Summary, width, ink, false)...)
	}
	if issue.Assignee != "" {
		lines = append(lines, row(width, nil, []span{fg("assignee  ", subtle), fg(issue.Assignee, muted)}, nil))
	}
	if issue.Error != "" {
		lines = append(lines, row(width, nil, []span{fg("! cached · refresh failed: "+issue.Error, red)}, nil))
	}
	return lines
}

func (m model) buildDetailContent(width int) []string {
	if m.help {
		return m.helpLines(width)
	}
	if m.detailMode == 4 {
		lines := []string{section("ACROSS YOUR PRS", width), ""}
		if len(m.state.RecentEvents) == 0 {
			return append(lines, dim("No changes recorded yet. Changes appear after the second sync."))
		}
		for _, ev := range m.state.RecentEvents {
			lines = append(lines,
				row(width, nil, []span{fg(ev.PRID, accent)}, []span{fg(ev.At.Local().Format("Jan 02 15:04"), subtle)}),
				row(width, nil, []span{gap(2), fg(ev.Message, ink)}, nil))
		}
		return lines
	}
	pr, ok := m.chosen()
	if !ok {
		return []string{"", dim("Select a PR to see its checks, review and Jira context.")}
	}
	header := []string{
		row(width, nil, []span{fg(shortRepo(pr.Repo), muted), fg("  #"+strconv.Itoa(pr.Number), subtle)}, []span{fg("updated "+ageLabel(pr.UpdatedAt, m.now), subtle)}),
	}
	header = append(header, wrapStyled(pr.Title, width, bright, true)...)
	switch m.detailMode {
	case 1:
		lines := append(header, "")
		return append(lines, markdownLines(pr.Body, width)...)
	case 2:
		return append(header, m.commentLines(pr, width)...)
	case 3:
		lines := append(header, "")
		if len(pr.Issues) == 0 {
			return append(lines, dim("No Jira issue linked. Add a key to the branch, title or body."))
		}
		idx := min(m.jiraIndex, len(pr.Issues)-1)
		issue := pr.Issues[idx]
		title := "JIRA"
		if len(pr.Issues) > 1 {
			title = fmt.Sprintf("JIRA %d/%d · I next", idx+1, len(pr.Issues))
		}
		lines = append(lines, section(title, width))
		lines = append(lines, m.issueBlock(issue, width)...)
		lines = append(lines, "")
		return append(lines, markdownLines(issue.Description, width)...)
	}
	// overview
	name, reasons, c := overview(pr)
	lines := append(header, "", row(width, nil, []span{badge(name, c), gap(2), fg(reasons, c)}, nil), "")
	lines = append(lines,
		field(width, "Checks", checksSpans(pr)),
		field(width, "Review", reviewSpans(pr)))
	if pr.MergeState != "" {
		lines = append(lines, field(width, "Merge", mergeSpans(pr)))
	}
	if pr.Draft {
		lines = append(lines, field(width, "Draft", []span{fg("◇ draft", muted)}))
	}
	lines = append(lines, field(width, "Talk", []span{fg(fmt.Sprintf("%d comments", pr.Comments), ink), fg(fmt.Sprintf("  ·  %d reviews", pr.ReviewCount), muted)}))
	if pr.Branch != "" {
		lines = append(lines, field(width, "Branch", []span{fg(pr.Branch, muted)}))
	}
	if len(pr.Changes) > 0 {
		lines = append(lines, "", section("NEW SINCE LAST SYNC", width))
		for _, ch := range pr.Changes {
			lines = append(lines, row(width, nil, []span{fg("● ", teal), fg(ch, ink)}, nil))
		}
	}
	if pr.Stack != nil {
		lines = append(lines, "")
		lines = append(lines, m.stackMap(pr, width)...)
	}
	lines = append(lines, "", section("JIRA", width))
	if len(pr.JiraKeys) == 0 {
		lines = append(lines, dim("No Jira key in branch, title or body."))
	}
	for i, issue := range pr.Issues {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.issueBlock(issue, width)...)
	}
	if len(pr.Issues) == 0 && len(pr.JiraKeys) > 0 {
		lines = append(lines, row(width, nil, []span{fg(strings.Join(pr.JiraKeys, "  "), violet), fg("  not loaded yet", subtle)}, nil))
	}
	var activity []string
	for _, ev := range m.state.RecentEvents {
		if ev.PRID != pr.ID {
			continue
		}
		activity = append(activity, row(width, nil, []span{fg(ev.At.Local().Format("Jan 02 15:04")+"  ", subtle), fg(ev.Message, ink)}, nil))
		if len(activity) >= 5 {
			break
		}
	}
	if len(activity) > 0 {
		lines = append(lines, "", section("ACTIVITY", width))
		lines = append(lines, activity...)
	}
	lines = append(lines, "", section("DESCRIPTION", width))
	if preview := bodyPreview(pr.Body, width, 8); len(preview) == 0 {
		lines = append(lines, dim("No description provided."))
	} else {
		lines = append(lines, preview...)
	}
	lines = append(lines, "", row(width, nil, []span{fg("d", ink), fg(" full description   ", subtle), fg("c", ink), fg(" comments   ", subtle), fg("i", ink), fg(" jira", subtle)}, nil))
	if pr.DetailError != "" {
		lines = append(lines, "", row(width, nil, []span{fg("! GitHub detail stale: "+pr.DetailError, red)}, nil))
	}
	return lines
}

func (m model) commentLines(pr PR, width int) []string {
	lines := []string{""}
	comments, loaded := m.commentsByPR[pr.ID]
	switch {
	case m.commentsLoadingFor == pr.ID:
		return append(lines, dim("◐ Loading conversation, reviews and inline comments…"))
	case !loaded:
		return append(lines, dim("Press c to load comments."))
	case len(comments) == 0:
		return append(lines, dim("No comments or reviews yet."))
	}
	cursor := min(m.commentCursor, len(comments)-1)
	item := comments[cursor]
	// position dots: ● for the current comment, · for the others (capped)
	var dots strings.Builder
	for i := range min(len(comments), max(1, width/2-12)) {
		if i == cursor {
			dots.WriteString("●")
		} else {
			dots.WriteString("·")
		}
	}
	lines = append(lines, row(width, nil, []span{fg(dots.String(), accent)}, []span{fg(fmt.Sprintf("%d/%d", cursor+1, len(comments)), ink), fg("  n next · p prev", subtle)}))
	lines = append(lines, "")
	name := item.Author
	if name == "" {
		name = "unknown author"
	}
	kind := item.Kind
	kindColor := muted
	switch {
	case strings.Contains(kind, "review"):
		kindColor = accent
	case item.Path != "":
		kindColor = amber
	}
	head := []span{bold(name, bright)}
	if kind != "" {
		head = append(head, gap(2), fg(kind, kindColor))
	}
	var when []span
	if !item.CreatedAt.IsZero() {
		when = []span{fg(item.CreatedAt.Local().Format("Jan 02 15:04"), subtle)}
	}
	lines = append(lines, row(width, nil, head, when))
	if item.Path != "" {
		location := item.Path
		if item.Line > 0 {
			location += ":" + strconv.Itoa(item.Line)
		}
		lines = append(lines, row(width, nil, []span{fg("↳ ", subtle), fg(location, amber)}, nil))
	}
	lines = append(lines, "")
	return append(lines, markdownLines(commentDisplayBody(item.Body), width)...)
}

func (m model) helpLines(width int) []string {
	keys := [][2]string{
		{"j k  ↑ ↓", "move through the queue"},
		{"g G", "first / last"},
		{"^d ^u", "page down / up"},
		{"1–5  tab", "all · attention · changed · no Jira · stacks"},
		{"/", "search titles, bodies, Jira, comments"},
		{"#", "jump to a PR number"},
		{"esc", "clear search and filter"},
		{"", ""},
		{"s d c i e", "overview · description · comments · jira · activity"},
		{"n p", "next / previous comment"},
		{"I", "next linked Jira issue"},
		{"{ }", "move down / up the PR's stack"},
		{"[ ]  ^b ^f", "scroll detail by line / half page"},
		{"", ""},
		{"↵  o", "open PR in browser"},
		{"J", "open Jira issue in browser"},
		{"r", fmt.Sprintf("sync now (auto every %ds)", m.collector.Config.PollSeconds)},
		{"q", "quit"},
	}
	lines := []string{section("KEYBOARD", width), ""}
	for _, k := range keys {
		if k[0] == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, row(width, nil, []span{fg(fmt.Sprintf("%-12s", k[0]), bright), fg(k[1], muted)}, nil))
	}
	return append(lines, "", dim("Any key closes this. prwale find N searches beyond loaded PRs."))
}

// ── chrome ───────────────────────────────────────────────────────────────

func (m model) headerLine(width int) (string, []int) {
	counts := []int{len(m.snapshot.PRs), 0, 0, 0, 0}
	for _, pr := range m.snapshot.PRs {
		if len(attentionReasons(pr)) > 0 {
			counts[1]++
		}
		if len(pr.Changes) > 0 {
			counts[2]++
		}
		if len(pr.JiraKeys) == 0 {
			counts[3]++
		}
		if pr.Stack != nil {
			counts[4]++
		}
	}
	names := []string{"All", "Attention", "Changed", "No Jira", "Stacks"}
	countColors := []color.Color{muted, red, teal, violet, ink}
	left := []span{gap(1), bold("prwale", accent), gap(4)}
	// columns where each tab starts and ends, for the underline
	marks := []int{}
	pos := spansWidth(left)
	for i, name := range names {
		nameSpan := fg(name, muted)
		countColor := subtle
		if counts[i] > 0 && i > 0 {
			countColor = countColors[i]
		}
		if m.filter == i {
			nameSpan = bold(name, bright)
		}
		cell := []span{nameSpan, gap(1), fg(strconv.Itoa(counts[i]), countColor)}
		if m.filter == i {
			marks = []int{pos, pos + spansWidth(cell)}
		}
		left = append(left, cell...)
		left = append(left, gap(3))
		pos += spansWidth(cell) + 3
	}
	var right []span
	switch {
	case m.loading:
		right = append(right, fg("◐ ", amber), fg("syncing", amber))
	case !m.snapshot.FetchedAt.IsZero():
		if m.showingCache {
			right = append(right, fg("○ ", muted), fg(verb("saved ", width)+m.snapshot.FetchedAt.Local().Format("15:04"), muted))
		} else {
			right = append(right, fg("● ", green), fg(verb("synced ", width)+m.snapshot.FetchedAt.Local().Format("15:04"), muted))
		}
	}
	if len(m.snapshot.Warnings) > 0 {
		right = append(right, fg(fmt.Sprintf("  ! %d", len(m.snapshot.Warnings)), amber))
	}
	if !m.loading && !m.nextSync.IsZero() && width >= 110 {
		right = append(right, fg("  next "+countdown(m.nextSync, m.now), subtle))
	}
	right = append(right, gap(1))
	return row(width, nil, left, right), marks
}

func verb(word string, width int) string {
	if width < 110 {
		return ""
	}
	return word
}

func (m model) ruleLine(width int, junction string, split int, marks []int) string {
	line := lipgloss.NewStyle().Foreground(border)
	hi := lipgloss.NewStyle().Foreground(accent)
	var b strings.Builder
	runStyle, run := line, 0
	flush := func() {
		if run > 0 {
			ch := "─"
			if runStyle.GetForeground() == accent {
				ch = "━"
			}
			b.WriteString(runStyle.Render(strings.Repeat(ch, run)))
		}
		run = 0
	}
	for x := 0; x < width; x++ {
		if split > 0 && x == split {
			flush()
			b.WriteString(line.Render(junction))
			continue
		}
		style := line
		if len(marks) == 2 && x >= marks[0] && x < marks[1] {
			style = hi
		}
		if style.GetForeground() != runStyle.GetForeground() {
			flush()
			runStyle = style
		}
		run++
	}
	flush()
	return b.String()
}

func (m model) footerLine(width int) string {
	var left []span
	key := func(k, what string) {
		left = append(left, fg(k, ink), fg(" "+what+"   ", subtle))
	}
	switch {
	case m.searching:
		left = []span{gap(1), bold("/", accent), fg(m.query, bright), fg("▌", accent), fg(fmt.Sprintf("   %d match", len(m.visible())), subtle)}
		if len(m.visible()) != 1 {
			left = append(left, fg("es", subtle))
		}
		left = append(left, fg("  ·  ↵ keep  esc clear", subtle))
	case m.numberSearching:
		left = []span{gap(1), bold("#", accent), fg(m.numberQuery, bright), fg("▌", accent), fg("   ↵ exact  esc clear", subtle)}
	default:
		left = append(left, gap(1))
		if m.query != "" {
			left = append(left, bold("/"+m.query, accent), fg("   ", subtle))
		}
		if m.numberQuery != "" {
			left = append(left, bold("#"+m.numberQuery, accent), fg("   ", subtle))
		}
		key("j/k", "move")
		key("/", "search")
		if width >= 110 {
			key("#", "number")
			key("↵", "open")
			key("J", "jira")
			key("r", "sync")
		}
		key("?", "keys")
	}
	var right []span
	if m.message != "" {
		c := subtle
		lower := strings.ToLower(m.message)
		if strings.Contains(lower, "failed") || strings.Contains(lower, "could not") {
			c = red
		}
		right = []span{fg(m.message, c), gap(1)}
	}
	return row(width, nil, left, right)
}

func (m model) View() tea.View {
	w, h := max(1, m.width), max(1, m.height)
	if w < 40 || h < 12 {
		view := tea.NewView(clip("prwale · enlarge the terminal", w))
		view.AltScreen = true
		view.MouseMode = tea.MouseModeCellMotion
		return view
	}
	header, marks := m.headerLine(w)
	bodyH := m.bodyHeight()
	var body []string
	split := 0
	divider := lipgloss.NewStyle().Foreground(border).Render("│")
	dw, dh := m.detailGeometry()
	detail := append(m.tabStrip(dw+4), m.paddedDetail(dw, dh)...)
	if m.wide() {
		leftW := m.leftWidth()
		split = leftW
		list := m.listLines(leftW, bodyH)
		for r := 0; r < bodyH; r++ {
			l, d := "", ""
			if r < len(list) {
				l = list[r]
			}
			if r < len(detail) {
				d = detail[r]
			}
			body = append(body, pad(l, leftW)+divider+pad(d, w-leftW-1))
		}
	} else {
		listH := m.listHeight()
		list := m.listLines(w, listH)
		for r := 0; r < listH; r++ {
			l := ""
			if r < len(list) {
				l = list[r]
			}
			body = append(body, pad(l, w))
		}
		body = append(body, rule(w))
		for r := 0; r < bodyH-listH-1; r++ {
			d := ""
			if r < len(detail) {
				d = detail[r]
			}
			body = append(body, pad(d, w))
		}
	}
	content := []string{header, m.ruleLine(w, "┬", split, marks)}
	content = append(content, body...)
	content = append(content, m.ruleLine(w, "┴", split, nil), m.footerLine(w))
	view := tea.NewView(strings.Join(content, "\n"))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

// paddedDetail adds a two-cell margin on both sides and, below the content,
// a scroll position line when the content is taller than the pane.
func (m model) paddedDetail(width, height int) []string {
	all := m.detailContent(width)
	padded := m.paddedDetailContent(width)
	start := max(0, min(m.detailOffset, len(padded)-height))
	out := make([]string, 0, height+1)
	out = append(out, padded[start:min(len(padded), start+height)]...)
	for len(out) < height {
		out = append(out, "")
	}
	hint := ""
	if overflow := len(all) - height; overflow > 0 {
		start := min(m.detailOffset, overflow)
		switch {
		case start == 0:
			hint = "] scroll for more"
		case start >= overflow:
			hint = "end · [ back"
		default:
			hint = fmt.Sprintf("%d%% · [ ] scroll", start*100/overflow)
		}
	}
	out = append(out, "  "+row(width, nil, nil, []span{fg(hint, subtle)})+"  ")
	return out
}

// stackMap draws the stack top-down, like the stack map on github.com:
//
//	STACK #42 · 3 PRs → main ───────────
//	┌ #7218  feat: part two          ◐ 3 running
//	├ #7214  feat: pin pg contract   ✗ 1 failing   ◂ this PR
//	└ #7210  fixtures                ✓ merged
//	  main
func (m model) stackMap(pr PR, width int) []string {
	s := pr.Stack
	title := fmt.Sprintf("STACK · %d PRs", len(s.Entries))
	if s.Native {
		title = fmt.Sprintf("STACK #%d · %d PRs", s.Number, len(s.Entries))
	}
	lines := []string{section(title, width)}
	mine := map[int]PR{}
	for _, p := range m.snapshot.PRs {
		if p.Repo == pr.Repo {
			mine[p.Number] = p
		}
	}
	for i := len(s.Entries) - 1; i >= 0; i-- {
		e := s.Entries[i]
		connector := "├ "
		switch {
		case len(s.Entries) == 1:
			connector = "─ "
		case i == len(s.Entries)-1:
			connector = "┌ "
		case i == 0:
			connector = "└ "
		}
		glyph, glyphColor, reason := "·", muted, e.State
		titleText := e.Title
		p, isMine := mine[e.Number]
		switch {
		case e.State == "merged":
			glyph, glyphColor, reason = "✓", green, "merged"
		case e.State == "closed":
			glyph, glyphColor, reason = "×", subtle, "closed"
		case isMine:
			glyph, glyphColor, reason = prSignal(p)
			titleText = p.Title
		case e.Draft:
			glyph, glyphColor, reason = "◇", muted, "draft"
		default:
			reason = "open · not yours"
		}
		if titleText == "" {
			titleText = e.Head
		}
		titleColor := ink
		if e.Number == pr.Number {
			titleColor = bright
		}
		left := []span{fg(connector, subtle), bold(glyph, glyphColor), gap(1), fg(fmt.Sprintf("#%d", e.Number), subtle), gap(2)}
		left = append(left, titleSpans(PR{Title: titleText, JiraKeys: p.JiraKeys}, titleColor, e.Number == pr.Number)...)
		reasonColor := muted
		if glyphColor == red {
			reasonColor = red
		}
		right := []span{fg(reason, reasonColor)}
		if e.Number == pr.Number {
			right = append(right, fg("  ◂ this", accent))
		}
		lines = append(lines, row(width, nil, left, right))
	}
	lines = append(lines, row(width, nil, []span{gap(2), fg(s.Base, muted)}, nil))
	if blocker, ok := stackBlocker(pr, m.snapshot.PRs); ok {
		_, _, reason := prSignal(blocker)
		lines = append(lines, row(width, nil, []span{fg("! ", red), fg(fmt.Sprintf("blocked below by #%d (%s)", blocker.Number, reason), red)}, nil))
	}
	source := "native GitHub stack"
	if !s.Native {
		source = "inferred from branches: each PR targets the one below"
	}
	lines = append(lines, row(width, nil, []span{fg(source, subtle), fg("   {", ink), fg(" down  ", subtle), fg("}", ink), fg(" up", subtle)}, nil))
	return lines
}

// liftSubtle swaps subtle text for muted: subtle is 2.4:1 on the selected
// background, below the 3:1 floor it is allowed (see docs/design-system).
func liftSubtle(spans []span) []span {
	out := make([]span, len(spans))
	for i, s := range spans {
		out[i] = s
		if s.style.GetForeground() == subtle {
			out[i].style = s.style.Foreground(muted)
		}
	}
	return out
}
