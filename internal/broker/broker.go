package broker

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"gogcli-sandbox/internal/gog"
	"gogcli-sandbox/internal/policy"
	"gogcli-sandbox/internal/redact"
	"gogcli-sandbox/internal/types"
)

type Broker struct {
	Policy    *policy.Policy
	Runner    gog.Runner
	Logger    Logger
	Verbose   bool
	labelOnce sync.Once
	labelErr  error
}

func (b *Broker) Handle(ctx context.Context, req *types.Request) *types.Response {
	start := time.Now()
	fields := map[string]any{}
	if req != nil {
		fields["id"] = req.ID
		fields["action"] = req.Action
	}

	if req == nil {
		b.logError("request_nil", fields, start)
		return &types.Response{Ok: false, Error: types.NewError("bad_request", "request is required", "")}
	}
	if b.Verbose {
		fieldsVerbose := cloneFields(fields)
		fieldsVerbose["param_keys"] = paramKeys(req.Params)
		if b.Logger != nil {
			b.Logger.Info("request_received", fieldsVerbose)
		}
	}
	if req.ID == "" {
		b.logError("missing_id", fields, start)
		return &types.Response{Ok: false, Error: types.NewError("bad_request", "id is required", "")}
	}
	if req.Action == "" {
		b.logError("missing_action", fields, start)
		return &types.Response{ID: req.ID, Ok: false, Error: types.NewError("bad_request", "action is required", "")}
	}
	if !b.Policy.IsActionAllowed(req.Action) {
		b.logDenied("action_denied", fields, start)
		return &types.Response{ID: req.ID, Ok: false, Error: types.NewError("forbidden", "action not allowed", "")}
	}
	if req.Action == "gmail.search" || req.Action == "gmail.thread.list" {
		if b.Policy != nil && b.Policy.Gmail != nil && len(b.Policy.Gmail.AllowedLabels) > 0 {
			if err := b.ensureLabelMap(ctx); err != nil {
				b.logError("label_map_error", fields, start)
				return &types.Response{ID: req.ID, Ok: false, Error: types.NewError("upstream_error", "failed to resolve label ids", "")}
			}
		}
	}

	params, warnings, err := b.Policy.ValidateAndRewrite(ctx, req.Action, req.Params)
	if err != nil {
		b.logDenied("policy_denied", fields, start)
		return &types.Response{ID: req.ID, Ok: false, Error: types.NewError("forbidden", err.Error(), "")}
	}

	runAction := req.Action
	if req.Action == "gmail.send" && b.Policy != nil && b.Policy.DraftSendRequired(params) {
		runAction = "gmail.drafts.create"
		warnings = append(warnings, "action_rewritten:gmail.drafts.create")
		if b.Verbose && b.Logger != nil {
			b.Logger.Info("action_rewritten", map[string]any{"from": req.Action, "to": runAction})
		}
	}

	data, err := b.Runner.Run(ctx, runAction, params)
	if err != nil {
		b.logError("gog_error", fields, start)
		return &types.Response{ID: req.ID, Ok: false, Error: types.NewError("upstream_error", err.Error(), "")}
	}

	clean, redactionWarnings, err := redact.Redact(req.Action, data, b.Policy)
	if err != nil {
		b.logError("redact_error", fields, start)
		return &types.Response{ID: req.ID, Ok: false, Error: types.NewError("redaction_error", err.Error(), "")}
	}
	warnings = append(warnings, redactionWarnings...)

	resp := &types.Response{ID: req.ID, Ok: true, Data: clean}
	if len(warnings) > 0 {
		resp.Warnings = warnings
	}

	b.logAllowed("request_ok", fields, start)
	return resp
}

func (b *Broker) logAllowed(msg string, fields map[string]any, start time.Time) {
	fields = cloneFields(fields)
	fields["decision"] = "allow"
	fields["duration_ms"] = time.Since(start).Milliseconds()
	if b.Logger != nil {
		b.Logger.Info(msg, fields)
	}
}

func (b *Broker) logDenied(msg string, fields map[string]any, start time.Time) {
	fields = cloneFields(fields)
	fields["decision"] = "deny"
	fields["duration_ms"] = time.Since(start).Milliseconds()
	if b.Logger != nil {
		b.Logger.Info(msg, fields)
	}
}

func (b *Broker) logError(msg string, fields map[string]any, start time.Time) {
	fields = cloneFields(fields)
	fields["decision"] = "error"
	fields["duration_ms"] = time.Since(start).Milliseconds()
	if b.Logger != nil {
		b.Logger.Error(msg, fields)
	}
}

func cloneFields(fields map[string]any) map[string]any {
	clone := map[string]any{}
	for k, v := range fields {
		clone[k] = v
	}
	return clone
}

func paramKeys(params map[string]interface{}) []string {
	if len(params) == 0 {
		return nil
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (b *Broker) ensureLabelMap(ctx context.Context) error {
	if b == nil || b.Policy == nil || b.Runner == nil {
		return nil
	}
	b.labelOnce.Do(func() {
		data, err := b.Runner.Run(ctx, "gmail.labels.list", nil)
		if err != nil {
			b.labelErr = err
			return
		}
		idToName := map[string]string{}
		root, ok := data.(map[string]interface{})
		if !ok {
			b.labelErr = errors.New("invalid labels response")
			return
		}
		rawLabels, ok := root["labels"]
		if !ok {
			b.labelErr = errors.New("labels missing")
			return
		}
		items, ok := rawLabels.([]interface{})
		if !ok {
			b.labelErr = errors.New("labels invalid")
			return
		}
		for _, item := range items {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			id, _ := m["id"].(string)
			name, _ := m["name"].(string)
			if id != "" && name != "" {
				idToName[id] = name
			}
		}
		if len(idToName) == 0 {
			b.labelErr = errors.New("labels empty")
			return
		}
		b.Policy.SetLabelMap(idToName)
	})
	return b.labelErr
}
