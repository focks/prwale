package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

type issueState struct {
	Status  string `json:"status"`
	Updated string `json:"updated"`
}

type prState struct {
	Title       string                `json:"title"`
	Draft       bool                  `json:"draft"`
	HeadSHA     string                `json:"head_sha"`
	Review      string                `json:"review"`
	MergeState  string                `json:"merge_state"`
	ReviewCount int                   `json:"review_count"`
	Checks      string                `json:"checks"`
	Comments    int                   `json:"comments"`
	Issues      map[string]issueState `json:"issues"`
	StackKey    string                `json:"stack_key,omitempty"`
	StackPos    int                   `json:"stack_pos,omitempty"`
	StackSize   int                   `json:"stack_size,omitempty"`
}

type Event struct {
	At      time.Time `json:"at"`
	PRID    string    `json:"pr_id"`
	Message string    `json:"message"`
}

type WatchState struct {
	Version      int                `json:"version"`
	Initialized  bool               `json:"initialized"`
	PRs          map[string]prState `json:"prs"`
	RecentEvents []Event            `json:"recent_events"`
}

func statePath() (string, error) {
	dir, err := dataDir()
	return filepath.Join(dir, "state.json"), err
}

func loadState() (WatchState, error) {
	path, err := statePath()
	if err != nil {
		return WatchState{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return WatchState{Version: 1, PRs: map[string]prState{}}, nil
	}
	if err != nil {
		return WatchState{}, err
	}
	var state WatchState
	if err := json.Unmarshal(data, &state); err != nil {
		return WatchState{}, fmt.Errorf("%s: %w", path, err)
	}
	if state.Version != 1 {
		return WatchState{}, fmt.Errorf("unsupported state version %d", state.Version)
	}
	if state.PRs == nil {
		state.PRs = map[string]prState{}
	}
	return state, nil
}

func saveState(state WatchState) error {
	path, err := statePath()
	if err != nil {
		return err
	}
	return privateWrite(path, state)
}

func compareSnapshot(s *Snapshot, old WatchState) WatchState {
	next := WatchState{Version: 1, Initialized: true, PRs: make(map[string]prState, len(s.PRs)), RecentEvents: slices.Clone(old.RecentEvents)}
	add := func(id, message string) {
		next.RecentEvents = append([]Event{{At: s.FetchedAt, PRID: id, Message: message}}, next.RecentEvents...)
	}
	for i := range s.PRs {
		pr := &s.PRs[i]
		prev, existed := old.PRs[pr.ID]
		current := prState{Title: pr.Title, Draft: pr.Draft, HeadSHA: pr.HeadSHA, Review: pr.Review, MergeState: pr.MergeState,
			ReviewCount: pr.ReviewCount, Checks: pr.Checks, Comments: pr.Comments, Issues: map[string]issueState{}}
		if pr.Stack != nil {
			current.StackKey, current.StackPos, current.StackSize = pr.Stack.Key, pr.Stack.Position(pr.Number), len(pr.Stack.Entries)
		}
		for _, issue := range pr.Issues {
			if issue.Error != "" {
				if previous, ok := prev.Issues[issue.Key]; ok {
					current.Issues[issue.Key] = previous
				}
				continue
			}
			current.Issues[issue.Key] = issueState{Status: issue.Status, Updated: issue.Updated}
		}
		if pr.DetailError != "" && existed {
			current.Review, current.Checks = prev.Review, prev.Checks
			current.HeadSHA, current.ReviewCount = prev.HeadSHA, prev.ReviewCount
			current.MergeState = prev.MergeState
		}
		if old.Initialized {
			if !existed {
				pr.Changes = append(pr.Changes, "new open PR")
			} else {
				if prev.Title != current.Title {
					pr.Changes = append(pr.Changes, "title changed")
				}
				if prev.Draft != current.Draft {
					pr.Changes = append(pr.Changes, "draft state changed")
				}
				if prev.HeadSHA != "" && current.HeadSHA != "" && prev.HeadSHA != current.HeadSHA {
					pr.Changes = append(pr.Changes, "new commit pushed")
				}
				if prev.Review != current.Review {
					pr.Changes = append(pr.Changes, "review: "+current.Review)
				}
				if prev.MergeState != "" && current.MergeState != "" && prev.MergeState != current.MergeState {
					pr.Changes = append(pr.Changes, "merge state: "+current.MergeState)
				}
				if prev.ReviewCount < current.ReviewCount {
					pr.Changes = append(pr.Changes, fmt.Sprintf("%d new review(s)", current.ReviewCount-prev.ReviewCount))
				}
				if prev.Checks != current.Checks {
					pr.Changes = append(pr.Changes, "checks: "+current.Checks)
				}
				switch {
				case prev.StackKey == "" && current.StackKey != "":
					pr.Changes = append(pr.Changes, fmt.Sprintf("joined a stack (%d/%d)", current.StackPos, current.StackSize))
				case prev.StackKey != "" && current.StackKey == "":
					pr.Changes = append(pr.Changes, "left its stack")
				case prev.StackKey != "" && (prev.StackPos != current.StackPos || prev.StackSize != current.StackSize):
					pr.Changes = append(pr.Changes, fmt.Sprintf("stack now %d/%d", current.StackPos, current.StackSize))
				}
				if prev.Comments < current.Comments {
					pr.Changes = append(pr.Changes, fmt.Sprintf("%d new comment(s)", current.Comments-prev.Comments))
				}
				for key, issue := range current.Issues {
					before, ok := prev.Issues[key]
					if !ok {
						pr.Changes = append(pr.Changes, key+" linked")
					} else if before.Status != issue.Status {
						pr.Changes = append(pr.Changes, key+": "+issue.Status)
					} else if before.Updated != issue.Updated {
						pr.Changes = append(pr.Changes, key+" updated")
					}
				}
			}
			for _, change := range pr.Changes {
				add(pr.ID, change)
			}
		}
		next.PRs[pr.ID] = current
	}
	if old.Initialized && !s.Truncated {
		for id := range old.PRs {
			if _, ok := next.PRs[id]; !ok {
				add(id, "no longer open")
			}
		}
	} else if s.Truncated {
		for id, previous := range old.PRs {
			if _, ok := next.PRs[id]; !ok {
				next.PRs[id] = previous
			}
		}
	}
	if len(next.RecentEvents) > 100 {
		next.RecentEvents = next.RecentEvents[:100]
	}
	return next
}
