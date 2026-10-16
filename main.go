package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const usage = `prwale — open PR and Jira radar

Usage:
  prwale init      Create ~/.prwale/config.json and check CLI connections
  prwale           Watch open PRs in the interactive TUI
  prwale watch     Same as above
  prwale doctor    Recheck gh and jira connection status
  prwale once      Refresh once and print a compact summary
  prwale find N    Find your open PRs with exact number N across repositories
  prwale help      Show this help

The app uses your existing gh and jira CLI authentication. It never stores tokens.
Set PRWALE_HOME to use a different private data directory.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "prwale:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "watch"
	if len(args) > 0 {
		command = args[0]
	}
	if len(args) > 1 && command != "find" {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(args[1:], " "))
	}
	switch command {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	case "init":
		_, path, created, err := initConfig()
		if err != nil {
			return err
		}
		if created {
			fmt.Println("Created", path)
		} else {
			fmt.Println("Using existing", path)
		}
		cache, err := openCache()
		if err != nil {
			return err
		}
		if err := cache.Close(); err != nil {
			return err
		}
		return doctor()
	case "doctor":
		return doctor()
	case "watch", "once", "find":
		config, err := loadConfig()
		if err != nil {
			return fmt.Errorf("load configuration (run `prwale init` first): %w", err)
		}
		collector := Collector{Runner: execRunner{}, Config: config}
		if command == "find" {
			if len(args) != 2 {
				return fmt.Errorf("usage: prwale find PR_NUMBER")
			}
			number, err := strconv.Atoi(args[1])
			if err != nil || number < 1 {
				return fmt.Errorf("PR_NUMBER must be a positive integer")
			}
			matches, truncated, err := collector.FindNumber(context.Background(), number)
			if err != nil {
				return err
			}
			if len(matches) == 0 {
				fmt.Printf("No authored open PR #%d found.\n", number)
			}
			for _, pr := range matches {
				fmt.Printf("%s  %s\n%s\n", pr.ID, pr.Title, pr.URL)
			}
			if truncated {
				fmt.Fprintln(os.Stderr, "Search reached GitHub's 1000-result limit; some PRs may be absent.")
			}
			return nil
		}
		cache, err := openCache()
		if err != nil {
			return err
		}
		defer cache.Close()
		collector.Cache = cache
		state, err := loadState()
		if err != nil {
			return err
		}
		if command == "once" {
			snapshot, err := collector.Collect(context.Background())
			if err != nil {
				return err
			}
			next := compareSnapshot(&snapshot, state)
			if err := saveState(next); err != nil {
				return err
			}
			if err := cache.SaveSnapshot(snapshot); err != nil {
				return err
			}
			fmt.Printf("%d open PRs · %d warning(s)\n", len(snapshot.PRs), len(snapshot.Warnings))
			for _, pr := range snapshot.PRs {
				fmt.Printf("%s  %s  [%s]", pr.ID, pr.Title, strings.Join(pr.JiraKeys, ", "))
				if len(pr.Changes) > 0 {
					fmt.Printf("  NEW: %s", strings.Join(pr.Changes, "; "))
				}
				fmt.Println()
			}
			for _, warning := range snapshot.Warnings {
				fmt.Fprintln(os.Stderr, "warning:", warning)
			}
			return nil
		}
		initial, _, err := cache.LoadSnapshot()
		if err != nil {
			return err
		}
		_, err = tea.NewProgram(newModel(collector, state, initial)).Run()
		return err
	default:
		return fmt.Errorf("unknown command %q; run `prwale help`", command)
	}
}

func doctor() error {
	status := checkConnections(context.Background(), execRunner{})
	if err := saveConnections(status); err != nil {
		return err
	}
	printStatus := func(name string, s ServiceStatus) {
		if s.OK {
			fmt.Printf("✓ %s: %s\n", name, s.Message)
		} else {
			fmt.Printf("✗ %s: %s\n", name, s.Message)
		}
	}
	printStatus("GitHub (gh)", status.GitHub)
	printStatus("Jira (jira)", status.Jira)
	if !status.GitHub.OK {
		fmt.Println("GitHub action: run `gh auth login` to reconnect.")
	}
	if !status.Jira.OK {
		fmt.Println("Jira action: check `jira me` and your Jira CLI configuration.")
	}
	if !status.GitHub.OK || !status.Jira.OK {
		return fmt.Errorf("one or more CLI connections need attention")
	}
	return nil
}
