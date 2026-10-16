package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Version            int `json:"version"`
	PollSeconds        int `json:"poll_seconds"`
	MaxPRs             int `json:"max_prs"`
	CommandTimeout     int `json:"command_timeout_seconds"`
	PRDetailTTLSeconds int `json:"pr_detail_ttl_seconds"`
	JiraTTLSeconds     int `json:"jira_ttl_seconds"`
	CommentsTTLSeconds int `json:"comments_ttl_seconds"`
}

func defaultConfig() Config {
	return Config{Version: 1, PollSeconds: 120, MaxPRs: 100, CommandTimeout: 25, PRDetailTTLSeconds: 600, JiraTTLSeconds: 300, CommentsTTLSeconds: 300}
}

func (c Config) validate() error {
	if c.Version != 1 {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	if c.PollSeconds < 15 || c.PollSeconds > 3600 {
		return errors.New("poll_seconds must be between 15 and 3600")
	}
	if c.MaxPRs < 1 || c.MaxPRs > 1000 {
		return errors.New("max_prs must be between 1 and 1000")
	}
	if c.CommandTimeout < 5 || c.CommandTimeout > 120 {
		return errors.New("command_timeout_seconds must be between 5 and 120")
	}
	if c.PRDetailTTLSeconds < 30 || c.PRDetailTTLSeconds > 86400 {
		return errors.New("pr_detail_ttl_seconds must be between 30 and 86400")
	}
	if c.JiraTTLSeconds < 30 || c.JiraTTLSeconds > 86400 {
		return errors.New("jira_ttl_seconds must be between 30 and 86400")
	}
	if c.CommentsTTLSeconds < 30 || c.CommentsTTLSeconds > 86400 {
		return errors.New("comments_ttl_seconds must be between 30 and 86400")
	}
	return nil
}

func (c Config) timeout() time.Duration { return time.Duration(c.CommandTimeout) * time.Second }

func dataDir() (string, error) {
	if dir := os.Getenv("PRWALE_HOME"); dir != "" {
		return filepath.Abs(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".prwale"), nil
}

func configPath() (string, error) {
	dir, err := dataDir()
	return filepath.Join(dir, "config.json"), err
}

func loadConfig() (Config, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	c := defaultConfig()
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, c.validate()
}

func initConfig() (Config, string, bool, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, "", false, err
	}
	if _, err := os.Stat(path); err == nil {
		c, err := loadConfig()
		return c, path, false, err
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, path, false, err
	}
	c := defaultConfig()
	data, _ := json.MarshalIndent(c, "", "  ")
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return Config{}, path, false, err
	}
	// O_EXCL guarantees init never overwrites an existing configuration.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, os.ErrExist) {
		c, err := loadConfig()
		return c, path, false, err
	}
	if err != nil {
		return Config{}, path, false, err
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return Config{}, path, true, writeErr
	}
	if closeErr != nil {
		return Config{}, path, true, closeErr
	}
	return c, path, true, nil
}

func privateWrite(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	// Atomic replacement keeps a previous state readable if the process exits.
	f, err := os.CreateTemp(filepath.Dir(path), ".prwale-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
