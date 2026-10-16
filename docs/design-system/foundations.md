# Foundations

## Color

prwale never paints the terminal background. Colors are foregrounds, plus three backgrounds: the selected row, code, and the one state badge. Go names are in `view.go` and `markdown.go`; hex values and usage notes are in `tokens.json`.

| Token | Go | Hex | Role |
| --- | --- | --- | --- |
| surface | — | `#15181E` | Reference terminal background, for previews and contrast checks only |
| selected | `selectedBG` | `#243044` | Selected queue row, both lines, full pane width |
| code-bg | `codeBG` | `#202734` | Inline code and fenced code blocks |
| border | `border` | `#384254` | Rules, pane divider, `┬` `┴` junctions, inactive tab underline |
| bright | `bright` | `#F2F4F8` | Titles, selected row title, active tab label, headings |
| ink | `ink` | `#D3D9E3` | Body text |
| muted | `muted` | `#8E98AA` | Secondary text: repo, inactive tabs, non-urgent reasons |
| subtle | `subtle` | `#5F697B` | Hints, labels, timestamps, commit prefixes |
| accent | `accent` | `#7AB8FF` | Focus and navigation only: brand, `▍` selection bar, `━` active tab, search prompt, links |
| red | `red` | `#FF7A8A` | Blocked on you |
| amber | `amber` | `#F2C46D` | Waiting on others |
| green | `green` | `#7FD6A0` | Clear: passing, approved, merged, synced |
| teal | `teal` | `#5FD4C8` | Changed since the last sync |
| violet | `violet` | `#B5A4FF` | Jira identity: keys and the No Jira count |
| on-fill | `onFill` | `#0F131A` | Text on a filled badge |

### Rules

- **Add a meaning, not a color.** A new state uses an existing role. A new role needs a new token here, in `tokens.json` and in Go, in the same change.
- **Contrast.** Text that carries meaning is 4.5:1 or more on `surface`. `subtle` is the only exception, with a 3:1 floor: use it only for things nobody needs to read to act. On the selected row `subtle` drops to 2.4:1, so `liftSubtle` turns it into `muted` there. Run `skills/prwale-tui-review/scripts/contrast.py` after any color change.
- **Color is never the only signal.** Every state also has a glyph and a word (`✗ 2 failing`), so the UI still reads in monochrome and for color-blind users. Red and green never appear without their glyphs.

## Glyphs

Single-cell Unicode only. No emoji, because their width varies between terminals and breaks alignment.

| Glyph | Color | Meaning | Priority |
| --- | --- | --- | --- |
| `⊘` | red | Merge conflict | 1 |
| `✗` | red | Checks failing | 2 |
| `↺` | red | Changes requested | 3 |
| `?` | muted | GitHub detail unavailable | 4 |
| `◇` | muted | Draft | 5 |
| `◐` | amber | Checks running | 6 |
| `○` | amber | Review required | 7 |
| `✓` | green | Approved / checks pass / merged | 8 |
| `·` | muted | No CI | 9 |

The priority column is the order in `prSignal`: a PR shows the first glyph that applies.

Structural glyphs: `▍` selection bar, `━` active tab, `●` changed (teal) or status dot, `┌ ├ └` stack map, `◂` "this PR" in the stack map, `↳` inline comment path, `•` `◦` `▪` list bullets by depth, `▎` block quote bar, `↵` Enter in hints.

## Type

The terminal's own monospace font. Weight and color carry the hierarchy:

| Level | Style |
| --- | --- |
| PR title in detail, H1/H2 | bold `bright` (H1 also underlined) |
| Group header, section label | bold, the group's color / `muted` |
| Selected row title, active tab | bold `bright` |
| Body | `ink` |
| Secondary | `muted` |
| Hint, label, time | `subtle` |

Italic is used only for markdown emphasis. Underline is used only for links and H1.

## Layout

- **Chrome:** 4 rows (header, rule, rule, footer). `chromeTop = 2`, `chromeBottom = 2`.
- **Wide (≥ 88 columns, `wideBreakpoint`):** list on the left at 44% of the width, clamped to 44–84 columns, a `│` divider, then the detail pane. Two-line queue rows.
- **Narrow (< 88 columns):** the list on top (45% of the body height) with one-line rows, a rule, then the detail pane.
- **Under 40×12:** a single line asking for a bigger terminal.
- **Detail pane:** a 2-line tab strip, content with a 2-cell margin on both sides, and the bottom row reserved for the scroll hint.
- **Below 110 columns:** the header drops "synced/next", the footer drops `#`, `↵`, `J` and `r` hints.

## Spacing

Spacing is counted in cells.

| Step | Use |
| --- | --- |
| 1 | Between a glyph and its text |
| 2 | Detail margins; between fields on a row; between `#number`, reason, Jira and repo |
| 3 | Between tabs; between footer hints |
| 5 | Indent of a queue row's second line, so `#number` sits under the title |
| 10 | Width of field labels in the overview (`Checks`, `Review` …) |

One blank line separates list groups (dropped when space is tight) and detail sections.
