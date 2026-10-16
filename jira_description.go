package main

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Jira Cloud returns rich text as Atlassian Document Format. Keep all textual
// content, including lists and code blocks, in a form the terminal can scroll.
func jiraDescription(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return plain
	}
	var node jiraNode
	if json.Unmarshal(raw, &node) != nil {
		return ""
	}
	var b strings.Builder
	writeJiraNode(&b, node, "")
	return strings.TrimSpace(b.String())
}

type jiraNode struct {
	Type    string     `json:"type"`
	Text    string     `json:"text"`
	Content []jiraNode `json:"content"`
	Marks   []jiraMark `json:"marks"`
	Attrs   struct {
		Text      string `json:"text"`
		URL       string `json:"url"`
		ID        string `json:"id"`
		ShortName string `json:"shortName"`
		Level     int    `json:"level"`
		Language  string `json:"language"`
		PanelType string `json:"panelType"`
		State     string `json:"state"`
	} `json:"attrs"`
}

type jiraMark struct {
	Type  string `json:"type"`
	Attrs struct {
		Href string `json:"href"`
	} `json:"attrs"`
}

// markText applies ADF marks as markdown so the shared renderer styles them.
func markText(text string, marks []jiraMark) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	lead := text[:len(text)-len(strings.TrimLeft(text, " "))]
	trail := text[len(strings.TrimRight(text, " ")):]
	core := strings.TrimSpace(text)
	for _, m := range marks {
		switch m.Type {
		case "code":
			core = "`" + core + "`"
		case "strong":
			core = "**" + core + "**"
		case "em":
			core = "*" + core + "*"
		case "strike":
			core = "~~" + core + "~~"
		case "link":
			if m.Attrs.Href != "" {
				core = "[" + core + "](" + m.Attrs.Href + ")"
			}
		}
	}
	return lead + core + trail
}

func writeJiraNode(b *strings.Builder, n jiraNode, prefix string) {
	switch n.Type {
	case "text":
		b.WriteString(markText(n.Text, n.Marks))
		return
	case "heading":
		level := min(6, max(1, n.Attrs.Level))
		b.WriteString(strings.Repeat("#", level) + " ")
		for _, child := range n.Content {
			writeJiraNode(b, child, "")
		}
		b.WriteString("\n\n")
		return
	case "codeBlock":
		b.WriteString("```" + n.Attrs.Language + "\n")
		for _, child := range n.Content {
			b.WriteString(child.Text)
		}
		b.WriteString("\n```\n\n")
		return
	case "blockquote", "panel":
		var inner strings.Builder
		for _, child := range n.Content {
			writeJiraNode(&inner, child, "")
		}
		if n.Type == "panel" && n.Attrs.PanelType != "" {
			kind := map[string]string{"info": "NOTE", "note": "NOTE", "success": "TIP", "warning": "WARNING", "error": "CAUTION"}[n.Attrs.PanelType]
			if kind != "" {
				b.WriteString("> [!" + kind + "]\n")
			}
		}
		for _, line := range strings.Split(strings.TrimSpace(inner.String()), "\n") {
			b.WriteString("> " + line + "\n")
		}
		b.WriteString("\n")
		return
	case "table":
		for i, rowNode := range n.Content {
			writeJiraNode(b, rowNode, "")
			if i == 0 {
				b.WriteString("|" + strings.Repeat(" --- |", len(rowNode.Content)) + "\n")
			}
		}
		b.WriteString("\n")
		return
	case "hardBreak":
		b.WriteByte('\n')
		return
	case "mention":
		b.WriteString(n.Attrs.Text)
		return
	case "inlineCard", "blockCard":
		b.WriteString(n.Attrs.URL)
		if n.Type == "blockCard" {
			b.WriteByte('\n')
		}
		return
	case "emoji":
		if n.Attrs.Text != "" {
			b.WriteString(n.Attrs.Text)
		} else {
			b.WriteString(n.Attrs.ShortName)
		}
		return
	case "media":
		if n.Attrs.ID != "" {
			b.WriteString("[attachment: " + n.Attrs.ID + "]")
		}
		return
	case "tableRow":
		b.WriteString("| ")
		for i, cell := range n.Content {
			if i > 0 {
				b.WriteString(" | ")
			}
			var value strings.Builder
			for _, child := range cell.Content {
				writeJiraNode(&value, child, "")
			}
			b.WriteString(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(value.String()), "\n", " / "), "|", "\\|"))
		}
		b.WriteString(" |\n")
		return
	case "rule":
		b.WriteString("\n---\n\n")
		return
	case "bulletList", "orderedList":
		indent := strings.Repeat("  ", len(prefix)/2)
		for i, child := range n.Content {
			marker := "- "
			if n.Type == "orderedList" {
				marker = strconv.Itoa(i+1) + ". "
			}
			writeJiraNode(b, child, indent+marker)
		}
		return
	case "listItem":
		b.WriteString(prefix)
		for j, child := range n.Content {
			if child.Type == "bulletList" || child.Type == "orderedList" {
				nested := strings.Repeat(" ", len(prefix))
				writeJiraNode(b, child, nested[:len(nested)-len(nested)%2]+"  ")
				continue
			}
			var item strings.Builder
			writeJiraNode(&item, child, "")
			text := strings.TrimRight(item.String(), "\n")
			if j > 0 {
				text = strings.Repeat(" ", len(prefix)) + text
			}
			b.WriteString(text + "\n")
		}
		return
	case "taskList":
		for _, child := range n.Content {
			writeJiraNode(b, child, prefix)
		}
		return
	case "taskItem":
		box := "[ ] "
		if strings.EqualFold(n.Attrs.State, "DONE") {
			box = "[x] "
		}
		b.WriteString(prefix + "- " + box)
		for _, child := range n.Content {
			writeJiraNode(b, child, "")
		}
		b.WriteString("\n")
		return
	}
	for _, child := range n.Content {
		writeJiraNode(b, child, prefix)
	}
	if n.Type == "paragraph" {
		b.WriteString("\n\n")
	}
}
