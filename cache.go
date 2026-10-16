package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Cache stores the most recent screen and per-resource refresh timestamps.
// The existing state.json remains the activity ledger; no migration or reset
// of a user's previous state is needed.
type Cache struct{ db *sql.DB }

func cachePath() (string, error) {
	dir, err := dataDir()
	return filepath.Join(dir, "cache.db"), err
}

func openCache() (*Cache, error) {
	path, err := cachePath()
	if err != nil {
		return nil, err
	}
	return openCacheAt(path)
}

func openCacheAt(path string) (*Cache, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	// Create the file privately before SQLite opens it. Existing files are
	// opened in place; a corrupt database is never removed automatically.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		err = f.Close()
	} else if errors.Is(err, os.ErrExist) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	statements := []string{
		`PRAGMA busy_timeout = 5000`,
		`CREATE TABLE IF NOT EXISTS cached_prs (id TEXT PRIMARY KEY, payload BLOB NOT NULL, fetched_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS cached_issues (issue_key TEXT PRIMARY KEY, payload BLOB NOT NULL, fetched_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS cached_comments (pr_id TEXT PRIMARY KEY, payload BLOB NOT NULL, fetched_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS cached_projects (id INTEGER PRIMARY KEY CHECK(id=1), payload BLOB NOT NULL, fetched_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS cached_stacks (repo TEXT PRIMARY KEY, payload BLOB NOT NULL, fetched_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS last_snapshot (id INTEGER PRIMARY KEY CHECK(id=1), payload BLOB NOT NULL)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("SQLite cache %s: %w", path, err)
		}
	}
	return &Cache{db: db}, nil
}

func (c *Cache) Close() error { return c.db.Close() }

func (c *Cache) getResource(table, keyColumn, key string, target any) (time.Time, bool, error) {
	var payload []byte
	var epoch int64
	err := c.db.QueryRow(`SELECT payload, fetched_at FROM `+table+` WHERE `+keyColumn+`=?`, key).Scan(&payload, &epoch)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return time.Time{}, false, err
	}
	return time.Unix(epoch, 0), true, nil
}

func (c *Cache) putResource(table, keyColumn, key string, value any, at time.Time) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = c.db.Exec(`INSERT INTO `+table+` (`+keyColumn+`,payload,fetched_at) VALUES (?,?,?) ON CONFLICT(`+keyColumn+`) DO UPDATE SET payload=excluded.payload,fetched_at=excluded.fetched_at`, key, payload, at.Unix())
	return err
}

func (c *Cache) GetPR(id string) (PR, time.Time, bool, error) {
	var pr PR
	at, ok, err := c.getResource("cached_prs", "id", id, &pr)
	return pr, at, ok, err
}

func (c *Cache) PutPR(pr PR, at time.Time) error {
	pr.Changes = nil
	pr.Issues = nil
	pr.DetailError = ""
	return c.putResource("cached_prs", "id", pr.ID, pr, at)
}

func (c *Cache) GetIssue(key string) (Issue, time.Time, bool, error) {
	var issue Issue
	at, ok, err := c.getResource("cached_issues", "issue_key", key, &issue)
	return issue, at, ok, err
}

func (c *Cache) PutIssue(issue Issue, at time.Time) error {
	issue.Error = ""
	return c.putResource("cached_issues", "issue_key", issue.Key, issue, at)
}

func (c *Cache) GetComments(id string) ([]Comment, time.Time, bool, error) {
	var comments []Comment
	at, ok, err := c.getResource("cached_comments", "pr_id", id, &comments)
	return comments, at, ok, err
}

func (c *Cache) PutComments(id string, comments []Comment, at time.Time) error {
	return c.putResource("cached_comments", "pr_id", id, comments, at)
}

func (c *Cache) GetProjects() (map[string]bool, time.Time, bool, error) {
	var projects map[string]bool
	at, ok, err := c.getResource("cached_projects", "id", "1", &projects)
	return projects, at, ok, err
}

func (c *Cache) PutProjects(projects map[string]bool, at time.Time) error {
	return c.putResource("cached_projects", "id", "1", projects, at)
}

func (c *Cache) LoadSnapshot() (Snapshot, bool, error) {
	var payload []byte
	err := c.db.QueryRow(`SELECT payload FROM last_snapshot WHERE id=1`).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, err
	}
	var snapshot Snapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return Snapshot{}, false, fmt.Errorf("cached snapshot: %w", err)
	}
	return snapshot, true, nil
}

func (c *Cache) SaveSnapshot(snapshot Snapshot) error {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	_, err = c.db.Exec(`INSERT INTO last_snapshot (id,payload) VALUES (1,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload`, payload)
	return err
}

func (c *Cache) GetStacks(repo string) (repoStacks, time.Time, bool, error) {
	var stacks repoStacks
	at, ok, err := c.getResource("cached_stacks", "repo", repo, &stacks)
	return stacks, at, ok, err
}

func (c *Cache) PutStacks(repo string, stacks repoStacks, at time.Time) error {
	return c.putResource("cached_stacks", "repo", repo, stacks, at)
}
