# Content

## Voice

Short, lowercase, and factual. prwale reports state; it does not cheer or apologize.

## Reasons (queue row, line two)

Lowercase phrases with a number where one exists:

| Reason | When |
| --- | --- |
| `merge conflict` | mergeStateStatus is DIRTY |
| `2 failing` | checks failed |
| `changes requested` | review decision CHANGES_REQUESTED |
| `detail unavailable` | GitHub detail fetch failed |
| `draft` | draft PR |
| `3 running` | checks pending |
| `awaiting review` | review required |
| `approved` | approved |
| `checks pass` | all checks passed, no review signal |
| `no CI` | nothing reported |

## Time

- Ages: `now`, `12m`, `6h`, `3d`, then `Oct 07`.
- Timestamps in the detail pane: `Jan 02 15:04`, local time.
- Countdown: `mm:ss`, or `h:mm:ss` from one hour up.

## Labels

- Group headers and section labels are UPPERCASE: `NEEDS ATTENTION`, `JIRA`, `STACK #42 · 3 PRs`.
- Badges are UPPERCASE: `NEEDS ATTENTION`, `WAITING`, `CLEAR`.
- Tabs and field labels are Title case: `Overview`, `Checks`.

## Empty states

Say what is missing in a full sentence, and say how to fix it when there is a fix:

- `No Jira key in branch, title or body.`
- `No Jira issue linked. Add a key to the branch, title or body.`
- `No comments or reviews yet.`
- `Nothing matches /query`
- `No changes recorded yet. Changes appear after the second sync.`

## Messages (footer, right)

Past tense for done (`Synced 53 open PRs`, `Opened in browser`), with `Refresh failed: …` / `Open failed: …` / `Could not …` for failures, which render red. Keep them under about 60 characters; errors pass through `compactError`.

## Key hints

`key what` pairs, three spaces apart: `j/k move   / search   ? keys`. Use `↵` for Enter. Hint lines inside the detail pane use the same pattern: `d full description   c comments   i jira`.
