package main

import (
	"image/color"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// A small GitHub-flavoured markdown renderer for the terminal. It covers what
// PR descriptions, review comments and Jira issues actually use: headings,
// emphasis, inline code, links, nested and task lists, block quotes, fenced
// code, tables and rules. Every line it returns is at most width cells wide.
// Newlines are hard breaks, as on github.com.

var (
	codeBG      = lipgloss.Color("#202734")
	reHeading   = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	reRule      = regexp.MustCompile(`^\s{0,3}([-*_])(\s*[-*_]){2,}\s*$`)
	reList      = regexp.MustCompile(`^(\s*)([-*+]|\d{1,9}[.)])\s+(.*)$`)
	reTask      = regexp.MustCompile(`^\[([ xX])\]\s+(.*)$`)
	reTableSep  = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)
	reHTMLTag   = regexp.MustCompile(`</?(?:details|summary|p|div|span|sub|sup|br|hr|img|picture|source|kbd|b|i|em|strong|a|code|ul|ol|li|h[1-6])\b[^>]*>`)
	reBareURL   = regexp.MustCompile(`https?://[^\s<>()\[\]]+[^\s<>()\[\].,;:!?'"]`)
	reSummary   = regexp.MustCompile(`(?i)<summary>(.*?)</summary>`)
	reAlert     = regexp.MustCompile(`^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*$`)
	reFence     = regexp.MustCompile("^\\s{0,3}(```+|~~~+)\\s*([\\w+#.-]*)")
	reHTMLStart = regexp.MustCompile(`<!--`)
)

func renderMarkdown(body string, width int) []string {
	width = max(1, width)
	body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\t", "    ")
	if strings.TrimSpace(body) == "" {
		return []string{dim("No description provided.")}
	}
	src := strings.Split(body, "\n")
	var out []string
	blank := func() {
		if len(out) > 0 && out[len(out)-1] != "" {
			out = append(out, "")
		}
	}
	inComment := false
	for i := 0; i < len(src); i++ {
		raw := src[i]
		trimmed := strings.TrimSpace(raw)

		// HTML comments are metadata (bot markers, templates): hide them.
		if inComment || reHTMLStart.MatchString(trimmed) {
			before, after, _ := strings.Cut(raw, "<!--")
			if inComment {
				before, after = "", raw
			}
			if _, rest, closed := strings.Cut(after, "-->"); closed {
				inComment = false
				raw = before + rest
				trimmed = strings.TrimSpace(raw)
				if trimmed == "" {
					continue
				}
			} else {
				inComment = true
				if strings.TrimSpace(before) == "" {
					continue
				}
				raw, trimmed = before, strings.TrimSpace(before)
			}
		}

		if m := reFence.FindStringSubmatch(raw); m != nil {
			fence := m[1]
			var code []string
			for i++; i < len(src); i++ {
				if strings.HasPrefix(strings.TrimSpace(src[i]), fence[:3]) {
					break
				}
				code = append(code, src[i])
			}
			blank()
			out = append(out, codeBlock(code, m[2], width)...)
			out = append(out, "")
			continue
		}

		if trimmed == "" {
			blank()
			continue
		}

		if strings.HasPrefix(trimmed, "|") && i+1 < len(src) && reTableSep.MatchString(src[i+1]) {
			rows := [][]string{splitRow(trimmed)}
			i += 2
			for ; i < len(src) && strings.HasPrefix(strings.TrimSpace(src[i]), "|"); i++ {
				rows = append(rows, splitRow(strings.TrimSpace(src[i])))
			}
			i--
			blank()
			out = append(out, table(rows, width)...)
			out = append(out, "")
			continue
		}

		if m := reHeading.FindStringSubmatch(trimmed); m != nil {
			blank()
			level := len(m[1])
			base := lipgloss.NewStyle().Foreground(bright).Bold(true)
			if level >= 3 {
				base = lipgloss.NewStyle().Foreground(ink).Bold(true)
			}
			text := m[2]
			if level == 1 {
				base = base.Underline(true)
			}
			out = append(out, wrapInline(parseInline(text, base), width, nil, nil)...)
			continue
		}

		if reRule.MatchString(raw) {
			blank()
			out = append(out, rule(width), "")
			continue
		}

		if strings.HasPrefix(trimmed, ">") {
			var quote []string
			for ; i < len(src) && strings.HasPrefix(strings.TrimSpace(src[i]), ">"); i++ {
				line := strings.TrimPrefix(strings.TrimSpace(src[i]), ">")
				quote = append(quote, strings.TrimPrefix(line, " "))
			}
			i--
			barColor := color.Color(border)
			if len(quote) > 0 {
				if m := reAlert.FindStringSubmatch(strings.TrimSpace(quote[0])); m != nil {
					barColor = alertColor(m[1])
					quote[0] = "**" + strings.ToUpper(m[1][:1]) + strings.ToLower(m[1][1:]) + "**"
				}
			}
			bar := []span{fg("▎ ", barColor)}
			for _, line := range renderMarkdown(strings.Join(quote, "\n"), max(1, width-2)) {
				out = append(out, row(width, nil, append(append([]span{}, bar...), span{line, lipgloss.NewStyle()}), nil))
			}
			continue
		}

		if m := reList.FindStringSubmatch(raw); m != nil {
			depth := min(4, len(m[1])/2)
			marker, text := m[2], m[3]
			var lead []span
			indent := strings.Repeat("  ", depth)
			if t := reTask.FindStringSubmatch(text); t != nil {
				text = t[2]
				if t[1] == " " {
					lead = []span{fg(indent+"○ ", subtle)}
				} else {
					lead = []span{fg(indent+"✓ ", green)}
				}
			} else if marker[0] >= '0' && marker[0] <= '9' {
				lead = []span{fg(indent+strings.TrimRight(marker, ".)")+". ", subtle)}
			} else {
				glyphs := []string{"• ", "◦ ", "▪ ", "◦ ", "▪ "}
				lead = []span{fg(indent+glyphs[depth], subtle)}
			}
			cont := []span{gap(spansWidth(lead))}
			out = append(out, wrapInline(parseInline(text, lipgloss.NewStyle().Foreground(ink)), width, lead, cont)...)
			continue
		}

		// <details><summary>…</summary> → a bold label; other layout tags vanish.
		if m := reSummary.FindStringSubmatch(trimmed); m != nil {
			out = append(out, wrapInline(parseInline("▸ "+m[1], lipgloss.NewStyle().Foreground(ink).Bold(true)), width, nil, nil)...)
			continue
		}
		cleaned := strings.TrimSpace(reHTMLTag.ReplaceAllString(trimmed, ""))
		if cleaned == "" {
			continue
		}
		indent := min(8, len(raw)-len(strings.TrimLeft(raw, " ")))
		var lead []span
		if indent >= 2 {
			lead = []span{gap(indent / 2 * 2)}
		}
		out = append(out, wrapInline(parseInline(cleaned, lipgloss.NewStyle().Foreground(ink)), width, lead, lead)...)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return []string{dim("No description provided.")}
	}
	return out
}

func alertColor(kind string) color.Color {
	switch kind {
	case "WARNING", "CAUTION":
		return amber
	case "IMPORTANT":
		return violet
	case "TIP":
		return green
	}
	return accent
}

func codeBlock(lines []string, lang string, width int) []string {
	style := lipgloss.NewStyle().Foreground(ink).Background(codeBG)
	var out []string
	if lang != "" {
		out = append(out, row(width, codeBG, []span{gap(1)}, []span{fg(lang, subtle), gap(1)}))
	}
	inner := max(1, width-2)
	for _, line := range lines {
		for _, part := range hardWrap(line, inner) {
			out = append(out, style.Render(" "+part+strings.Repeat(" ", max(0, inner-lipgloss.Width(part)))+" "))
		}
	}
	if len(lines) == 0 {
		out = append(out, style.Render(strings.Repeat(" ", width)))
	}
	return out
}

// hardWrap splits a line into chunks of at most width cells without
// reflowing words, so code keeps its shape.
func hardWrap(line string, width int) []string {
	if lipgloss.Width(line) <= width {
		return []string{line}
	}
	var parts []string
	var cur strings.Builder
	curW := 0
	for _, r := range line {
		rw := max(0, ansi.StringWidth(string(r)))
		if curW+rw > width {
			parts = append(parts, cur.String())
			cur.Reset()
			curW = 0
		}
		cur.WriteRune(r)
		curW += rw
	}
	return append(parts, cur.String())
}

func splitRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	var cells []string
	var cur strings.Builder
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' && i+1 < len(line) && line[i+1] == '|' {
			cur.WriteByte('|')
			i++
			continue
		}
		if line[i] == '|' {
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(line[i])
	}
	return append(cells, strings.TrimSpace(cur.String()))
}

func table(rows [][]string, width int) []string {
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	cells := make([][][]span, len(rows))
	widths := make([]int, cols)
	for i, r := range rows {
		cells[i] = make([][]span, cols)
		for c := 0; c < cols; c++ {
			text := ""
			if c < len(r) {
				text = r[c]
			}
			base := lipgloss.NewStyle().Foreground(ink)
			if i == 0 {
				base = lipgloss.NewStyle().Foreground(bright).Bold(true)
			}
			cells[i][c] = parseInline(text, base)
			widths[c] = max(widths[c], spansWidth(cells[i][c]))
		}
	}
	sep := 3
	avail := width - sep*(cols-1)
	for total(widths) > avail {
		widest := 0
		for c := range widths {
			if widths[c] > widths[widest] {
				widest = c
			}
		}
		if widths[widest] <= 3 {
			break
		}
		widths[widest]--
	}
	divider := fg(" │ ", border)
	var out []string
	for i := range cells {
		var line []span
		for c := 0; c < cols; c++ {
			if c > 0 {
				line = append(line, divider)
			}
			rendered, used := renderSpans(cells[i][c], widths[c], nil)
			line = append(line, span{rendered, lipgloss.NewStyle()}, gap(widths[c]-used))
		}
		out = append(out, row(width, nil, line, nil))
		if i == 0 {
			var b strings.Builder
			for c, w := range widths {
				if c > 0 {
					b.WriteString("─┼─")
				}
				b.WriteString(strings.Repeat("─", w))
			}
			out = append(out, clip(lipgloss.NewStyle().Foreground(border).Render(b.String()), width))
		}
	}
	return out
}

func total(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}

// ── inline ───────────────────────────────────────────────────────────────

type inlineFlags struct {
	bold, italic, strike bool
}

func styleFor(base lipgloss.Style, f inlineFlags) lipgloss.Style {
	s := base
	if f.bold {
		s = s.Bold(true)
	}
	if f.italic {
		s = s.Italic(true)
	}
	if f.strike {
		s = s.Strikethrough(true).Foreground(muted)
	}
	return s
}

func isWordByte(b byte) bool {
	r, _ := utf8.DecodeRuneInString(string(b))
	return b >= 0x80 || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func linkSpan(text, url string, base lipgloss.Style) span {
	style := base.Foreground(accent).Underline(true)
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		style = style.Hyperlink(url)
	}
	return span{text, style}
}

// parseInline turns one line of inline markdown into styled spans.
func parseInline(s string, base lipgloss.Style) []span {
	var out []span
	var buf strings.Builder
	var f inlineFlags
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		text := buf.String()
		buf.Reset()
		style := styleFor(base, f)
		// bare URLs become links
		last := 0
		for _, loc := range reBareURL.FindAllStringIndex(text, -1) {
			if loc[0] > last {
				out = append(out, span{text[last:loc[0]], style})
			}
			out = append(out, linkSpan(text[loc[0]:loc[1]], text[loc[0]:loc[1]], style))
			last = loc[1]
		}
		if last < len(text) {
			out = append(out, span{text[last:], style})
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && strings.IndexByte("\\`*_{}[]()#+-.!|~<>", s[i+1]) >= 0:
			buf.WriteByte(s[i+1])
			i++
		case c == '`':
			run := 1
			for i+run < len(s) && s[i+run] == '`' {
				run++
			}
			fence := strings.Repeat("`", run)
			if end := strings.Index(s[i+run:], fence); end >= 0 {
				flush()
				code := strings.TrimSpace(s[i+run : i+run+end])
				out = append(out, span{code, lipgloss.NewStyle().Foreground(bright).Background(codeBG)})
				i += run + end + run - 1
			} else {
				buf.WriteString(fence)
				i += run - 1
			}
		case (c == '*' || c == '_') && i+1 < len(s) && s[i+1] == c:
			marker := s[i : i+2]
			opening := !f.bold
			ok := true
			if c == '_' {
				if opening {
					ok = i == 0 || !isWordByte(s[i-1])
				} else {
					ok = i+2 >= len(s) || !isWordByte(s[i+2])
				}
			}
			if ok && opening {
				ok = strings.Contains(s[i+2:], marker)
			}
			if ok {
				flush()
				f.bold = !f.bold
			} else {
				buf.WriteString(marker)
			}
			i++
		case c == '*' || c == '_':
			opening := !f.italic
			ok := false
			if opening {
				ok = i+1 < len(s) && s[i+1] != ' ' && strings.IndexByte(s[i+1:], c) > 0
				if c == '_' && i > 0 && isWordByte(s[i-1]) {
					ok = false // snake_case
				}
			} else {
				ok = i > 0 && s[i-1] != ' '
				if c == '_' && i+1 < len(s) && isWordByte(s[i+1]) {
					ok = false
				}
			}
			if ok {
				flush()
				f.italic = !f.italic
			} else {
				buf.WriteByte(c)
			}
		case c == '~' && i+1 < len(s) && s[i+1] == '~' && (f.strike || strings.Contains(s[i+2:], "~~")):
			flush()
			f.strike = !f.strike
			i++
		case c == '!' && i+1 < len(s) && s[i+1] == '[':
			if text, url, n, ok := parseLink(s[i+1:]); ok {
				flush()
				label := text
				if label == "" {
					label = "image"
				}
				out = append(out, linkSpan("▣ "+label, url, styleFor(base, f).Foreground(muted)))
				i += n
			} else {
				buf.WriteByte(c)
			}
		case c == '[':
			if text, url, n, ok := parseLink(s[i:]); ok {
				flush()
				for _, inner := range parseInline(text, styleFor(base, f)) {
					out = append(out, linkSpan(inner.text, url, inner.style))
				}
				i += n - 1
			} else {
				buf.WriteByte(c)
			}
		case c == '<' && strings.HasPrefix(s[i:], "<http"):
			if end := strings.IndexByte(s[i:], '>'); end > 0 {
				flush()
				url := s[i+1 : i+end]
				out = append(out, linkSpan(url, url, styleFor(base, f)))
				i += end
			} else {
				buf.WriteByte(c)
			}
		default:
			buf.WriteByte(c)
		}
	}
	flush()
	return out
}

// parseLink reads "[text](url)" at the start of s and returns the number of
// bytes consumed.
func parseLink(s string) (string, string, int, bool) {
	if len(s) == 0 || s[0] != '[' {
		return "", "", 0, false
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				if i+1 < len(s) && s[i+1] == '(' {
					if end := strings.IndexByte(s[i+2:], ')'); end >= 0 {
						url := strings.Fields(s[i+2 : i+2+end])
						u := ""
						if len(url) > 0 {
							u = url[0]
						}
						return s[1:i], u, i + 2 + end + 1, true
					}
				}
				return "", "", 0, false
			}
		}
	}
	return "", "", 0, false
}

// wrapInline word-wraps styled spans to width. first prefixes the first line,
// rest every following line (both may be nil).
func wrapInline(spans []span, width int, first, rest []span) []string {
	type token struct {
		text  string
		style lipgloss.Style
		space bool
		src   int
	}
	var tokens []token
	for n, s := range spans {
		start := 0
		for i, r := range s.text {
			if r == ' ' {
				if i > start {
					tokens = append(tokens, token{s.text[start:i], s.style, false, n})
				}
				tokens = append(tokens, token{" ", s.style, true, n})
				start = i + 1
			}
		}
		if start < len(s.text) {
			tokens = append(tokens, token{s.text[start:], s.style, false, n})
		}
	}
	var lines []string
	prefix := first
	var cur []span
	var curSrc []int
	add := func(text string, style lipgloss.Style, src int) {
		if n := len(cur); n > 0 && curSrc[n-1] == src {
			cur[n-1].text += text
			return
		}
		cur = append(cur, span{text, style})
		curSrc = append(curSrc, src)
	}
	curW := spansWidth(prefix)
	avail := func() int { return width }
	emit := func() {
		// drop trailing spaces
		if n := len(cur); n > 0 {
			cur[n-1].text = strings.TrimRight(cur[n-1].text, " ")
		}
		line := append(append([]span{}, prefix...), cur...)
		rendered, used := renderSpans(line, width, nil)
		lines = append(lines, rendered+strings.Repeat(" ", max(0, width-used)))
		prefix, cur, curSrc = rest, nil, nil
		curW = spansWidth(prefix)
	}
	for _, t := range tokens {
		tw := lipgloss.Width(t.text)
		if t.space {
			if len(cur) == 0 {
				continue
			}
			if curW+tw > avail() {
				emit()
				continue
			}
			add(t.text, t.style, t.src)
			curW += tw
			continue
		}
		if curW+tw > avail() && len(cur) > 0 {
			emit()
		}
		room := avail() - curW
		for tw > room && room > 0 {
			// a single word longer than the line: split it
			head := ansi.Truncate(t.text, room, "")
			if head == "" {
				break
			}
			add(head, t.style, t.src)
			emit()
			t.text = strings.TrimPrefix(t.text, head)
			tw = lipgloss.Width(t.text)
			room = avail() - curW
		}
		add(t.text, t.style, t.src)
		curW += tw
	}
	if len(cur) > 0 || len(lines) == 0 {
		emit()
	}
	return lines
}

// markdownLines is kept as the entry point used by the detail pane.
func markdownLines(body string, width int) []string { return renderMarkdown(body, width) }
