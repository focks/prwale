package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type PR struct {
	ID             string
	Repo           string
	Number         int
	Title          string
	URL            string
	Body           string
	Branch         string
	BaseRef        string
	HeadSHA        string
	Draft          bool
	UpdatedAt      time.Time
	Review         string
	MergeState     string
	Checks         string
	Comments       int
	SearchComments int
	ReviewCount    int
	JiraKeys       []string
	Issues         []Issue
	DetailError    string
	Changes        []string
	Stack          *Stack `json:",omitempty"`
}

type Issue struct {
	Key         string
	Summary     string
	Description string
	Status      string
	Assignee    string
	Priority    string
	Updated     string
	URL         string
	Error       string
}

type Snapshot struct {
	PRs       []PR
	FetchedAt time.Time
	Warnings  []string
	Truncated bool
}

type searchPR struct {
	Number        int       `json:"number"`
	Title         string    `json:"title"`
	URL           string    `json:"url"`
	Body          string    `json:"body"`
	IsDraft       bool      `json:"isDraft"`
	UpdatedAt     time.Time `json:"updatedAt"`
	CommentsCount int       `json:"commentsCount"`
	Repository    struct {
		NameWithOwner string `json:"nameWithOwner"`
		Name          string `json:"name"`
		Owner         struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

type detailPR struct {
	Body              *string           `json:"body"`
	HeadRefName       string            `json:"headRefName"`
	BaseRefName       string            `json:"baseRefName"`
	HeadRefOID        string            `json:"headRefOid"`
	ReviewDecision    string            `json:"reviewDecision"`
	MergeStateStatus  string            `json:"mergeStateStatus"`
	Comments          []json.RawMessage `json:"comments"`
	Reviews           []json.RawMessage `json:"reviews"`
	StatusCheckRollup []struct {
		Typename   string `json:"__typename"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		State      string `json:"state"`
	} `json:"statusCheckRollup"`
}

var jiraKeyRE = regexp.MustCompile(`[A-Z][A-Z0-9]{1,9}-[1-9][0-9]*`)
var explicitJiraLineRE = regexp.MustCompile(`(?i)\b(?:jira|ticket|issue)\s*:\s*(.*)$`)
var jiraURLRE = regexp.MustCompile(`(?i)https?://[^\s/]+\.atlassian\.net/browse/([A-Z][A-Z0-9]{1,9}-[1-9][0-9]*)`)

func keyBoundary(s string, start, end int) bool {
	word := func(b byte) bool { return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' }
	return (start == 0 || !word(s[start-1])) && (end == len(s) || !word(s[end]))
}

func keysFromPR(branch, title, body string) []string {
	seen := make(map[string]bool)
	var keys []string
	add := func(s string, normalize bool) {
		if normalize {
			s = strings.ToUpper(s)
		}
		for _, loc := range jiraKeyRE.FindAllStringIndex(s, -1) {
			if !keyBoundary(s, loc[0], loc[1]) {
				continue
			}
			key := s[loc[0]:loc[1]]
			if !seen[key] {
				seen[key] = true
				keys = append(keys, key)
			}
		}
	}
	add(branch, false)
	add(title, false)
	for _, line := range strings.Split(body, "\n") {
		for _, match := range jiraURLRE.FindAllStringSubmatch(line, -1) {
			add(match[1], true)
		}
		if match := explicitJiraLineRE.FindStringSubmatch(line); match != nil {
			add(match[1], true)
		}
	}
	return keys
}

func parseSearch(data []byte) ([]PR, error) {
	var raw []searchPR
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("GitHub search JSON: %w", err)
	}
	prs := make([]PR, 0, len(raw))
	for _, x := range raw {
		repo := x.Repository.NameWithOwner
		if repo == "" && x.Repository.Owner.Login != "" && x.Repository.Name != "" {
			repo = x.Repository.Owner.Login + "/" + x.Repository.Name
		}
		if repo == "" || x.Number < 1 || x.URL == "" {
			return nil, errors.New("GitHub search response missing repository, PR number, or URL")
		}
		prs = append(prs, PR{ID: repo + "#" + strconv.Itoa(x.Number), Repo: repo, Number: x.Number,
			Title: x.Title, URL: x.URL, Body: x.Body, Draft: x.IsDraft,
			UpdatedAt: x.UpdatedAt, Comments: x.CommentsCount, SearchComments: x.CommentsCount})
	}
	return prs, nil
}

func parseDetail(data []byte, pr *PR) error {
	var raw detailPR
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	pr.Branch = raw.HeadRefName
	pr.BaseRef = raw.BaseRefName
	if raw.Body != nil {
		pr.Body = *raw.Body
	}
	pr.HeadSHA = raw.HeadRefOID
	pr.Review = raw.ReviewDecision
	pr.MergeState = raw.MergeStateStatus
	pr.ReviewCount = len(raw.Reviews)
	if pr.Review == "" {
		pr.Review = "not reviewed"
	}
	// The search result has an aggregate comment count, which remains valid even
	// if gh's detail response has a different pagination shape.
	if len(raw.Comments) > pr.Comments {
		pr.Comments = len(raw.Comments)
	}
	passed, failed, pending := 0, 0, 0
	for _, c := range raw.StatusCheckRollup {
		state := strings.ToUpper(c.Conclusion)
		if state == "" {
			state = strings.ToUpper(c.State)
		}
		if state == "" {
			state = strings.ToUpper(c.Status)
		}
		switch state {
		case "SUCCESS", "NEUTRAL", "SKIPPED", "PASSING", "EXPECTED":
			passed++
		case "FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED":
			failed++
		default:
			pending++
		}
	}
	pr.Checks = "none"
	if failed > 0 {
		pr.Checks = fmt.Sprintf("%d failed · %d passed · %d pending", failed, passed, pending)
	} else if pending > 0 {
		pr.Checks = fmt.Sprintf("%d pending · %d passed", pending, passed)
	} else if passed > 0 {
		pr.Checks = fmt.Sprintf("%d passed", passed)
	}
	pr.JiraKeys = keysFromPR(pr.Branch, pr.Title, pr.Body)
	return nil
}

func parseIssue(data []byte, key string) (Issue, error) {
	var raw struct {
		Key    string `json:"key"`
		Self   string `json:"self"`
		Fields struct {
			Summary     string          `json:"summary"`
			Description json.RawMessage `json:"description"`
			Status      struct {
				Name string `json:"name"`
			} `json:"status"`
			Assignee struct {
				DisplayName string `json:"displayName"`
			} `json:"assignee"`
			Priority struct {
				Name string `json:"name"`
			} `json:"priority"`
			Updated string `json:"updated"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Issue{}, err
	}
	if raw.Key != key {
		return Issue{}, fmt.Errorf("Jira returned %q for %q", raw.Key, key)
	}
	url := ""
	if i := strings.Index(raw.Self, "/rest/api/"); i >= 0 {
		url = raw.Self[:i] + "/browse/" + key
	}
	return Issue{Key: key, Summary: raw.Fields.Summary, Status: raw.Fields.Status.Name,
		Description: jiraDescription(raw.Fields.Description),
		Assignee:    raw.Fields.Assignee.DisplayName, Priority: raw.Fields.Priority.Name,
		Updated: raw.Fields.Updated, URL: url}, nil
}

func parseProjectKeys(data []byte) (map[string]bool, error) {
	keys := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] == "KEY" {
			continue
		}
		key := fields[0]
		if key == strings.ToUpper(key) && !strings.ContainsAny(key, "-:") {
			keys[key] = true
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("Jira returned no accessible projects")
	}
	return keys, nil
}

type Collector struct {
	Runner runner
	Config Config
	Cache  *Cache
}

func (c Collector) fetch(ctx context.Context, name string, args ...string) ([]byte, error) {
	short, cancel := context.WithTimeout(ctx, c.Config.timeout())
	defer cancel()
	return c.Runner.Run(short, name, args...)
}

func (c Collector) searchAuthored(ctx context.Context, limit int) ([]PR, error) {
	data, err := c.fetch(ctx, "gh", "search", "prs", "--author", "@me", "--state", "open", "--limit", strconv.Itoa(limit), "--sort", "updated", "--order", "desc", "--json", "number,title,url,body,isDraft,updatedAt,repository,commentsCount")
	if err != nil {
		return nil, err
	}
	return parseSearch(data)
}

// FindNumber searches all GitHub results available to gh search, without
// fetching full details for unrelated PRs. Numbers are not unique across repos.
func (c Collector) FindNumber(ctx context.Context, number int) ([]PR, bool, error) {
	prs, err := c.searchAuthored(ctx, 1000)
	if err != nil {
		return nil, false, err
	}
	var matches []PR
	for _, pr := range prs {
		if pr.Number == number {
			matches = append(matches, pr)
		}
	}
	return matches, len(prs) >= 1000, nil
}

func (c Collector) Collect(ctx context.Context) (Snapshot, error)       { return c.collect(ctx, false) }
func (c Collector) CollectForced(ctx context.Context) (Snapshot, error) { return c.collect(ctx, true) }

func applyCachedDetail(pr *PR, cached PR) {
	pr.Body = cached.Body
	pr.Branch = cached.Branch
	pr.BaseRef = cached.BaseRef
	pr.HeadSHA = cached.HeadSHA
	pr.Review = cached.Review
	pr.MergeState = cached.MergeState
	pr.Checks = cached.Checks
	pr.ReviewCount = cached.ReviewCount
	if cached.Comments > pr.Comments {
		pr.Comments = cached.Comments
	}
	pr.JiraKeys = keysFromPR(pr.Branch, pr.Title, pr.Body)
}

func (c Collector) collect(ctx context.Context, force bool) (Snapshot, error) {
	prs, err := c.searchAuthored(ctx, c.Config.MaxPRs)
	if err != nil {
		return Snapshot{}, err
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	var warningMu sync.Mutex
	var cacheWarnings []string
	addCacheWarning := func(message string) {
		warningMu.Lock()
		cacheWarnings = append(cacheWarnings, message)
		warningMu.Unlock()
	}
	for i := range prs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			pr := &prs[i]
			var cached PR
			var cachedAt time.Time
			var cachedOK bool
			if c.Cache != nil {
				var cacheErr error
				cached, cachedAt, cachedOK, cacheErr = c.Cache.GetPR(pr.ID)
				if cacheErr != nil {
					addCacheWarning(pr.ID + " cache: " + compactError(cacheErr))
				}
			}
			fresh := cachedOK && !force && cached.DetailError == "" && cached.UpdatedAt.Equal(pr.UpdatedAt) && cached.Title == pr.Title && cached.Draft == pr.Draft && cached.SearchComments == pr.SearchComments && time.Since(cachedAt) < time.Duration(c.Config.PRDetailTTLSeconds)*time.Second
			if fresh {
				applyCachedDetail(pr, cached)
				return
			}
			data, err := c.fetch(ctx, "gh", "pr", "view", strconv.Itoa(pr.Number), "-R", pr.Repo, "--json", "body,headRefName,baseRefName,headRefOid,reviewDecision,mergeStateStatus,statusCheckRollup,comments,reviews")
			if err == nil {
				err = parseDetail(data, pr)
			}
			if err != nil {
				pr.DetailError = compactError(err)
				if cachedOK {
					applyCachedDetail(pr, cached)
				} else {
					pr.JiraKeys = keysFromPR("", pr.Title, pr.Body)
				}
			} else if c.Cache != nil {
				if err := c.Cache.PutPR(*pr, time.Now()); err != nil {
					addCacheWarning(pr.ID + " cache write: " + compactError(err))
				}
			}
		}(i)
	}
	wg.Wait()
	attachStacks(prs, c.loadNativeStacks(ctx, stackRepos(prs), force))
	projects := map[string]bool{}
	var projectsAt time.Time
	var projectsOK bool
	if c.Cache != nil {
		var cacheErr error
		projects, projectsAt, projectsOK, cacheErr = c.Cache.GetProjects()
		if cacheErr != nil {
			addCacheWarning("Jira project cache: " + compactError(cacheErr))
		}
	}
	var projectErr error
	if force || !projectsOK || time.Since(projectsAt) >= time.Hour {
		projectData, err := c.fetch(ctx, "jira", "project", "list")
		if err == nil {
			projects, err = parseProjectKeys(projectData)
		}
		if err != nil {
			projectErr = err
		} else if c.Cache != nil {
			projectsOK = true
			if err := c.Cache.PutProjects(projects, time.Now()); err != nil {
				addCacheWarning("Jira project cache write: " + compactError(err))
			}
		}
	}
	if projectsOK || projectErr == nil {
		for i := range prs {
			filtered := prs[i].JiraKeys[:0]
			for _, key := range prs[i].JiraKeys {
				prefix, _, _ := strings.Cut(key, "-")
				if projects[prefix] {
					filtered = append(filtered, key)
				}
			}
			prs[i].JiraKeys = filtered
		}
	}
	keys := map[string]bool{}
	for _, pr := range prs {
		for _, key := range pr.JiraKeys {
			keys[key] = true
		}
	}
	issues := make(map[string]Issue, len(keys))
	var mu sync.Mutex
	for key := range keys {
		if projectErr != nil && !projectsOK {
			issues[key] = Issue{Key: key, Error: "Jira projects unavailable: " + compactError(projectErr)}
			continue
		}
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var cached Issue
			var cachedAt time.Time
			var cachedOK bool
			if c.Cache != nil {
				var cacheErr error
				cached, cachedAt, cachedOK, cacheErr = c.Cache.GetIssue(key)
				if cacheErr != nil {
					addCacheWarning(key + " cache: " + compactError(cacheErr))
				}
			}
			if cachedOK && !force && time.Since(cachedAt) < time.Duration(c.Config.JiraTTLSeconds)*time.Second {
				mu.Lock()
				issues[key] = cached
				mu.Unlock()
				return
			}
			data, err := c.fetch(ctx, "jira", "issue", "view", key, "--raw")
			issue := Issue{Key: key}
			if err == nil {
				issue, err = parseIssue(data, key)
			}
			if err != nil {
				if cachedOK {
					issue = cached
				}
				issue.Error = compactError(err)
			} else if c.Cache != nil {
				if err := c.Cache.PutIssue(issue, time.Now()); err != nil {
					addCacheWarning(key + " cache write: " + compactError(err))
				}
			}
			mu.Lock()
			issues[key] = issue
			mu.Unlock()
		}(key)
	}
	wg.Wait()
	var warnings []string
	warnings = append(warnings, cacheWarnings...)
	if projectErr != nil {
		warnings = append(warnings, "Jira project list: "+compactError(projectErr))
	}
	for key, issue := range issues {
		if projectErr == nil && issue.Error != "" {
			warnings = append(warnings, key+": "+issue.Error)
		}
	}
	for i := range prs {
		for _, key := range prs[i].JiraKeys {
			issue := issues[key]
			prs[i].Issues = append(prs[i].Issues, issue)
		}
		if prs[i].DetailError != "" {
			warnings = append(warnings, prs[i].ID+": "+prs[i].DetailError)
		}
	}
	slices.Sort(warnings)
	truncated := len(prs) >= c.Config.MaxPRs
	if truncated {
		warnings = append(warnings, "GitHub search reached max_prs; increase it to see more and avoid missing changes")
	}
	return Snapshot{PRs: prs, FetchedAt: time.Now(), Warnings: warnings, Truncated: truncated}, nil
}
