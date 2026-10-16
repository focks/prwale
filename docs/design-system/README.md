# prwale design system

The visual language of the prwale terminal UI. A queue of your open PRs on the left, one PR in depth on the right, and nothing on screen that does not help decide what to do next.

This folder is the source of truth for how prwale looks. The code that implements it lives in `view.go` (layout, rows, detail pane), `markdown.go` (body rendering) and `render_cache.go` (performance). If the code and this folder disagree, fix one of them in the same change.

| File | What it covers |
| --- | --- |
| [foundations.md](foundations.md) | Color roles, glyphs, type, layout grid, spacing, breakpoints |
| [components.md](components.md) | Header, queue tabs, list groups, queue row, detail tabs, overview, stack map, comments, markdown, footer |
| [content.md](content.md) | Words: reasons, ages, empty states, messages, key hints |
| [tokens.json](tokens.json) | Machine-readable tokens; each color names its Go variable (`go`) |

Agents: the `prwale-design-system` skill (for building UI) and the `prwale-tui-review` skill (for checking it) are in `skills/`, linked from `.claude/skills` and `.codex/skills`.

## Principles

1. **Triage first.** The queue is grouped by what you must do next: NEEDS ATTENTION, CHANGED, WAITING, QUIET. Order inside a group is most recently updated first.
2. **One meaning per color.** Red is blocked on you, amber is waiting on others, green is clear, teal is new since the last sync, violet is Jira, accent is focus and navigation. A color is never decoration.
3. **One glyph per PR.** A row leads with the single most important signal (see the glyph table in foundations). Detail lives in the detail pane.
4. **Quiet by default.** Only red reasons are colored in the list. Waiting and draft text stays `muted`. Twenty waiting PRs must not shout.
5. **Chrome is four rows.** Header, rule, panes, rule, footer. Everything else is content.
6. **Every frame is exact.** Each line is exactly its pane width: never wider (tests enforce it), never ragged (selection backgrounds must reach the edge).
7. **Fast enough to scroll.** A scroll tick re-renders under 1 ms on the benchmark (`go test -bench Scroll`). Expensive work goes through `render_cache.go`.

## At a glance

```
 prwale    All 53   Attention 21   Changed 0   No Jira 4   Stacks 5        ● synced 05:23  next 01:42
─━━━━━━━━─────────────────────────────────────────┬──────────────────────────────────────────────────
  NEEDS ATTENTION                                21│ s Overview   d Description   c Comments 3   i Jira
▍ ✗  test: pin import report contract…          6h│─━━━━━━━━━━──────────────────────────────────────
▍    #7234  2 failing  DEMO-42 To Do  stack 3/3 …│  service  #7234                       updated 6h
  ⊘  upkeep: update retry policy                2d│  test: [DEMO-42] pin import report contract …
     #222  merge conflict  no jira  tools         │
                                                  │   NEEDS ATTENTION   checks failing
  WAITING                                       30│  Checks    ✗ 2 failed   ✓ 16 passed   ◐ 1 running
  ○  fix: autotune diff viewer wraps long SQL…  2d│  Review    ○ review required
─────────────────────────────────────────────────┴──────────────────────────────────────────────────
 j/k move   / search   # number   ↵ open   J jira   r sync   ? keys                 Synced 53 open PRs
```
