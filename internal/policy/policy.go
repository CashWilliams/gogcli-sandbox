package policy

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gogcli-sandbox/internal/timerange"
)

type Policy struct {
	AllowedActions []string        `json:"allowed_actions"`
	Gmail          *GmailPolicy    `json:"gmail,omitempty"`
	Calendar       *CalendarPolicy `json:"calendar,omitempty"`

	allowedActionSet map[string]struct{}
	labelIDToName    map[string]string
	labelNameToID    map[string]string
	labelMu          sync.RWMutex
	timeZoneProvider func(context.Context) (*time.Location, error)
}

type GmailPolicy struct {
	AllowedReadLabels     []string `json:"allowed_read_labels"`
	AllowedAddLabels      []string `json:"allowed_add_labels"`
	AllowedRemoveLabels   []string `json:"allowed_remove_labels"`
	AllowedSenders        []string `json:"allowed_senders"`
	AllowedSendRecipients []string `json:"allowed_send_recipients"`
	MaxDays               int      `json:"max_days"`
	AllowBody             bool     `json:"allow_body"`
	AllowLinks            bool     `json:"allow_links"`
	DraftOnly             bool     `json:"draft_only"`
	AllowAttachments      bool     `json:"allow_attachments"`
}

type CalendarPolicy struct {
	AllowedCalendars []string `json:"allowed_calendars"`
	AllowDetails     bool     `json:"allow_details"`
	MaxDays          int      `json:"max_days"`
}

func (p *Policy) Validate() error {
	if len(p.AllowedActions) == 0 {
		return errors.New("allowed_actions must not be empty")
	}
	p.allowedActionSet = make(map[string]struct{}, len(p.AllowedActions))
	needsGmail := false
	needsCalendar := false
	for _, action := range p.AllowedActions {
		action = strings.TrimSpace(action)
		if action == "" {
			return errors.New("allowed_actions contains empty action")
		}
		p.allowedActionSet[action] = struct{}{}
		if strings.HasPrefix(action, "gmail.") {
			needsGmail = true
		}
		if strings.HasPrefix(action, "calendar.") {
			needsCalendar = true
		}
	}
	if needsGmail && p.Gmail == nil {
		return errors.New("gmail policy is required for gmail actions")
	}
	if needsCalendar && p.Calendar == nil {
		return errors.New("calendar policy is required for calendar actions")
	}
	return nil
}

func (p *Policy) SetLabelMap(idToName map[string]string) {
	if p == nil {
		return
	}
	normalized := map[string]string{}
	rev := map[string]string{}
	for id, name := range idToName {
		id = strings.TrimSpace(id)
		name = strings.TrimSpace(name)
		if id == "" || name == "" {
			continue
		}
		normalized[strings.ToLower(id)] = name
		rev[strings.ToLower(name)] = id
	}
	p.labelMu.Lock()
	p.labelIDToName = normalized
	p.labelNameToID = rev
	p.labelMu.Unlock()
}

func (p *Policy) LabelNameForID(id string) (string, bool) {
	if p == nil {
		return "", false
	}
	p.labelMu.RLock()
	defer p.labelMu.RUnlock()
	if p.labelIDToName == nil {
		return "", false
	}
	name, ok := p.labelIDToName[strings.ToLower(strings.TrimSpace(id))]
	return name, ok
}

func (p *Policy) LabelIDForName(name string) (string, bool) {
	if p == nil {
		return "", false
	}
	p.labelMu.RLock()
	defer p.labelMu.RUnlock()
	if p.labelNameToID == nil {
		return "", false
	}
	id, ok := p.labelNameToID[strings.ToLower(strings.TrimSpace(name))]
	return id, ok
}

func (p *Policy) SetTimeZoneProvider(fn func(context.Context) (*time.Location, error)) {
	if p == nil {
		return
	}
	p.timeZoneProvider = fn
}

func (p *Policy) IsActionAllowed(action string) bool {
	_, ok := p.allowedActionSet[action]
	return ok
}

func (p *Policy) ValidateAndRewrite(ctx context.Context, action string, params map[string]interface{}) (map[string]interface{}, []string, error) {
	if params == nil {
		params = map[string]interface{}{}
	}
	warnings := []string{}

	switch action {
	case "gmail.search", "gmail.thread.list":
		return p.rewriteGmailQuery(params, warnings)
	case "gmail.thread.get":
		return p.rewriteGmailThreadGet(params, warnings)
	case "gmail.thread.modify":
		return p.rewriteGmailThreadModify(params, warnings)
	case "gmail.get":
		return p.rewriteGmailGet(params, warnings)
	case "gmail.send":
		return p.rewriteGmailSend(params, warnings)
	case "gmail.drafts.create":
		return p.rewriteGmailDraftCreate(params, warnings)
	case "gmail.labels.list":
		return params, warnings, nil
	case "gmail.labels.get":
		return p.rewriteGmailLabelsGet(params, warnings)
	case "gmail.labels.modify":
		return p.rewriteGmailLabelsModify(params, warnings)
	case "calendar.list":
		return params, warnings, nil
	case "calendar.events":
		return p.rewriteCalendarEvents(ctx, params, warnings)
	case "calendar.freebusy":
		return p.rewriteCalendarFreeBusy(ctx, params, warnings)
	case "policy.actions":
		if len(params) > 0 {
			return nil, nil, errors.New("params must be empty")
		}
		return params, warnings, nil
	default:
		return nil, nil, fmt.Errorf("unsupported action: %s", action)
	}
}

func (p *Policy) rewriteGmailQuery(params map[string]interface{}, warnings []string) (map[string]interface{}, []string, error) {
	query, ok := getString(params, "query")
	if !ok || strings.TrimSpace(query) == "" {
		return nil, nil, errors.New("params.query is required")
	}

	if p.Gmail != nil {
		if p.Gmail.MaxDays > 0 {
			maxDays := p.Gmail.MaxDays
			if days, ok := extractNewerThanDays(query); ok {
				if days > maxDays {
					return nil, nil, fmt.Errorf("query newer_than exceeds max_days (%d)", maxDays)
				}
			} else if after, ok := extractAfterDate(query); ok {
				limit := time.Now().AddDate(0, 0, -maxDays)
				if after.Before(limit) {
					return nil, nil, fmt.Errorf("query after date exceeds max_days (%d)", maxDays)
				}
			} else {
				query = strings.TrimSpace(query + " newer_than:" + strconv.Itoa(maxDays) + "d")
				warnings = append(warnings, "query_rewritten:newer_than")
			}
		}

		if len(p.Gmail.AllowedSenders) > 0 {
			query = appendSenderRestriction(query, p.Gmail.AllowedSenders)
			warnings = append(warnings, "query_rewritten:sender_restriction")
		}
	}

	params["query"] = query
	return params, warnings, nil
}

func (p *Policy) rewriteGmailThreadGet(params map[string]interface{}, warnings []string) (map[string]interface{}, []string, error) {
	if val, ok := getString(params, "id"); ok {
		params["thread_id"] = val
		return params, warnings, nil
	}
	if val, ok := getString(params, "thread_id"); ok {
		params["thread_id"] = val
		return params, warnings, nil
	}
	return nil, nil, errors.New("params.id or params.thread_id is required")
}

func (p *Policy) rewriteGmailThreadModify(params map[string]interface{}, warnings []string) (map[string]interface{}, []string, error) {
	threadID, ok := getStringAny(params, "thread_id", "id")
	if !ok || strings.TrimSpace(threadID) == "" {
		return nil, nil, errors.New("params.thread_id is required")
	}

	addLabels, _ := getStringSlice(params, "add")
	removeLabels, _ := getStringSlice(params, "remove")
	if len(addLabels) == 0 && len(removeLabels) == 0 {
		return nil, nil, errors.New("params.add or params.remove is required")
	}
	if err := p.validateLabels(addLabels, p.Gmail.AllowedAddLabels, "add", false); err != nil {
		return nil, nil, err
	}
	if err := p.validateLabels(removeLabels, p.Gmail.AllowedRemoveLabels, "remove", false); err != nil {
		return nil, nil, err
	}

	params["thread_id"] = strings.TrimSpace(threadID)
	if len(addLabels) > 0 {
		params["add"] = strings.Join(addLabels, ",")
	}
	if len(removeLabels) > 0 {
		params["remove"] = strings.Join(removeLabels, ",")
	}
	return params, warnings, nil
}

func (p *Policy) rewriteGmailGet(params map[string]interface{}, warnings []string) (map[string]interface{}, []string, error) {
	if val, ok := getString(params, "id"); ok {
		params["message_id"] = val
	} else if val, ok := getString(params, "message_id"); ok {
		params["message_id"] = val
	} else {
		return nil, nil, errors.New("params.id or params.message_id is required")
	}

	if format, ok := getString(params, "format"); ok && format != "" && format != "metadata" {
		return nil, nil, errors.New("format must be metadata")
	}
	params["format"] = "metadata"

	if _, ok := params["headers"]; ok {
		delete(params, "headers")
		warnings = append(warnings, "headers_ignored:default")
	}

	return params, warnings, nil
}

func (p *Policy) rewriteGmailSend(params map[string]interface{}, warnings []string) (map[string]interface{}, []string, error) {
	if p.Gmail == nil {
		return nil, nil, errors.New("gmail policy missing")
	}
	if params == nil {
		params = map[string]interface{}{}
	}

	if _, ok := params["track"]; ok {
		return nil, nil, errors.New("tracking is not allowed")
	}
	if _, ok := params["track_split"]; ok {
		return nil, nil, errors.New("tracking is not allowed")
	}
	if _, ok := params["reply_all"]; ok {
		return nil, nil, errors.New("reply_all is not allowed")
	}
	if _, ok := params["thread_id"]; ok && p.Gmail.DraftOnly {
		return nil, nil, errors.New("thread_id is not supported in draft_only mode")
	}
	if _, ok := params["attach"]; ok && !p.Gmail.AllowAttachments {
		return nil, nil, errors.New("attachments are not allowed")
	}

	if reason := p.draftSendReason(params); reason != "" {
		warnings = append(warnings, "draft_only:"+reason)
	}
	return params, warnings, nil
}

func (p *Policy) rewriteGmailDraftCreate(params map[string]interface{}, warnings []string) (map[string]interface{}, []string, error) {
	if p.Gmail == nil {
		return nil, nil, errors.New("gmail policy missing")
	}
	if params == nil {
		params = map[string]interface{}{}
	}
	if _, ok := params["track"]; ok {
		return nil, nil, errors.New("tracking is not allowed")
	}
	if _, ok := params["track_split"]; ok {
		return nil, nil, errors.New("tracking is not allowed")
	}
	if _, ok := params["reply_all"]; ok {
		return nil, nil, errors.New("reply_all is not allowed")
	}
	if _, ok := params["thread_id"]; ok {
		return nil, nil, errors.New("thread_id is not supported for draft creation")
	}
	if _, ok := params["attach"]; ok && !p.Gmail.AllowAttachments {
		return nil, nil, errors.New("attachments are not allowed")
	}
	return params, warnings, nil
}

func (p *Policy) rewriteGmailLabelsGet(params map[string]interface{}, warnings []string) (map[string]interface{}, []string, error) {
	label, ok := getStringAny(params, "label", "label_id", "id")
	if !ok || strings.TrimSpace(label) == "" {
		return nil, nil, errors.New("params.label is required")
	}
	label = strings.TrimSpace(label)
	if err := p.validateLabels([]string{label}, p.Gmail.AllowedReadLabels, "read", true); err != nil {
		return nil, nil, err
	}
	params["label"] = label
	return params, warnings, nil
}

func (p *Policy) rewriteGmailLabelsModify(params map[string]interface{}, warnings []string) (map[string]interface{}, []string, error) {
	threadIDs, ok := getStringSlice(params, "thread_ids")
	if !ok {
		if tid, ok := getStringAny(params, "thread_id", "id"); ok {
			threadIDs = []string{tid}
		}
	}
	if len(threadIDs) == 0 {
		return nil, nil, errors.New("params.thread_ids is required")
	}

	addLabels, _ := getStringSlice(params, "add")
	removeLabels, _ := getStringSlice(params, "remove")
	if len(addLabels) == 0 && len(removeLabels) == 0 {
		return nil, nil, errors.New("params.add or params.remove is required")
	}
	if err := p.validateLabels(addLabels, p.Gmail.AllowedAddLabels, "add", false); err != nil {
		return nil, nil, err
	}
	if err := p.validateLabels(removeLabels, p.Gmail.AllowedRemoveLabels, "remove", false); err != nil {
		return nil, nil, err
	}

	params["thread_ids"] = threadIDs
	if len(addLabels) > 0 {
		params["add"] = strings.Join(addLabels, ",")
	}
	if len(removeLabels) > 0 {
		params["remove"] = strings.Join(removeLabels, ",")
	}
	return params, warnings, nil
}

func (p *Policy) DraftSendRequired(params map[string]interface{}) bool {
	if p == nil || p.Gmail == nil {
		return false
	}
	return p.draftSendReason(params) != ""
}

func (p *Policy) validateLabels(labels []string, allowed []string, mode string, allowEmpty bool) error {
	if len(labels) == 0 {
		return nil
	}
	if p == nil || p.Gmail == nil {
		return errors.New("gmail policy missing")
	}
	if len(allowed) == 0 {
		if allowEmpty {
			return nil
		}
		return fmt.Errorf("no labels allowed for %s", mode)
	}
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if !p.isLabelAllowed(label, allowed) {
			return fmt.Errorf("label not allowed: %s", label)
		}
	}
	return nil
}

func (p *Policy) isLabelAllowed(label string, allowed []string) bool {
	if p == nil || p.Gmail == nil {
		return false
	}
	if len(allowed) == 0 {
		return false
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return false
	}
	labelLower := strings.ToLower(label)
	allowedSet := map[string]struct{}{}
	for _, allowedLabel := range allowed {
		allowedLabel = strings.TrimSpace(allowedLabel)
		if allowedLabel == "" {
			continue
		}
		allowedSet[strings.ToLower(allowedLabel)] = struct{}{}
	}
	if _, ok := allowedSet[labelLower]; ok {
		return true
	}
	if id, ok := p.LabelIDForName(label); ok {
		if _, ok := allowedSet[strings.ToLower(id)]; ok {
			return true
		}
	}
	if name, ok := p.LabelNameForID(label); ok {
		if _, ok := allowedSet[strings.ToLower(name)]; ok {
			return true
		}
	}
	return false
}

func (p *Policy) draftSendReason(params map[string]interface{}) string {
	if p == nil || p.Gmail == nil {
		return ""
	}
	if p.Gmail.DraftOnly {
		return "policy"
	}
	if len(p.Gmail.AllowedSendRecipients) == 0 {
		return ""
	}
	recipients, ok := collectRecipients(params)
	if !ok {
		return "recipients_missing"
	}
	if !recipientsAllowed(recipients, p.Gmail.AllowedSendRecipients) {
		return "recipient_not_allowed"
	}
	return ""
}

func (p *Policy) rewriteCalendarEvents(ctx context.Context, params map[string]interface{}, warnings []string) (map[string]interface{}, []string, error) {
	cal, ok := getString(params, "calendar_id")
	if !ok {
		return nil, nil, errors.New("params.calendar_id is required")
	}
	if p.Calendar != nil && len(p.Calendar.AllowedCalendars) > 0 {
		if !stringInSlice(cal, p.Calendar.AllowedCalendars) {
			return nil, nil, errors.New("calendar_id is not allowed")
		}
	}
	rangeWarnings, err := p.resolveCalendarRange(ctx, params, false)
	if err != nil {
		return nil, nil, err
	}
	warnings = append(warnings, rangeWarnings...)
	return params, warnings, nil
}

func (p *Policy) rewriteCalendarFreeBusy(ctx context.Context, params map[string]interface{}, warnings []string) (map[string]interface{}, []string, error) {
	rangeWarnings, err := p.resolveCalendarRange(ctx, params, true)
	if err != nil {
		return nil, nil, err
	}
	warnings = append(warnings, rangeWarnings...)

	calIDs, ok := getStringSlice(params, "calendar_ids")
	if !ok {
		return nil, nil, errors.New("params.calendar_ids is required")
	}
	if p.Calendar != nil && len(p.Calendar.AllowedCalendars) > 0 {
		for _, id := range calIDs {
			if !stringInSlice(id, p.Calendar.AllowedCalendars) {
				return nil, nil, errors.New("calendar_ids contains disallowed calendar")
			}
		}
	}

	return params, warnings, nil
}

var newerThanRe = regexp.MustCompile(`(?i)\bnewer_than:(\d+)d`)
var afterRe = regexp.MustCompile(`(?i)\bafter:(\d{4})/(\d{2})/(\d{2})`)

func extractNewerThanDays(query string) (int, bool) {
	m := newerThanRe.FindStringSubmatch(query)
	if len(m) != 2 {
		return 0, false
	}
	val, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return val, true
}

func extractAfterDate(query string) (time.Time, bool) {
	m := afterRe.FindStringSubmatch(query)
	if len(m) != 4 {
		return time.Time{}, false
	}
	val := fmt.Sprintf("%s-%s-%s", m[1], m[2], m[3])
	t, err := time.Parse("2006-01-02", val)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func appendSenderRestriction(query string, senders []string) string {
	parts := []string{}
	for _, sender := range senders {
		sender = strings.TrimSpace(sender)
		if sender == "" {
			continue
		}
		if !strings.HasPrefix(sender, "@") {
			sender = "@" + sender
		}
		parts = append(parts, "from:"+sender)
	}
	if len(parts) == 0 {
		return query
	}
	return strings.TrimSpace(query + " (" + strings.Join(parts, " OR ") + ")")
}

func getString(params map[string]interface{}, key string) (string, bool) {
	val, ok := params[key]
	if !ok || val == nil {
		return "", false
	}
	switch v := val.(type) {
	case string:
		return v, true
	default:
		return "", false
	}
}

func getStringAny(params map[string]interface{}, keys ...string) (string, bool) {
	for _, key := range keys {
		if val, ok := getString(params, key); ok {
			return val, true
		}
	}
	return "", false
}

func getBool(params map[string]interface{}, key string) (bool, bool) {
	val, ok := params[key]
	if !ok || val == nil {
		return false, false
	}
	switch v := val.(type) {
	case bool:
		return v, true
	case string:
		if v == "true" {
			return true, true
		}
		if v == "false" {
			return false, true
		}
		return false, false
	default:
		return false, false
	}
}

func getInt(params map[string]interface{}, key string) (int, bool) {
	val, ok := params[key]
	if !ok || val == nil {
		return 0, false
	}
	switch v := val.(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func getStringSlice(params map[string]interface{}, key string) ([]string, bool) {
	val, ok := params[key]
	if !ok || val == nil {
		return nil, false
	}
	if s, ok := val.(string); ok {
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	}
	arr, ok := val.([]interface{})
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func stringInSlice(s string, list []string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func parseAbsoluteTime(val string) (time.Time, bool) {
	val = strings.TrimSpace(val)
	if val == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, val); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02T15:04:05-0700", val); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func cleanupTimeParams(params map[string]interface{}) {
	cleanupTimeParams(params)
}

func (p *Policy) resolveCalendarRange(ctx context.Context, params map[string]interface{}, require bool) ([]string, error) {
	if p.Calendar == nil {
		return nil, errors.New("calendar policy missing")
	}

	flags := timerange.Flags{}
	if from, ok := getStringAny(params, "time_min", "from"); ok {
		flags.From = from
	}
	if to, ok := getStringAny(params, "time_max", "to"); ok {
		flags.To = to
	}
	if v, ok := getBool(params, "today"); ok {
		flags.Today = v
	}
	if v, ok := getBool(params, "tomorrow"); ok {
		flags.Tomorrow = v
	}
	if v, ok := getBool(params, "week"); ok {
		flags.Week = v
	}
	if v, ok := getInt(params, "days"); ok {
		flags.Days = v
	}
	if v, ok := getString(params, "week_start"); ok {
		flags.WeekStart = v
	}

	hasTimeFlags := flags.From != "" || flags.To != "" || flags.Today || flags.Tomorrow || flags.Week || flags.Days > 0
	if require && !hasTimeFlags {
		return nil, errors.New("params.time_min and params.time_max are required")
	}

	defaultDays := 7
	if p.Calendar.MaxDays > 0 && p.Calendar.MaxDays < defaultDays {
		defaultDays = p.Calendar.MaxDays
	}
	defaultWindow := time.Duration(defaultDays) * 24 * time.Hour

	needsTZ := flags.Today || flags.Tomorrow || flags.Week || flags.Days > 0 || flags.From == "" || flags.To == ""
	if !needsTZ {
		if fromAbs, okFrom := parseAbsoluteTime(flags.From); okFrom {
			if toAbs, okTo := parseAbsoluteTime(flags.To); okTo {
				if toAbs.Before(fromAbs) {
					return nil, errors.New("params.time_max must be after time_min")
				}
				if p.Calendar.MaxDays > 0 {
					maxWindow := time.Duration(p.Calendar.MaxDays) * 24 * time.Hour
					if toAbs.Sub(fromAbs) > maxWindow {
						return nil, errors.New("calendar range exceeds max_days")
					}
				}
				params["time_min"] = fromAbs.Format(time.RFC3339)
				params["time_max"] = toAbs.Format(time.RFC3339)
				cleanupTimeParams(params)
				return nil, nil
			}
		}
		needsTZ = true
	}

	if p.timeZoneProvider == nil {
		return nil, errors.New("timezone provider not configured")
	}
	loc, err := p.timeZoneProvider(ctx)
	if err != nil {
		return nil, err
	}
	if loc == nil {
		return nil, errors.New("timezone unavailable")
	}

	defaults := timerange.Defaults{FromOffset: 0, ToOffset: defaultWindow, ToFromOffset: defaultWindow}
	tr, err := timerange.Resolve(time.Now(), loc, flags, defaults)
	if err != nil {
		return nil, err
	}
	if tr.To.Before(tr.From) {
		return nil, errors.New("params.time_max must be after time_min")
	}
	if p.Calendar.MaxDays > 0 {
		maxWindow := time.Duration(p.Calendar.MaxDays) * 24 * time.Hour
		if tr.To.Sub(tr.From) > maxWindow {
			return nil, errors.New("calendar range exceeds max_days")
		}
	}

	delete(params, "from")
	delete(params, "to")
	delete(params, "time_min")
	delete(params, "time_max")
	delete(params, "today")
	delete(params, "tomorrow")
	delete(params, "week")
	delete(params, "days")
	delete(params, "week_start")

	params["time_min"] = tr.From.Format(time.RFC3339)
	params["time_max"] = tr.To.Format(time.RFC3339)
	return nil, nil
}

func collectRecipients(params map[string]interface{}) ([]string, bool) {
	recipients := []string{}
	for _, key := range []string{"to", "cc", "bcc"} {
		raw, ok := params[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case string:
			recipients = append(recipients, splitRecipients(v)...)
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					recipients = append(recipients, splitRecipients(s)...)
				}
			}
		}
	}
	if len(recipients) == 0 {
		return nil, false
	}
	return recipients, true
}

func splitRecipients(input string) []string {
	out := []string{}
	for _, part := range strings.Split(input, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		addr, err := mail.ParseAddress(part)
		if err == nil && addr != nil && addr.Address != "" {
			out = append(out, strings.ToLower(addr.Address))
			continue
		}
		out = append(out, strings.ToLower(part))
	}
	return out
}

func recipientsAllowed(recipients []string, allowed []string) bool {
	allowedSet := map[string]struct{}{}
	for _, addr := range allowed {
		addr = strings.ToLower(strings.TrimSpace(addr))
		if addr != "" {
			allowedSet[addr] = struct{}{}
		}
	}
	for _, rcpt := range recipients {
		if _, ok := allowedSet[strings.ToLower(rcpt)]; !ok {
			return false
		}
	}
	return true
}
