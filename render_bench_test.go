package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func benchModel() model {
	m := newModel(Collector{Config: defaultConfig()}, WatchState{})
	m.width, m.height = 200, 56
	var prs []PR
	body := strings.Repeat("## Section\nSome long paragraph with `code` and words that wrap across the pane many times over. ", 40) + "\n- bullet one\n- bullet two\n```\ncode block\n```\n"
	for i := 0; i < 60; i++ {
		prs = append(prs, PR{ID: "a/b#" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Repo: "a/b", Number: 7000 + i, Title: "fix: something", Body: body, Checks: "1 failed · 3 passed", JiraKeys: []string{"AI-1"}, Issues: []Issue{{Key: "AI-1", Description: body}}, UpdatedAt: time.Now()})
	}
	m.snapshot = Snapshot{PRs: prs, FetchedAt: time.Now()}
	m.loading = false
	m.detailMode = 1
	return m
}

func BenchmarkScrollDescription(b *testing.B) {
	m := benchModel()
	for i := 0; i < b.N; i++ {
		next, _ := m.Update(tea.MouseWheelMsg{X: 150, Y: 20, Button: tea.MouseWheelDown})
		m = next.(model)
		if m.detailOffset > 200 {
			m.detailOffset = 0
		}
		_ = m.View()
	}
}

func BenchmarkScrollDescriptionSearching(b *testing.B) {
	m := benchModel()
	m.query = "something"
	for i := 0; i < b.N; i++ {
		next, _ := m.Update(tea.MouseWheelMsg{X: 150, Y: 20, Button: tea.MouseWheelDown})
		m = next.(model)
		if m.detailOffset > 200 {
			m.detailOffset = 0
		}
		_ = m.View()
	}
}
