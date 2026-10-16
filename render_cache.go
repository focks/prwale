package main

import "fmt"

// renderCache memoizes the expensive parts of a frame. Scrolling and the
// one-second pulse re-render constantly, but the filtered queue, the list
// rows and the wrapped detail body only change when their inputs change.
// The cache is a pointer so every copy of the (value-typed) model shares it;
// entries are keyed on everything they read, so a stale hit is impossible.
type renderCache struct {
	visKey string
	vis    []int

	listKey  string
	list     []string
	listRows []int

	detailKey    string
	detail       []string
	detailPadded []string
}

func (m model) prsKey() string {
	var first *PR
	if len(m.snapshot.PRs) > 0 {
		first = &m.snapshot.PRs[0]
	}
	return fmt.Sprintf("%p/%d/%d", first, len(m.snapshot.PRs), m.snapshot.FetchedAt.UnixNano())
}

func (m model) visibleKey() string {
	comments := 0
	if m.query != "" {
		for _, items := range m.commentsByPR {
			comments += len(items) + 1
		}
	}
	return fmt.Sprintf("%s|%d|%q|%q|%t|%d", m.prsKey(), m.filter, m.query, m.numberQuery, m.numberExact, comments)
}

func (m model) visible() []int {
	if m.cache == nil {
		return m.computeVisible()
	}
	key := m.visibleKey()
	if key != m.cache.visKey || m.cache.vis == nil {
		m.cache.vis = m.computeVisible()
		if m.cache.vis == nil {
			m.cache.vis = []int{}
		}
		m.cache.visKey = key
	}
	return m.cache.vis
}

func (m model) listView(width, height int) ([]string, []int) {
	if m.cache == nil {
		return m.buildListView(width, height)
	}
	key := fmt.Sprintf("%s|%d|%d|%d|%d|%d|%t|%d", m.visibleKey(), m.selected, m.offset, width, height, m.width, m.loading, m.now.Unix()/60)
	if key != m.cache.listKey {
		m.cache.list, m.cache.listRows = m.buildListView(width, height)
		m.cache.listKey = key
	}
	return m.cache.list, m.cache.listRows
}

func (m model) detailContent(width int) []string {
	if m.cache == nil {
		return m.buildDetailContent(width)
	}
	prKey, comments := "-", ""
	if pr, ok := m.chosen(); ok {
		items, loaded := m.commentsByPR[pr.ID]
		var first *Comment
		if len(items) > 0 {
			first = &items[0]
		}
		comments = fmt.Sprintf("%t/%p/%d/%t", loaded, first, len(items), m.commentsLoadingFor == pr.ID)
		prKey = pr.ID + "|" + ageLabel(pr.UpdatedAt, m.now)
	}
	key := fmt.Sprintf("%s|%s|%d|%t|%d|%d|%d|%s|%d|%d", m.prsKey(), prKey, m.detailMode, m.help, width, m.jiraIndex, m.commentCursor, comments, len(m.state.RecentEvents), m.collector.Config.PollSeconds)
	if key != m.cache.detailKey {
		m.cache.detail = m.buildDetailContent(width)
		m.cache.detailPadded = nil
		m.cache.detailKey = key
	}
	return m.cache.detail
}

// paddedDetailContent is detailContent with every line padded to the pane
// width and given its two-cell side margins, computed once per content.
func (m model) paddedDetailContent(width int) []string {
	lines := m.detailContent(width)
	if m.cache != nil && m.cache.detailPadded != nil {
		return m.cache.detailPadded
	}
	padded := make([]string, len(lines))
	for i, line := range lines {
		padded[i] = "  " + pad(line, width) + "  "
	}
	if m.cache != nil {
		m.cache.detailPadded = padded
	}
	return padded
}
