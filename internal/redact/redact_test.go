package redact

import (
	"testing"

	"gogcli-sandbox/internal/policy"
)

func TestRedactDropsBodyAndLinks(t *testing.T) {
	pol := &policy.Policy{AllowedActions: []string{"gmail.search"}, Gmail: &policy.GmailPolicy{AllowLinks: false}}
	if err := pol.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	input := map[string]interface{}{
		"snippet": "See https://example.com for details",
		"body":    "secret",
	}
	out, warnings, err := Redact("gmail.search", input, pol)
	if err != nil {
		t.Fatalf("redact: %v", err)
	}

	result := out.(map[string]interface{})
	if _, ok := result["body"]; ok {
		t.Fatalf("expected body to be dropped")
	}
	if result["snippet"].(string) == input["snippet"].(string) {
		t.Fatalf("expected snippet to be redacted")
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warnings")
	}
}

func TestRedactEnforcesAllowedLabelIDsWhenPresent(t *testing.T) {
	pol := &policy.Policy{AllowedActions: []string{"gmail.get"}, Gmail: &policy.GmailPolicy{AllowedReadLabels: []string{"Label_123"}}}
	if err := pol.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	input := map[string]interface{}{
		"message": map[string]interface{}{
			"labelIds": []interface{}{"Label_999"},
		},
	}
	_, _, err := Redact("gmail.get", input, pol)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestRedactFiltersSearchResultsByLabel(t *testing.T) {
	pol := &policy.Policy{AllowedActions: []string{"gmail.search"}, Gmail: &policy.GmailPolicy{AllowedReadLabels: []string{"CATEGORY_UPDATES"}}}
	if err := pol.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	input := map[string]interface{}{
		"threads": []interface{}{
			map[string]interface{}{"id": "t1", "labels": []interface{}{"CATEGORY_UPDATES"}},
			map[string]interface{}{"id": "t2", "labels": []interface{}{"INBOX"}},
		},
	}
	out, warnings, err := Redact("gmail.search", input, pol)
	if err != nil {
		t.Fatalf("redact: %v", err)
	}
	result := out.(map[string]interface{})
	threads := result["threads"].([]interface{})
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warnings")
	}
}

func TestRedactFiltersSearchResultsByLabelIDMapping(t *testing.T) {
	pol := &policy.Policy{AllowedActions: []string{"gmail.search"}, Gmail: &policy.GmailPolicy{AllowedReadLabels: []string{"Label_123"}}}
	if err := pol.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	pol.SetLabelMap(map[string]string{"Label_123": "My Label"})
	input := map[string]interface{}{
		"threads": []interface{}{
			map[string]interface{}{"id": "t1", "labels": []interface{}{"My Label"}},
			map[string]interface{}{"id": "t2", "labels": []interface{}{"Other"}},
		},
	}
	out, warnings, err := Redact("gmail.search", input, pol)
	if err != nil {
		t.Fatalf("redact: %v", err)
	}
	result := out.(map[string]interface{})
	threads := result["threads"].([]interface{})
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warnings")
	}
}

func TestRedactDraftIgnoresLabelAllowlist(t *testing.T) {
	pol := &policy.Policy{AllowedActions: []string{"gmail.send"}, Gmail: &policy.GmailPolicy{AllowedReadLabels: []string{"Label_123"}}}
	if err := pol.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	input := map[string]interface{}{
		"draftId": "d1",
		"message": map[string]interface{}{
			"id": "m1",
		},
	}
	_, _, err := Redact("gmail.drafts.create", input, pol)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRedactFiltersCalendarList(t *testing.T) {
	pol := &policy.Policy{AllowedActions: []string{"calendar.list"}, Calendar: &policy.CalendarPolicy{AllowedCalendars: []string{"cal1"}}}
	if err := pol.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	input := map[string]interface{}{
		"calendars": []interface{}{
			map[string]interface{}{"id": "cal1", "summary": "One"},
			map[string]interface{}{"id": "cal2", "summary": "Two"},
		},
	}
	out, warnings, err := Redact("calendar.list", input, pol)
	if err != nil {
		t.Fatalf("redact: %v", err)
	}
	result := out.(map[string]interface{})
	cals := result["calendars"].([]interface{})
	if len(cals) != 1 {
		t.Fatalf("expected 1 calendar, got %d", len(cals))
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warnings")
	}
}

func TestRedactFiltersLabelsList(t *testing.T) {
	pol := &policy.Policy{AllowedActions: []string{"gmail.labels.list"}, Gmail: &policy.GmailPolicy{AllowedReadLabels: []string{"Label_123"}}}
	if err := pol.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	input := map[string]interface{}{
		"labels": []interface{}{
			map[string]interface{}{"id": "Label_123", "name": "Allowed"},
			map[string]interface{}{"id": "Label_999", "name": "Other"},
		},
	}
	out, warnings, err := Redact("gmail.labels.list", input, pol)
	if err != nil {
		t.Fatalf("redact: %v", err)
	}
	result := out.(map[string]interface{})
	labels := result["labels"].([]interface{})
	if len(labels) != 1 {
		t.Fatalf("expected 1 label, got %d", len(labels))
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warnings")
	}
}
