package gog

import (
	"strings"
	"time"
)

type RunnerProvider interface {
	RunnerFor(account string) Runner
}

type RunnerFactory struct {
	Path           string
	DefaultAccount string
	Timeout        time.Duration
}

func (f *RunnerFactory) RunnerFor(account string) Runner {
	resolved := strings.TrimSpace(account)
	if resolved == "" {
		resolved = strings.TrimSpace(f.DefaultAccount)
	}
	return &GogRunner{Path: f.Path, Account: resolved, Timeout: f.Timeout}
}
