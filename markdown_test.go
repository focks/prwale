package main

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestMarkdownRendering(t *testing.T) {
	body := strings.Join([]string{
		"<!-- template: hidden -->",
		"## What / How",
		"Pins the **existing** `build_import_report` contract, see [the spec](https://example.com/spec) and snake_case_names.",
		"- top level",
		"  - nested item",
		"- [x] done task",
		"- [ ] open task",
		"1. first",
		"> [!WARNING]",
		"> careful here",
		"```sql",
		"select 1",
		"```",
		"| Window | Factor |",
		"| --- | --- |",
		"| 30 day | 365 / 29 |",
		"---",
		"~~old~~ new",
	}, "\n")
	lines := renderMarkdown(body, 60)
	plain := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"What / How", "Pins the existing build_import_report contract", "the spec", "snake_case_names", "• top level", "◦ nested item", "✓ done task", "○ open task", "1. first", "Warning", "careful here", "select 1", "Window", "365 / 29", "old new"} {
		if !strings.Contains(plain, want) {
			t.Errorf("missing %q in:\n%s", want, plain)
		}
	}
	for _, unwanted := range []string{"template: hidden", "**", "](", "```", "[x]", "[!WARNING]"} {
		if strings.Contains(plain, unwanted) {
			t.Errorf("raw markdown %q leaked:\n%s", unwanted, plain)
		}
	}
	for _, line := range lines {
		if w := lipgloss.Width(line); w > 60 {
			t.Fatalf("line width %d > 60: %q", w, ansi.Strip(line))
		}
	}
}

func TestMarkdownWrapsLongWords(t *testing.T) {
	for _, line := range renderMarkdown(strings.Repeat("x", 200)+" tail", 30) {
		if lipgloss.Width(line) > 30 {
			t.Fatalf("overflow: %d", lipgloss.Width(line))
		}
	}
}
