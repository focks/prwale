# Repository Guidelines

## Project Structure & Module Organization

`prwale` is a single Go `main` package at the repository root. `main.go` and `cli.go` handle commands and external CLI calls; `collect.go`, `stack.go`, and `state.go` assemble PR, Jira, and stack data. `tui.go`, `view.go`, and the Markdown files render the terminal interface. `cache.go` stores local state in SQLite. Tests live beside the code in `*_test.go` files. UI specifications live in `docs/design-system/`; reusable agent skills live in `skills/`.

## Build, Test, and Development Commands

- `go build -o prwale .` builds the executable (requires Go 1.26.3 or newer).
- `go test ./...` runs the Go unit tests.
- `go test -bench . ./...` runs the rendering benchmarks when changing display performance.
- `./prwale init` creates initial settings; `./prwale doctor` checks the required `gh` and `jira` CLIs; `./prwale once` performs one sync; `./prwale` starts the TUI.

For local experiments, set `PRWALE_HOME` to a temporary directory so test state stays separate from `~/.prwale`.

## Coding Style & Naming Conventions

Use standard Go formatting (`gofmt -w <changed-files>`) and tabs as produced by `gofmt`. Keep related behavior in the existing root-level modules. Use descriptive mixed-case Go names, `TestFeatureBehavior` for tests, and `BenchmarkFeature` for benchmarks. Run `go vet ./...` when changing command execution, parsing, or persistence.

## Testing Guidelines

Add focused tests beside the code for changes to collection, caching, stack inference, rendering, or TUI navigation. Existing tests use Go's `testing` package and fake CLI responses; keep new tests independent of live GitHub or Jira accounts. Run `go test ./...` before proposing a change. No coverage threshold is documented.

## Commit & Pull Request Guidelines

Use a short imperative commit subject that names the behavior changed. In PRs, describe the user-visible effect, link the relevant issue when one exists, list test commands and results, and include a terminal screenshot for visual TUI changes. See `CONTRIBUTING.md` for the submission checklist.

## Security & Configuration

Do not commit credentials or files from `~/.prwale`. Authentication remains in the `gh` and `jira` CLI configurations. Preserve private permissions and existing settings when modifying initialization or cache code.
