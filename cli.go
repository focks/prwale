package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if len(message) > 600 {
			message = message[:600] + "…"
		}
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, message)
	}
	return out, nil
}

type ServiceStatus struct {
	OK        bool      `json:"ok"`
	Message   string    `json:"message"`
	CheckedAt time.Time `json:"checked_at"`
}

type Connections struct {
	GitHub ServiceStatus `json:"github"`
	Jira   ServiceStatus `json:"jira"`
}

func checkConnections(ctx context.Context, r runner) Connections {
	check := func(name string, args ...string) ServiceStatus {
		short, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()
		out, err := r.Run(short, name, args...)
		if err != nil {
			return ServiceStatus{Message: compactError(err), CheckedAt: time.Now()}
		}
		return ServiceStatus{OK: true, Message: strings.TrimSpace(string(out)), CheckedAt: time.Now()}
	}
	github := check("gh", "auth", "status")
	if github.OK {
		for _, line := range strings.Split(github.Message, "\n") {
			if strings.Contains(line, "Logged in to") {
				github.Message = strings.TrimSpace(strings.TrimLeft(line, "✓- "))
				break
			}
		}
	}
	jira := check("jira", "me")
	if jira.OK {
		identity := jira.Message
		jira = check("jira", "serverinfo")
		if jira.OK {
			jira.Message = identity + " · API reachable"
		}
	}
	return Connections{GitHub: github, Jira: jira}
}

func compactError(err error) string {
	s := strings.Join(strings.Fields(err.Error()), " ")
	if len(s) > 240 {
		return s[:240] + "…"
	}
	return s
}

func saveConnections(c Connections) error {
	dir, err := dataDir()
	if err != nil {
		return err
	}
	return privateWrite(dir+string(os.PathSeparator)+"connections.json", c)
}
