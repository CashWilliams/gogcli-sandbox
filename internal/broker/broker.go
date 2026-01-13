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
	Policies       *policy.PolicySet
	RunnerProvider gog.RunnerProvider
	DefaultAccount string
	Logger         Logger
	Verbose        bool
	labelMu        sync.Mutex
	labelOnce      map[string]*sync.Once
	labelErr       map[string]error
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
	if req.Account != "" {
		fields["account"] = req.Account
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

	pol, account, err := b.resolvePolicy(req.Account)
	if err != nil {
		code := "forbidden"
		if errors.Is(err, policy.ErrAccountRequired) {
			code = "bad_request"
		}
		b.logDenied("account_denied", fields, start)
		return &types.Response{ID: req.ID, Ok: false, Error: types.NewError(code, err.Error(), "")}
	}
	fields["account"] = account

	if !pol.IsActionAllowed(req.Action) {
		b.logDenied("action_denied", fields, start)
		return &types.Response{ID: req.ID, Ok: false, Error: types.NewError("forbidden", "action not allowed", "")}
	}
	if req.Action == "gmail.search" || req.Action == "gmail.thread.list" {
		if pol != nil && pol.Gmail != nil && len(pol.Gmail.AllowedLabels) > 0 {
			if err := b.ensureLabelMap(ctx, account, pol); err != nil {
				b.logError("label_map_error", fields, start)
				return &types.Response{ID: req.ID, Ok: false, Error: types.NewError("upstream_error", "failed to resolve label ids", "")}
			}
		}
	}

	params, warnings, err := pol.ValidateAndRewrite(ctx, req.Action, req.Params)
	if err != nil {
		b.logDenied("policy_denied", fields, start)
		return &types.Response{ID: req.ID, Ok: false, Error: types.NewError("forbidden", err.Error(), "")}
	}

	runAction := req.Action
	if req.Action == "gmail.send" && pol != nil && pol.DraftSendRequired(params) {
		runAction = "gmail.drafts.create"
		warnings = append(warnings, "action_rewritten:gmail.drafts.create")
		if b.Verbose && b.Logger != nil {
			b.Logger.Info("action_rewritten", map[string]any{"from": req.Action, "to": runAction})
		}
	}

	if req.Action == "policy.actions" {
		actions := append([]string{}, pol.AllowedActions...)
		sort.Strings(actions)
		resp := &types.Response{ID: req.ID, Ok: true, Data: map[string]any{
			"account": account,
			"actions": actions,
		}}
		if len(warnings) > 0 {
			resp.Warnings = warnings
		}
		b.logAllowed("request_ok", fields, start)
		return resp
	}

	runner := b.RunnerProvider.RunnerFor(account)
	data, err := runner.Run(ctx, runAction, params)
	if err != nil {
		b.logError("gog_error", fields, start)
		return &types.Response{ID: req.ID, Ok: false, Error: types.NewError("upstream_error", err.Error(), "")}
	}

	clean, redactionWarnings, err := redact.Redact(req.Action, data, pol)
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

func (b *Broker) ensureLabelMap(ctx context.Context, account string, pol *policy.Policy) error {
	if b == nil || pol == nil || b.RunnerProvider == nil {
		return nil
	}
	once, err := b.labelOnceFor(account)
	if err != nil {
		return err
	}

	once.Do(func() {
		runner := b.RunnerProvider.RunnerFor(account)
		data, runErr := runner.Run(ctx, "gmail.labels.list", nil)
		if runErr != nil {
			b.setLabelErr(account, runErr)
			return
		}
		idToName := map[string]string{}
		root, ok := data.(map[string]interface{})
		if !ok {
			b.setLabelErr(account, errors.New("invalid labels response"))
			return
		}
		rawLabels, ok := root["labels"]
		if !ok {
			b.setLabelErr(account, errors.New("labels missing"))
			return
		}
		items, ok := rawLabels.([]interface{})
		if !ok {
			b.setLabelErr(account, errors.New("labels invalid"))
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
			b.setLabelErr(account, errors.New("labels empty"))
			return
		}
		pol.SetLabelMap(idToName)
		b.setLabelErr(account, nil)
	})
	return b.getLabelErr(account)
}

func (b *Broker) labelOnceFor(account string) (*sync.Once, error) {
	b.labelMu.Lock()
	defer b.labelMu.Unlock()
	if b.labelOnce == nil {
		b.labelOnce = map[string]*sync.Once{}
	}
	if b.labelErr == nil {
		b.labelErr = map[string]error{}
	}
	key := account
	if key == "" {
		key = "_default"
	}
	once, ok := b.labelOnce[key]
	if !ok {
		once = &sync.Once{}
		b.labelOnce[key] = once
	}
	return once, nil
}

func (b *Broker) setLabelErr(account string, err error) {
	b.labelMu.Lock()
	defer b.labelMu.Unlock()
	if b.labelErr == nil {
		b.labelErr = map[string]error{}
	}
	key := account
	if key == "" {
		key = "_default"
	}
	b.labelErr[key] = err
}

func (b *Broker) getLabelErr(account string) error {
	b.labelMu.Lock()
	defer b.labelMu.Unlock()
	if b.labelErr == nil {
		return nil
	}
	key := account
	if key == "" {
		key = "_default"
	}
	return b.labelErr[key]
}

func (b *Broker) resolvePolicy(account string) (*policy.Policy, string, error) {
	if b == nil || b.Policies == nil {
		return nil, "", errors.New("policy is required")
	}
	return b.Policies.Resolve(account, b.DefaultAccount)
}
