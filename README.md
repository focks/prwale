# prwale

`prwale` watches pull requests authored by your GitHub account and the Jira issues linked from their branch, title, or body. Its cards put PRs needing attention first and show Jira and CI context. The detail pane provides the complete PR description, complete Jira description, and a comment stream with conversation comments, reviews, and inline review comments.

## Requirements

- Go 1.26.3+
- `gh` installed and authenticated (`gh auth status`)
- `jira` CLI installed and configured (`jira me`), using [ankitpokhrel/jira-cli](https://github.com/ankitpokhrel/jira-cli)

Install from source after cloning:

```sh
go build -o prwale .
./prwale init
./prwale
./prwale find 1234
```

Once the source is published, `go install github.com/focks/prwale@latest` also installs the CLI. The executable needs `gh` and `jira` available on `PATH`.

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for the local checks and [SECURITY.md](SECURITY.md) for private vulnerability reports. The project is licensed under [MIT](LICENSE).

`init` creates `~/.prwale/config.json` once and records CLI connection status in `~/.prwale/connections.json`. Existing config is preserved. `prwale doctor` rechecks both CLIs, including Jira API reachability. `prwale once` runs a single sync for scripts or diagnosis. All state, including private PR titles, is stored under `~/.prwale` with private file permissions. Credentials stay in the existing CLI configurations; prwale does not copy them.

Configuration defaults:

```json
{
  "version": 1,
  "poll_seconds": 120,
  "max_prs": 100,
  "command_timeout_seconds": 25,
  "pr_detail_ttl_seconds": 600,
  "jira_ttl_seconds": 300,
  "comments_ttl_seconds": 300
}
```

Edit `~/.prwale/config.json` to change them. Existing configuration files gain defaults for the new cache settings automatically. `PRWALE_HOME` overrides the data directory, useful for isolated runs. The watcher searches PRs authored by your authenticated GitHub user across repositories. It syncs every `poll_seconds` while the TUI is open and shows the next-sync countdown. The next timer starts after each sync finishes, including failed syncs; `r` syncs immediately. Each refresh checks the current authored PR list; unchanged PR details and Jira issues are reused from `~/.prwale/cache.db` until their cache periods expire. The last screen loads immediately from SQLite when the TUI starts. Comments are fetched only when you open their pane, then cached briefly. The app uses `jira issue view KEY --raw` for Jira, including the full issue description. It handles both plain text and Jira Cloud rich text.

On the first sync, the current PRs form a baseline; subsequent syncs record new PRs, PRs that are no longer open, check and review changes, new comments, and Jira updates. Failed detail lookups show warnings while the rest of the list remains usable. A failed GitHub search retains the last successful screen and state.

## Design system

How prwale looks is specified in [docs/design-system](docs/design-system/README.md): color roles, glyphs, layout, components and copy, with tokens in `tokens.json`. Two agent skills live in `skills/` and are linked from `.claude/skills` and `.codex/skills`:

- `prwale-design-system`: how to build or change UI that follows the docs.
- `prwale-tui-review`: render preview frames from fixtures (`skills/prwale-tui-review/scripts/preview.sh`), check contrast (`scripts/contrast.py`), and review against a checklist.

## Stacked PRs

prwale understands stacked pull requests: chains where each PR targets the branch of the PR below it and the bottom PR targets the trunk.

- **Native GitHub stacks** (public preview July 2026, generally available October 2026) are read from `GET /repos/{owner}/{repo}/stacks` once per repository and cached for `jira_ttl_seconds`. They include teammates' PRs and already-merged layers. Repositories without the feature are remembered as unavailable for the same period.
- **Branch stacks** are inferred when another of your open PRs targets a PR's head branch. This covers Graphite, git-spice, `gh stack` branches before the stack exists on GitHub, and hand-made stacks. Native data wins when both describe a PR.

Queue `5` lists each stack top-down under its own header. Every stacked row shows `stack n/m`, in red when a PR below it needs attention. The overview draws the stack map with each layer's state and the trunk, and flags a PR that is blocked by one below it. `{` and `}` move through the stack.

## Markdown

PR descriptions, comments and Jira descriptions render GitHub-flavoured markdown: headings, bold, italic, strikethrough, inline code, links (clickable in terminals with OSC 8 hyperlinks), nested and task lists, quotes and `> [!NOTE]` callouts, fenced code, tables and rules. HTML comments are hidden. Jira rich text is converted to markdown first.

## Keys

| Key | Action |
| --- | --- |
| `j` / `k`, arrows, mouse wheel over queue | Move through the current search/filter results |
| `g` / `G` | First / last |
| `Ctrl+d` / `Ctrl+u`, Page Down / Up | Page down / up through results |
| `1`–`5`, `Tab` | All, needs attention, changed, no Jira, and stacks queues |
| `/` | Search PR titles, bodies, Jira descriptions, and loaded comments as you type |
| `#` | Search loaded PRs by number; Enter switches to an exact match |
| `s` | PR overview |
| `d` | Full PR description |
| `c` | PR comments and reviews, including inline comments |
| `n` / `p` | Next / previous comment in the comments pane |
| `i` / `I` | Full Jira description / next linked Jira issue |
| `{` / `}` | Move down / up the selected PR's stack |
| `[` / `]`, `Ctrl+b` / `Ctrl+f` | Scroll the detail pane by a line / half page |
| `e` | Activity across your PRs |
| `Enter`, `o` | Open selected PR in browser |
| `J` (`O` alias) | Open linked Jira in browser; when viewing Jira detail, open the displayed issue |
| `r` | Refresh PR and Jira details now; periodic sync also runs automatically |
| `Esc` | Clear search and filter |
| `?` | Help |
| `q`, `Ctrl+c` | Quit |

GitHub search is capped by `max_prs`, so a larger number may be needed if you author many open PRs. Jira issue matching recognizes uppercase keys such as `AI-1234` in the branch or title, plus explicit `Jira: AI-1234`, `Ticket: AI-1234`, or Atlassian browse links in the body. It uses `jira project list` to keep only projects your Jira account can access. Other text in long PR bodies is ignored when matching Jira keys; the complete body remains available in the description pane.

`#` filters the PRs already loaded in the TUI. `prwale find NUMBER` searches up to 1,000 authored open PRs across repositories without fetching details for unrelated PRs; it prints every exact number match because PR numbers can repeat in different repositories.
