package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Stacked pull requests: a chain where each PR targets the branch of the PR
// below it, and the bottom PR targets the trunk. prwale reads them from two
// sources:
//
//  1. GitHub native stacks (public preview July 2026, GA October 2026), via
//     GET /repos/{owner}/{repo}/stacks. These carry a stack number and the
//     full ordered member list, including PRs written by teammates.
//  2. Branch chains among your own open PRs (PR B's base == PR A's head).
//     This covers Graphite, git-spice, ghstack-style and hand-made stacks,
//     and repositories where native stacks are unavailable (GHES, forks).
//
// Native data wins when both describe the same PR.

const stacksAPIVersion = "2026-03-10"

type Stack struct {
	Key     string       `json:"key"`    // stable identity: "owner/repo#stack42" or "owner/repo@root-branch"
	Number  int          `json:"number"` // native stack number, 0 when inferred
	Native  bool         `json:"native"`
	Base    string       `json:"base"` // trunk the bottom PR targets
	Entries []StackEntry `json:"entries"`
}

type StackEntry struct {
	Number int    `json:"number"`
	Head   string `json:"head"`
	State  string `json:"state"` // open, closed, merged
	Draft  bool   `json:"draft"`
	Title  string `json:"title,omitempty"`
}

// Position returns the 1-based position of number counting from the bottom.
func (s *Stack) Position(number int) int {
	if s == nil {
		return 0
	}
	for i, e := range s.Entries {
		if e.Number == number {
			return i + 1
		}
	}
	return 0
}

func (s *Stack) Label() string {
	if s == nil {
		return ""
	}
	if s.Native {
		return fmt.Sprintf("stack #%d", s.Number)
	}
	return "branch stack"
}

type repoStacks struct {
	Available bool    `json:"available"`
	Stacks    []Stack `json:"stacks"`
}

type apiStack struct {
	Number int  `json:"number"`
	Open   bool `json:"open"`
	Base   struct {
		Ref string `json:"ref"`
	} `json:"base"`
	PullRequests []struct {
		Number   int     `json:"number"`
		State    string  `json:"state"`
		Draft    bool    `json:"draft"`
		MergedAt *string `json:"merged_at"`
		Title    string  `json:"title"`
		Head     struct {
			Ref string `json:"ref"`
		} `json:"head"`
	} `json:"pull_requests"`
}

func parseStacks(repo string, data []byte) ([]Stack, error) {
	// gh api --paginate concatenates pages as separate JSON arrays.
	dec := json.NewDecoder(strings.NewReader(string(data)))
	var out []Stack
	for dec.More() {
		var page []apiStack
		if err := dec.Decode(&page); err != nil {
			return nil, fmt.Errorf("stacks JSON: %w", err)
		}
		for _, a := range page {
			if !a.Open || len(a.PullRequests) == 0 {
				continue
			}
			s := Stack{Key: fmt.Sprintf("%s#stack%d", repo, a.Number), Number: a.Number, Native: true, Base: a.Base.Ref}
			for _, p := range a.PullRequests {
				state := strings.ToLower(p.State)
				if p.MergedAt != nil && *p.MergedAt != "" {
					state = "merged"
				}
				s.Entries = append(s.Entries, StackEntry{Number: p.Number, Head: p.Head.Ref, State: state, Draft: p.Draft, Title: p.Title})
			}
			out = append(out, s)
		}
	}
	return out, nil
}

// loadNativeStacks lists open native stacks per repository, cached per repo.
// A repository without the feature (404/422, older gh) is cached as
// unavailable for the same period so it is not asked again on every sync.
func (c Collector) loadNativeStacks(ctx context.Context, repos []string, force bool) map[string][]Stack {
	out := map[string][]Stack{}
	ttl := time.Duration(c.Config.JiraTTLSeconds) * time.Second
	for _, repo := range repos {
		if c.Cache != nil && !force {
			if cached, at, ok, err := c.Cache.GetStacks(repo); err == nil && ok && time.Since(at) < ttl {
				out[repo] = cached.Stacks
				continue
			}
		}
		result := repoStacks{}
		data, err := c.fetch(ctx, "gh", "api", "-H", "X-GitHub-Api-Version: "+stacksAPIVersion, "--paginate", "repos/"+repo+"/stacks?per_page=100")
		if err == nil {
			if stacks, perr := parseStacks(repo, data); perr == nil {
				result = repoStacks{Available: true, Stacks: stacks}
			}
		}
		if c.Cache != nil {
			_ = c.Cache.PutStacks(repo, result, time.Now())
		}
		out[repo] = result.Stacks
	}
	return out
}

// inferStacks links your open PRs into chains by branch: a PR whose base is
// the head of another of your open PRs in the same repo sits on top of it.
// Chains of two or more become stacks; a fork in the chain keeps the most
// recently updated child so every stack stays a straight line, like GitHub's.
func inferStacks(prs []PR) []Stack {
	byRepoHead := map[string]int{}
	for i, pr := range prs {
		if pr.Branch != "" {
			byRepoHead[pr.Repo+"\x00"+pr.Branch] = i
		}
	}
	parent := map[int]int{}
	children := map[int][]int{}
	for i, pr := range prs {
		if pr.BaseRef == "" || pr.BaseRef == pr.Branch {
			continue
		}
		if p, ok := byRepoHead[pr.Repo+"\x00"+pr.BaseRef]; ok && p != i {
			parent[i] = p
			children[p] = append(children[p], i)
		}
	}
	var stacks []Stack
	for i, pr := range prs {
		if _, hasParent := parent[i]; hasParent || len(children[i]) == 0 {
			continue
		}
		s := Stack{Key: pr.Repo + "@" + pr.Branch, Base: pr.BaseRef}
		seen := map[int]bool{}
		for cur := i; !seen[cur]; {
			seen[cur] = true
			p := prs[cur]
			state := "open"
			s.Entries = append(s.Entries, StackEntry{Number: p.Number, Head: p.Branch, State: state, Draft: p.Draft, Title: p.Title})
			next := children[cur]
			if len(next) == 0 {
				break
			}
			sort.SliceStable(next, func(a, b int) bool { return prs[next[a]].UpdatedAt.After(prs[next[b]].UpdatedAt) })
			cur = next[0]
		}
		if len(s.Entries) >= 2 {
			stacks = append(stacks, s)
		}
	}
	return stacks
}

// attachStacks sets PR.Stack for every PR that belongs to a native or
// inferred stack, filling titles of your own PRs into the entries.
func attachStacks(prs []PR, native map[string][]Stack) {
	index := map[string]int{}
	for i, pr := range prs {
		index[pr.ID] = i
		prs[i].Stack = nil
	}
	assign := func(repo string, s Stack) {
		st := s
		for j, e := range st.Entries {
			if k, ok := index[repo+"#"+strconv.Itoa(e.Number)]; ok {
				st.Entries[j].Title = prs[k].Title
				if st.Entries[j].State == "" {
					st.Entries[j].State = "open"
				}
			}
		}
		for _, e := range st.Entries {
			if k, ok := index[repo+"#"+strconv.Itoa(e.Number)]; ok && prs[k].Stack == nil {
				copyStack := st
				prs[k].Stack = &copyStack
			}
		}
	}
	for repo, stacks := range native {
		for _, s := range stacks {
			s.Entries = append([]StackEntry(nil), s.Entries...)
			assign(repo, s)
		}
	}
	for _, s := range inferStacks(prs) {
		repo := s.Key[:strings.LastIndex(s.Key, "@")]
		covered := false
		for _, e := range s.Entries {
			if k, ok := index[repo+"#"+strconv.Itoa(e.Number)]; ok && prs[k].Stack != nil && prs[k].Stack.Native {
				covered = true
			}
		}
		if !covered {
			assign(repo, s)
		}
	}
}

// stackBlocker returns the nearest open PR below pr in its stack that needs
// attention, if any. Only PRs present in the snapshot can be judged.
func stackBlocker(pr PR, prs []PR) (PR, bool) {
	if pr.Stack == nil {
		return PR{}, false
	}
	pos := pr.Stack.Position(pr.Number)
	byNumber := map[int]PR{}
	for _, p := range prs {
		if p.Repo == pr.Repo {
			byNumber[p.Number] = p
		}
	}
	for i := pos - 2; i >= 0; i-- {
		e := pr.Stack.Entries[i]
		if e.State != "open" {
			continue
		}
		if p, ok := byNumber[e.Number]; ok && len(attentionReasons(p)) > 0 {
			return p, true
		}
	}
	return PR{}, false
}

func stackRepos(prs []PR) []string {
	seen := map[string]bool{}
	var repos []string
	for _, pr := range prs {
		if !seen[pr.Repo] {
			seen[pr.Repo] = true
			repos = append(repos, pr.Repo)
		}
	}
	sort.Strings(repos)
	return repos
}
