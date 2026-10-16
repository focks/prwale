---
name: prwale-tui-review
description: Review the prwale TUI against its design system by rendering preview frames from fixture data, checking width/height, contrast and consistency. Use after UI changes, before committing view.go or markdown.go edits, or when asked whether prwale looks right.
---

# Review the prwale TUI

Use this to check a UI change visually and mechanically against `docs/design-system/`. No GitHub or Jira access is needed.

## 1. Render preview frames

`preview_test.go` renders fixture PRs covering every state: failing, conflict, changes requested, draft, running, approved, a branch stack, markdown bodies, inline comments. It renders at 200×56, 120×40 and 80×30, for the All and Stacks queues, in every detail tab.

```sh
skills/prwale-tui-review/scripts/preview.sh            # writes to /tmp/prwale-preview
skills/prwale-tui-review/scripts/preview.sh some/dir   # or a directory of your choice
```

Frames are named `<cols>x<rows>_<queue>_<tab>.ans` (for example `120x40_stacks_overview.ans`). View one in a real terminal at least as large as the frame:

```sh
cat /tmp/prwale-preview/120x40_all_overview.ans
```

To get an image (for an agent that reads screenshots): convert ANSI to HTML (`pip install ansi2html`, or `aha`), then screenshot it with a headless browser in a monospace font. Fixture data changes when you add states; extend `preview_test.go` for any new state you introduce so it is reviewable.

## 2. Mechanical checks

```sh
gofmt -l . && go vet . && go test ./...
python3 skills/prwale-tui-review/scripts/contrast.py      # after any color change
go test -run XXX -bench Scroll -benchmem .               # after any rendering change
```

`contrast.py` exits non-zero when a meaningful text color is below 4.5:1 on the terminal surface, or a badge is below 4.5:1. `subtle` has a 3:1 floor and must never carry information needed to act.

## 3. Look at every frame with this checklist

Layout
- [ ] Chrome is exactly four rows; the `┬` and `┴` junctions line up with the divider.
- [ ] No line is ragged: selected-row backgrounds reach the pane edge on both lines.
- [ ] Nothing wraps or overflows at 80 columns; the narrow layout stacks list over detail.
- [ ] Truncation keeps the important part: the reason survives on line two; titles end in `…`.

Meaning
- [ ] Every red item is something the user must fix; nothing waiting-on-others is red.
- [ ] Each row has exactly one state glyph, matching `prSignal` priority.
- [ ] Color is never the only signal: each colored state also has a glyph or a word.
- [ ] Accent appears only on navigation and focus: brand, selection bar, active tab, links, search.
- [ ] Only one filled badge is visible.

Content
- [ ] Copy follows `docs/design-system/content.md`: lowercase reasons, short ages, sentence empty states.
- [ ] Markdown renders with no raw syntax visible (`**`, `](`, three backticks, `<!--`, `[x]`).
- [ ] Stack map: top-down, current PR marked `◂ this`, trunk on the last line, blocker warning when expected.

## 4. Report

List findings grouped by severity: broken (overflow, wrong state, unreadable), inconsistent (deviates from the docs), polish. For each one give the frame name, what is wrong and the fix. If the docs are what is wrong, say so and propose the doc change.
