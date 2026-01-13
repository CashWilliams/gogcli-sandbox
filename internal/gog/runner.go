package gog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Runner interface {
	Run(ctx context.Context, action string, params map[string]interface{}) (any, error)
}

type GogRunner struct {
	Path    string
	Account string
	Timeout time.Duration
}

type ActionSpec struct {
	Command        []string
	Positional     []string
	ParamFlags     map[string]string
	MultiValueFlag map[string]string
}

var actionSpecs = map[string]ActionSpec{
	"gmail.search": {
		Command:    []string{"gmail", "search"},
		Positional: []string{"query"},
		ParamFlags: map[string]string{
			"max":    "--max",
			"page":   "--page",
			"oldest": "--oldest",
		},
	},
	"gmail.thread.list": {
		Command:    []string{"gmail", "search"},
		Positional: []string{"query"},
		ParamFlags: map[string]string{
			"max":    "--max",
			"page":   "--page",
			"oldest": "--oldest",
		},
	},
	"gmail.thread.get": {
		Command:    []string{"gmail", "thread", "get"},
		Positional: []string{"thread_id"},
	},
	"gmail.thread.modify": {
		Command:    []string{"gmail", "thread", "modify"},
		Positional: []string{"thread_id"},
		ParamFlags: map[string]string{
			"add":    "--add",
			"remove": "--remove",
		},
	},
	"gmail.get": {
		Command:    []string{"gmail", "get"},
		Positional: []string{"message_id"},
		ParamFlags: map[string]string{
			"format":  "--format",
			"headers": "--headers",
		},
	},
	"gmail.send": {
		Command: []string{"gmail", "send"},
		ParamFlags: map[string]string{
			"to":                  "--to",
			"cc":                  "--cc",
			"bcc":                 "--bcc",
			"subject":             "--subject",
			"body":                "--body",
			"body_html":           "--body-html",
			"reply_to_message_id": "--reply-to-message-id",
			"thread_id":           "--thread-id",
			"reply_all":           "--reply-all",
			"reply_to":            "--reply-to",
			"from":                "--from",
			"track":               "--track",
			"track_split":         "--track-split",
		},
		MultiValueFlag: map[string]string{
			"attach": "--attach",
		},
	},
	"gmail.drafts.create": {
		Command: []string{"gmail", "drafts", "create"},
		ParamFlags: map[string]string{
			"to":                  "--to",
			"cc":                  "--cc",
			"bcc":                 "--bcc",
			"subject":             "--subject",
			"body":                "--body",
			"body_html":           "--body-html",
			"reply_to_message_id": "--reply-to-message-id",
			"reply_to":            "--reply-to",
			"from":                "--from",
		},
		MultiValueFlag: map[string]string{
			"attach": "--attach",
		},
	},
	"gmail.labels.list": {
		Command: []string{"gmail", "labels", "list"},
	},
	"gmail.labels.get": {
		Command:    []string{"gmail", "labels", "get"},
		Positional: []string{"label"},
	},
	"gmail.labels.modify": {
		Command:    []string{"gmail", "labels", "modify"},
		Positional: []string{"thread_ids"},
		ParamFlags: map[string]string{
			"add":    "--add",
			"remove": "--remove",
		},
	},
	"calendar.list": {
		Command: []string{"calendar", "calendars"},
		ParamFlags: map[string]string{
			"max":  "--max",
			"page": "--page",
		},
	},
	"calendar.events": {
		Command:    []string{"calendar", "events"},
		Positional: []string{"calendar_id"},
		ParamFlags: map[string]string{
			"time_min": "--from",
			"time_max": "--to",
			"max":      "--max",
			"page":     "--page",
			"query":    "--query",
		},
	},
	"calendar.freebusy": {
		Command:    []string{"calendar", "freebusy"},
		Positional: []string{"calendar_ids"},
		ParamFlags: map[string]string{
			"time_min": "--from",
			"time_max": "--to",
		},
	},
}

func (g *GogRunner) Run(ctx context.Context, action string, params map[string]interface{}) (any, error) {
	spec, ok := actionSpecs[action]
	if !ok {
		return nil, fmt.Errorf("no command mapping for action: %s", action)
	}

	if params == nil {
		params = map[string]interface{}{}
	}

	args, err := buildArgs(spec, params)
	if err != nil {
		return nil, err
	}

	baseArgs := []string{}
	if g.Account != "" {
		baseArgs = append(baseArgs, "--account", g.Account)
	}
	baseArgs = append(baseArgs, "--json", "--no-input")
	baseArgs = append(baseArgs, spec.Command...)
	baseArgs = append(baseArgs, args...)

	ctx, cancel := context.WithTimeout(ctx, g.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, g.Path, baseArgs...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gog failed: %w: %s", err, truncate(stderr.String(), 256))
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("gog timed out: %w", ctx.Err())
	}

	var data any
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		return nil, fmt.Errorf("invalid gog json: %w", err)
	}
	return data, nil
}

func buildArgs(spec ActionSpec, params map[string]interface{}) ([]string, error) {
	args := []string{}
	seen := map[string]struct{}{}

	for _, key := range spec.Positional {
		val, ok := params[key]
		if !ok {
			return nil, fmt.Errorf("missing required param: %s", key)
		}
		argVals, err := normalizePositional(key, val)
		if err != nil {
			return nil, fmt.Errorf("param %s: %w", key, err)
		}
		args = append(args, argVals...)
		seen[key] = struct{}{}
	}

	for key, flag := range spec.ParamFlags {
		if val, ok := params[key]; ok {
			if b, ok := val.(bool); ok {
				if b {
					args = append(args, flag)
				}
				seen[key] = struct{}{}
				continue
			}
			argVals, err := normalizeValue(val)
			if err != nil {
				return nil, fmt.Errorf("param %s: %w", key, err)
			}
			if len(argVals) == 0 {
				continue
			}
			args = append(args, flag)
			args = append(args, argVals[0])
			seen[key] = struct{}{}
		}
	}

	for key, flag := range spec.MultiValueFlag {
		if val, ok := params[key]; ok {
			argVals, err := normalizeValue(val)
			if err != nil {
				return nil, fmt.Errorf("param %s: %w", key, err)
			}
			for _, v := range argVals {
				args = append(args, flag, v)
			}
			seen[key] = struct{}{}
		}
	}

	unknown := []string{}
	for key := range params {
		if _, ok := seen[key]; ok {
			continue
		}
		if _, ok := spec.ParamFlags[key]; ok {
			continue
		}
		if _, ok := spec.MultiValueFlag[key]; ok {
			continue
		}
		unknown = append(unknown, key)
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, errors.New("unknown params: " + strings.Join(unknown, ", "))
	}

	return args, nil
}

func normalizePositional(key string, val interface{}) ([]string, error) {
	vals, err := normalizeValue(val)
	if err != nil {
		return nil, err
	}
	if len(vals) == 0 {
		return nil, errors.New("empty value")
	}
	switch key {
	case "calendar_ids":
		return []string{strings.Join(vals, ",")}, nil
	default:
		return vals, nil
	}
}

func normalizeValue(val interface{}) ([]string, error) {
	switch v := val.(type) {
	case string:
		return []string{v}, nil
	case float64:
		return []string{strconv.FormatInt(int64(v), 10)}, nil
	case int:
		return []string{strconv.Itoa(v)}, nil
	case bool:
		if v {
			return []string{"true"}, nil
		}
		return []string{"false"}, nil
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			vals, err := normalizeValue(item)
			if err != nil {
				return nil, err
			}
			out = append(out, vals...)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported value type %T", val)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
