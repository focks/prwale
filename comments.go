package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

type Comment struct {
	Kind      string
	Author    string
	Body      string
	CreatedAt time.Time
	Path      string
	Line      int
	URL       string
}

var hiddenHTMLComment = regexp.MustCompile(`(?s)<!--.*?-->`)

func commentDisplayBody(body string) string {
	visible := strings.TrimSpace(hiddenHTMLComment.ReplaceAllString(body, ""))
	if visible == "" {
		return "(Comment contains only hidden metadata.)"
	}
	return visible
}

type ghComment struct {
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	Body           string    `json:"body"`
	CreatedAt      time.Time `json:"createdAt"`
	CreatedAtAPI   time.Time `json:"created_at"`
	SubmittedAt    time.Time `json:"submittedAt"`
	SubmittedAtAPI time.Time `json:"submitted_at"`
	State          string    `json:"state"`
	URL            string    `json:"url"`
	HTMLURL        string    `json:"html_url"`
	Path           string    `json:"path"`
	Line           int       `json:"line"`
	OriginalLine   int       `json:"original_line"`
}

func parseConversation(data []byte) ([]Comment, error) {
	var raw struct {
		Comments []ghComment `json:"comments"`
		Reviews  []ghComment `json:"reviews"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var out []Comment
	for _, item := range raw.Comments {
		out = append(out, Comment{Kind: "comment", Author: item.Author.Login, Body: item.Body, CreatedAt: item.CreatedAt, URL: item.URL})
	}
	for _, item := range raw.Reviews {
		body := strings.TrimSpace(item.Body)
		if body == "" {
			body = "Review: " + strings.ToLower(strings.ReplaceAll(item.State, "_", " "))
		}
		out = append(out, Comment{Kind: "review", Author: item.Author.Login, Body: body, CreatedAt: item.SubmittedAt, URL: item.URL})
	}
	return out, nil
}

func parseInlineComments(data []byte) ([]Comment, error) {
	pages, err := parseCommentPages(data)
	if err != nil {
		return nil, err
	}
	var out []Comment
	for _, item := range pages {
		line := item.Line
		if line == 0 {
			line = item.OriginalLine
		}
		out = append(out, Comment{Kind: "inline", Author: item.User.Login, Body: item.Body, CreatedAt: item.CreatedAtAPI, Path: item.Path, Line: line, URL: item.HTMLURL})
	}
	return out, nil
}

func parseCommentPages(data []byte) ([]ghComment, error) {
	var pages [][]ghComment
	if err := json.Unmarshal(data, &pages); err != nil {
		var single []ghComment
		if err2 := json.Unmarshal(data, &single); err2 != nil {
			return nil, err
		}
		pages = [][]ghComment{single}
	}
	var out []ghComment
	for _, page := range pages {
		out = append(out, page...)
	}
	return out, nil
}

// LoadComments is only called for the selected PR when the comments pane is
// opened. It combines conversation, reviews, and inline review threads.
func (c Collector) LoadComments(ctx context.Context, pr PR, force bool) ([]Comment, error) {
	var cached []Comment
	var cachedOK bool
	if c.Cache != nil {
		var at time.Time
		var err error
		cached, at, cachedOK, err = c.Cache.GetComments(pr.ID)
		slices.SortStableFunc(cached, func(a, b Comment) int { return b.CreatedAt.Compare(a.CreatedAt) })
		if err == nil && cachedOK && !force && time.Since(at) < time.Duration(c.Config.CommentsTTLSeconds)*time.Second {
			return cached, nil
		}
	}
	base := fmt.Sprintf("repos/%s", pr.Repo)
	request := func(endpoint string) ([]ghComment, error) {
		data, err := c.fetch(ctx, "gh", "api", endpoint+"?per_page=100", "--paginate", "--slurp")
		if err != nil {
			return nil, err
		}
		return parseCommentPages(data)
	}
	var comments []Comment
	var fetchErr error
	conversation, err := request(fmt.Sprintf("%s/issues/%d/comments", base, pr.Number))
	if err != nil {
		fetchErr = fmt.Errorf("conversation comments: %w", err)
	} else {
		for _, item := range conversation {
			comments = append(comments, Comment{Kind: "comment", Author: item.User.Login, Body: item.Body, CreatedAt: item.CreatedAtAPI, URL: item.HTMLURL})
		}
	}
	reviews, err := request(fmt.Sprintf("%s/pulls/%d/reviews", base, pr.Number))
	if err != nil {
		if fetchErr == nil {
			fetchErr = fmt.Errorf("reviews: %w", err)
		}
	} else {
		for _, item := range reviews {
			body := strings.TrimSpace(item.Body)
			if body == "" {
				body = "Review: " + strings.ToLower(strings.ReplaceAll(item.State, "_", " "))
			}
			comments = append(comments, Comment{Kind: "review", Author: item.User.Login, Body: body, CreatedAt: item.SubmittedAtAPI, URL: item.HTMLURL})
		}
	}
	inlineData, inlineErr := c.fetch(ctx, "gh", "api", fmt.Sprintf("%s/pulls/%d/comments?per_page=100", base, pr.Number), "--paginate", "--slurp")
	if inlineErr == nil {
		var inline []Comment
		inline, inlineErr = parseInlineComments(inlineData)
		if inlineErr == nil {
			comments = append(comments, inline...)
		}
	}
	if inlineErr != nil && fetchErr == nil {
		fetchErr = fmt.Errorf("inline comments: %w", inlineErr)
	}
	slices.SortStableFunc(comments, func(a, b Comment) int { return b.CreatedAt.Compare(a.CreatedAt) })
	if fetchErr != nil && cachedOK {
		return cached, fmt.Errorf("using saved comments: %w", fetchErr)
	}
	if c.Cache != nil && fetchErr == nil {
		if err := c.Cache.PutComments(pr.ID, comments, time.Now()); err != nil {
			return comments, fmt.Errorf("comments cache: %w", err)
		}
	}
	return comments, fetchErr
}
