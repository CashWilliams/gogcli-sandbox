package redact

import (
	"errors"
	"regexp"
	"strings"

	"gogcli-sandbox/internal/policy"
)

var urlRe = regexp.MustCompile(`https?://\S+`)
var emailRe = regexp.MustCompile(`(?i)[A-Z0-9._%+-]+@([A-Z0-9.-]+\.[A-Z]{2,})`)

var alwaysDropKeys = map[string]struct{}{
	"attachment":  {},
	"attachments": {},
}

var bodyKeys = map[string]struct{}{
	"body":     {},
	"payload":  {},
	"parts":    {},
	"raw":      {},
	"html":     {},
	"htmlbody": {},
	"mime":     {},
	"mimeType": {},
}

var snippetKeys = map[string]struct{}{
	"snippetHtml":  {},
	"snippet_html": {},
}

var calendarDetailKeys = map[string]struct{}{
	"hangoutLink":    {},
	"conferenceData": {},
	"location":       {},
	"description":    {},
	"htmlLink":       {},
}

func Redact(action string, data any, pol *policy.Policy) (any, []string, error) {
	warnings := []string{}
	switch action {
	case "gmail.search", "gmail.thread.list", "gmail.thread.get", "gmail.thread.modify", "gmail.get", "gmail.send", "gmail.drafts.create", "gmail.labels.list", "gmail.labels.get", "gmail.labels.modify":
		if pol.Gmail == nil {
			return nil, nil, errors.New("gmail policy missing")
		}
		clean, w, err := redactAny(data, pol)
		warnings = append(warnings, w...)
		if err != nil {
			return nil, nil, err
		}
		readAllowed := pol.Gmail.AllowedReadLabels
		labelUnion := allowedLabelUnion(pol.Gmail)
		switch action {
		case "gmail.search", "gmail.thread.list":
			if len(readAllowed) > 0 {
				filtered, fw, err := filterSearchResults(clean, readAllowed, pol)
				if err != nil {
					return nil, nil, err
				}
				warnings = append(warnings, fw...)
				return filtered, warnings, nil
			}
		case "gmail.labels.list":
			if len(labelUnion) > 0 {
				filtered, fw, err := filterLabelsList(clean, labelUnion)
				if err != nil {
					return nil, nil, err
				}
				warnings = append(warnings, fw...)
				return filtered, warnings, nil
			}
		case "gmail.send", "gmail.drafts.create":
			// Sends/drafts may not include label info; do not enforce label checks.
			return clean, warnings, nil
		default:
			if len(readAllowed) > 0 {
				if found, ok := hasAllowedLabelIDs(clean, readAllowed); found && !ok {
					return nil, nil, errors.New("response does not include allowed labels")
				}
			}
		}
		return clean, warnings, nil
	case "calendar.list", "calendar.events", "calendar.freebusy":
		if pol.Calendar == nil {
			return nil, nil, errors.New("calendar policy missing")
		}
		clean, w, err := redactAny(data, pol)
		warnings = append(warnings, w...)
		if err != nil {
			return nil, nil, err
		}
		if action == "calendar.list" && len(pol.Calendar.AllowedCalendars) > 0 {
			filtered, fw, err := filterCalendarList(clean, pol.Calendar.AllowedCalendars)
			if err != nil {
				return nil, nil, err
			}
			warnings = append(warnings, fw...)
			return filtered, warnings, nil
		}
		return clean, warnings, nil
	default:
		return data, warnings, nil
	}
}

func redactAny(val any, pol *policy.Policy) (any, []string, error) {
	warnings := []string{}
	switch v := val.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, item := range v {
			if shouldDropKey(key, pol) {
				warnings = append(warnings, "redacted:"+key)
				continue
			}
			clean, w, err := redactAny(item, pol)
			if err != nil {
				return nil, nil, err
			}
			warnings = append(warnings, w...)
			out[key] = clean
		}
		return out, warnings, nil
	case []interface{}:
		out := make([]interface{}, 0, len(v))
		for _, item := range v {
			clean, w, err := redactAny(item, pol)
			if err != nil {
				return nil, nil, err
			}
			warnings = append(warnings, w...)
			out = append(out, clean)
		}
		return out, warnings, nil
	case string:
		clean := sanitizeString(v, pol)
		if clean != v {
			warnings = append(warnings, "redacted:string")
		}
		return clean, warnings, nil
	default:
		return v, warnings, nil
	}
}

func sanitizeString(input string, pol *policy.Policy) string {
	output := input
	if pol.Gmail != nil && !pol.Gmail.AllowLinks {
		output = urlRe.ReplaceAllString(output, "[redacted]")
	}
	if pol.Gmail != nil && len(pol.Gmail.AllowedSenders) > 0 {
		output = maskEmails(output, pol.Gmail.AllowedSenders)
	}
	if pol.Calendar != nil && !pol.Calendar.AllowDetails {
		output = urlRe.ReplaceAllString(output, "[redacted]")
	}
	return output
}

func shouldDropKey(key string, pol *policy.Policy) bool {
	if _, ok := alwaysDropKeys[key]; ok {
		return true
	}
	if _, ok := snippetKeys[key]; ok {
		return true
	}
	if pol.Gmail != nil && !pol.Gmail.AllowBody {
		if _, ok := bodyKeys[key]; ok {
			return true
		}
	}
	if pol.Calendar != nil && !pol.Calendar.AllowDetails {
		if _, ok := calendarDetailKeys[key]; ok {
			return true
		}
	}
	return false
}

func maskEmails(input string, allowedDomains []string) string {
	allowed := map[string]struct{}{}
	for _, domain := range allowedDomains {
		allowed[strings.ToLower(strings.TrimPrefix(domain, "@"))] = struct{}{}
	}
	return emailRe.ReplaceAllStringFunc(input, func(match string) string {
		parts := strings.Split(match, "@")
		if len(parts) != 2 {
			return "[redacted]"
		}
		domain := strings.ToLower(parts[1])
		if _, ok := allowed[domain]; ok {
			return match
		}
		return "[redacted]"
	})
}

func hasAllowedLabelIDs(val any, allowed []string) (bool, bool) {
	set := map[string]struct{}{}
	for _, label := range allowed {
		set[strings.ToLower(label)] = struct{}{}
	}
	switch v := val.(type) {
	case map[string]interface{}:
		for key, item := range v {
			if strings.EqualFold(key, "labelIds") || strings.EqualFold(key, "label_ids") {
				if arr, ok := item.([]interface{}); ok {
					foundAny := false
					for _, label := range arr {
						if s, ok := label.(string); ok {
							foundAny = true
							if _, ok := set[strings.ToLower(s)]; ok {
								return true, true
							}
						}
					}
					if foundAny {
						return true, false
					}
				}
			}
			if found, ok := hasAllowedLabelIDs(item, allowed); found {
				return true, ok
			}
		}
	case []interface{}:
		for _, item := range v {
			if found, ok := hasAllowedLabelIDs(item, allowed); found {
				return true, ok
			}
		}
	}
	return false, false
}

func filterSearchResults(data any, allowed []string, pol *policy.Policy) (any, []string, error) {
	if len(allowed) == 0 {
		return data, nil, nil
	}
	root, ok := data.(map[string]interface{})
	if !ok {
		return data, nil, nil
	}
	rawThreads, ok := root["threads"]
	if !ok {
		return data, nil, nil
	}
	items, ok := rawThreads.([]interface{})
	if !ok {
		return data, nil, nil
	}

	filtered := make([]interface{}, 0, len(items))
	for _, item := range items {
		if allowedLabelForItem(item, allowed, pol) {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) != len(items) {
		root["threads"] = filtered
		return root, []string{"filtered:labels"}, nil
	}
	return root, nil, nil
}

func filterLabelsList(data any, allowed []string) (any, []string, error) {
	if len(allowed) == 0 {
		return data, nil, nil
	}
	root, ok := data.(map[string]interface{})
	if !ok {
		return data, nil, nil
	}
	rawLabels, ok := root["labels"]
	if !ok {
		return data, nil, nil
	}
	items, ok := rawLabels.([]interface{})
	if !ok {
		return data, nil, nil
	}

	set := map[string]struct{}{}
	for _, label := range allowed {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		set[strings.ToLower(label)] = struct{}{}
	}

	filtered := make([]interface{}, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		name, _ := m["name"].(string)
		if _, ok := set[strings.ToLower(id)]; ok {
			filtered = append(filtered, item)
			continue
		}
		if _, ok := set[strings.ToLower(name)]; ok {
			filtered = append(filtered, item)
			continue
		}
	}
	if len(filtered) != len(items) {
		root["labels"] = filtered
		return root, []string{"filtered:labels"}, nil
	}
	return root, nil, nil
}

func allowedLabelUnion(gmail *policy.GmailPolicy) []string {
	if gmail == nil {
		return nil
	}
	set := map[string]struct{}{}
	add := func(vals []string) {
		for _, v := range vals {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			set[strings.ToLower(v)] = struct{}{}
		}
	}
	add(gmail.AllowedReadLabels)
	add(gmail.AllowedAddLabels)
	add(gmail.AllowedRemoveLabels)

	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	return out
}

func allowedLabelForItem(item any, allowed []string, pol *policy.Policy) bool {
	labels := extractLabels(item)
	if len(labels) == 0 {
		return false
	}
	allowedSet := map[string]struct{}{}
	for _, label := range allowed {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		allowedSet[strings.ToLower(label)] = struct{}{}
		if pol != nil {
			if polLabelName, ok := pol.LabelNameForID(label); ok {
				allowedSet[strings.ToLower(polLabelName)] = struct{}{}
			}
		}
	}
	for _, label := range labels {
		if _, ok := allowedSet[strings.ToLower(label)]; ok {
			return true
		}
	}
	return false
}

func extractLabels(item any) []string {
	m, ok := item.(map[string]interface{})
	if !ok {
		return nil
	}
	for _, key := range []string{"labels", "labelIds", "label_ids"} {
		if val, ok := m[key]; ok {
			if labels := coerceStringList(val); len(labels) > 0 {
				return labels
			}
		}
	}
	return nil
}

func coerceStringList(val interface{}) []string {
	switch v := val.(type) {
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func filterCalendarList(data any, allowed []string) (any, []string, error) {
	if len(allowed) == 0 {
		return data, nil, nil
	}
	root, ok := data.(map[string]interface{})
	if !ok {
		return data, nil, nil
	}
	rawCals, ok := root["calendars"]
	if !ok {
		return data, nil, nil
	}
	items, ok := rawCals.([]interface{})
	if !ok {
		return data, nil, nil
	}
	allowedSet := map[string]struct{}{}
	for _, id := range allowed {
		id = strings.TrimSpace(id)
		if id != "" {
			allowedSet[strings.ToLower(id)] = struct{}{}
		}
	}
	filtered := make([]interface{}, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if id, ok := m["id"].(string); ok {
			if _, ok := allowedSet[strings.ToLower(id)]; ok {
				filtered = append(filtered, item)
			}
		}
	}
	if len(filtered) != len(items) {
		root["calendars"] = filtered
		return root, []string{"filtered:calendars"}, nil
	}
	return root, nil, nil
}
