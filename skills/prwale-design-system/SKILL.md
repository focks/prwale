---
name: prwale-design-system
description: Build or change anything visible in the prwale TUI (rows, panes, tabs, colors, glyphs, markdown, copy) following docs/design-system. Use for any edit to view.go or markdown.go, or a new screen, state, queue or section.
---

# prwale design system

prwale's look is specified in `docs/design-system/`. This skill is how to change the UI without drifting from it.

## Before you write code

1. Read `docs/design-system/README.md` (principles), then the parts of `foundations.md`, `components.md` and `content.md` that cover what you are touching.
2. Find the existing component that is closest to what you need (each one in `components.md` names its Go function) and extend it instead of building a parallel one.
3. Decide the **meaning** of anything new before its look: is it blocked on the user (red), waiting on others (amber), clear (green), changed (teal), Jira (violet), or navigation (accent)? If it is none of these it is `ink`, `muted` or `subtle` text. Only add a new color role when no existing meaning fits. A new role means a token in `tokens.json`, a Go variable, a row in `foundations.md`, and a contrast check, all in the same change.

## Building blocks (view.go)

- Compose lines from spans: `fg(text, color)`, `bold(text, color)`, `badge(text, bg)`, `gap(n)`.
- Lay a line out with `row(width, bg, left, right)`. It returns exactly `width` cells, truncates the left side first with `…`, and applies `bg` to every span. Never pass a pre-styled string to `lipgloss.NewStyle().Background(…).Render`: inner resets cut holes in the background. For a selected row, pass `selectedBG` as `bg` and run spans through `liftSubtle`.
- Use `section(title, width)` for detail sections, `field(width, label, spans)` for label/value rows, `rule(width)` for dividers.
- Render user-written text (PR bodies, comments, Jira) with `renderMarkdown(body, width)`, never raw.
- State comes from `prSignal(pr)` (glyph, color, reason) and `overview(pr)` (badge). Add a new PR state there, in priority order, and add it to the glyph table in `foundations.md`.
- Glyphs must be single-cell. No emoji. Prefer one already listed in `foundations.md`.

## Layout rules that tests enforce

- Every line of `View().Content` is ≤ the terminal width and the frame height ≤ the terminal height; the tests check this at 70, 100, 120 and 140 columns. Check 80 and 200 with the preview frames.
- The detail pane's last row is the scroll hint. Content height comes from `detailGeometry()`. If you change geometry, keep `detailGeometry`, `listHeight`, `mouseInList` and `listIndexAt` consistent.
- The list maps screen lines to PRs via `listView`'s second return value (used for mouse clicks). New header or spacer lines must return `-1` there.

## Performance

`View` runs on every keypress, scroll tick and the 1 s pulse. Anything proportional to body length or PR count goes behind `render_cache.go`: add whatever new inputs your content reads to the cache key (`detailContent` key, `visibleKey`, or the list key), or the screen will show stale data. Check with:

```sh
go test -run XXX -bench Scroll -benchmem .
```

A scroll tick should stay well under 1 ms on the benchmark.

## Copy

Follow `content.md`: lowercase reasons with numbers (`2 failing`), short ages (`6h`), UPPERCASE group and section labels, empty states as full sentences that say how to fix them, `key what` hint pairs.

## Finish

1. `gofmt -l .`, `go vet .` and `go test ./...` must be clean.
2. Run the `prwale-tui-review` skill: render previews, look at them at all three sizes, and run the contrast check if colors changed.
3. Update `docs/design-system/` in the same change for anything a future reader would otherwise get wrong: new tokens, glyphs, components, or changed behavior. Update the Keys table in `README.md` for new bindings, and the in-app help in `helpLines`.
