# Contributing to prwale

Thanks for helping improve prwale. Open an issue for substantial behavior changes so the expected user flow is clear before implementation. Small fixes can go directly to a pull request.

## Set up

Use Go 1.26.3 or newer. Install and authenticate the `gh` and `jira` CLIs for manual runs. Tests use fixtures and do not need live accounts.

```sh
go test ./...
go vet ./...
go build -o prwale .
```

Set `PRWALE_HOME` to a disposable directory for manual testing so private data stays out of the repository. The app keeps PR and Jira content in that directory; never attach real cache files, credentials, or private PR screenshots to issues.

## Make a change

Keep Go code formatted with `gofmt` and add focused `*_test.go` coverage for behavior changes. If you change terminal rendering, read `docs/design-system/` and use the preview instructions in `skills/prwale-tui-review/SKILL.md`. Include a screenshot using synthetic data for visual changes.

Before opening a pull request, run `gofmt -l .`, `go vet ./...`, and `go test ./...`. Describe the behavior change, link a relevant issue, and include the commands and results. Use a short imperative commit subject, such as `Fix stack ordering for merged layers`.
