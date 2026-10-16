# Components

Each component names the Go function that renders it. All of them build lines from `span`s with `row(width, bg, left, right)`, which pads or truncates to exactly `width` cells and applies `bg` to every span, so inner color resets never punch holes in a selection.

## Header — `headerLine`

`prwale` in bold `accent`, then the queue tabs, then sync state on the right.

- **Queue tabs:** `All`, `Attention`, `Changed`, `No Jira`, `Stacks`, each with a count. The active tab's name is bold `bright` and gets a `━` `accent` underline cut into the rule below (`ruleLine`). Counts use their role color when non-zero (Attention red, Changed teal, No Jira violet), otherwise `subtle`.
- **Sync state:** `◐ syncing` (amber), `● synced 15:04` (green dot), `○ saved 15:04` (cached data), `! N` warnings (amber), `next mm:ss` (subtle, wide only).

## List group header — `layoutList`, `groupOf`

`NEEDS ATTENTION` red · `CHANGED` teal · `WAITING` amber · `QUIET` green, bold, with the count right-aligned in `subtle`. In the Stacks queue the header is `STACK` (bright) + repo + native `#number` + `→ base`.

## Queue row — `listItem`

Wide, two lines:

```
▍ ✗  test: pin import report contract   ● 6h
▍    #7234  2 failing  DEMO-42 To Do  stack 3/3  service   3 cmt
```

1. Selection bar (`▍` accent, or a space), state glyph, title, then right-aligned `●` (teal, changed) and age.
2. Five-cell indent, `#number`, reason, Jira key (violet) and status (colored by category), `stack n/m`, repo, and `N cmt` when it fits.

- **Title:** the conventional-commit prefix is `subtle`. A `[KEY]` already shown on line two is removed.
- **Reason color:** red when the PR is blocked on you, otherwise `muted`. The `stack n/m` tag is red when a PR below it needs attention.
- **Truncation order:** the title gives way first on line one; the repo, then the comment count, on line two.
- **Selected:** `selectedBG` behind both lines, title bold `bright`, `subtle` lifted to `muted`.

Narrow, one line: `▍ ✗ #7234 test: pin import report… 6h`.

## Detail tab strip — `tabStrip`

`s Overview  d Description  c Comments 3  i Jira 2  e Activity`. The key is `subtle`, the label `muted`; the active label is bold `bright` with a `━` underline. Counts appear once known. Help (`?`) marks `keys` on the right.

## Overview — `buildDetailContent` (mode 0)

1. Repo and `#number`, with `updated 6h` on the right; the title, wrapped, bold `bright`.
2. **State badge:** one filled badge per screen, `NEEDS ATTENTION` (red), `WAITING` (amber) or `CLEAR` (green), with its reasons in the same color.
3. **Fields:** `Checks`, `Review`, `Merge`, `Draft`, `Talk`, `Branch`. Labels are 10 cells wide in `subtle`; each value carries its own glyph and color (`checksSpans`, `reviewSpans`, `mergeSpans`).
4. **Sections:** `NEW SINCE LAST SYNC`, `STACK`, `JIRA`, `ACTIVITY`, `DESCRIPTION` (first 8 lines), then the hint line `d full description  c comments  i jira`. A section header is a bold `muted` label followed by a `border` rule (`section`).

## Stack map — `stackMap`

```
STACK #42 · 3 PRs ────────────────────────────────
┌ ◐ #7218  feat: part two              3 running
├ ✗ #7214  feat: pin pg contract       1 failing  ◂ this
└ ✓ #7210  fixtures                    merged
  main
! blocked below by #7210 (1 failing)
native GitHub stack   { down  } up
```

Drawn top-down, like github.com. Connectors are `subtle`. Each layer shows its own state: your PRs use `prSignal`; teammates' PRs show `open · not yours`, `draft`, `merged` or `closed`. The current PR is bold `bright` with an accent `◂ this`. The trunk is on the last line, and a red warning appears when a layer below needs attention. The source line says whether the stack is native or inferred from branches.

## Comments — `commentLines`

A dot strip (`●··`, accent) with `2/8  n next · p prev`, then the author (bold `bright`), kind (`review` in accent, `inline` in amber), time on the right, `↳ path:line` in amber, and the markdown body.

## Markdown — `renderMarkdown`

| Element | Rendering |
| --- | --- |
| `#` / `##` | bold `bright` (H1 underlined); `###` and below bold `ink`; a blank line before |
| `**bold**` `*italic*` `~~strike~~` | bold / italic / struck through in `muted` |
| `` `code` `` | `bright` on `codeBG` |
| fenced code | full-width `codeBG` block with a language label on the right, hard-wrapped, never reflowed |
| `[text](url)`, bare URLs | `accent`, underlined, OSC 8 hyperlink |
| `![alt](url)` | `▣ alt` in `muted`, linked |
| `-` `*` `+` lists | `•` `◦` `▪` by depth (2 spaces per level), continuation lines aligned |
| `1.` lists | number in `subtle` |
| `- [ ]` / `- [x]` | `○` subtle / `✓` green |
| `>` quote | `▎` bar in `border`, content rendered recursively |
| `> [!NOTE]` and other alerts | bar in the alert color (NOTE accent, TIP green, IMPORTANT violet, WARNING/CAUTION amber), bold label |
| tables | bold `bright` header, `─┼─` rule, ` │ ` dividers in `border`; widest columns shrink first |
| `---` | full-width rule |
| `<!-- … -->` | hidden |
| `<details>`, `<summary>` | summary as bold `▸ text`; other layout tags removed |

Newlines are hard breaks, as on github.com. `snake_case` words are never italicized.

## Footer — `footerLine`

Left: key hints (key in `ink`, label in `subtle`), or the live search prompt (`/query▌  N matches · ↵ keep  esc clear`). Right: the last message in `subtle`, or `red` when it reports a failure.

## Scroll hint — `paddedDetail`

The bottom row of the detail pane, right-aligned in `subtle`: `] scroll for more`, `40% · [ ] scroll`, `end · [ back`. It is empty when everything fits.
