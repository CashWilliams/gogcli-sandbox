package policy

import (
	"context"
	"testing"
)

func TestRewriteGmailQueryAddsNewerThan(t *testing.T) {
	p := &Policy{AllowedActions: []string{"gmail.search"}, Gmail: &GmailPolicy{MaxDays: 7, AllowedLabels: []string{"Label_123"}}}
	if err := p.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	params := map[string]interface{}{"query": "label:Label_123"}
	out, warnings, err := p.ValidateAndRewrite(context.Background(), "gmail.search", params)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	q := out["query"].(string)
	if q != "label:Label_123 newer_than:7d" {
		t.Fatalf("unexpected query: %s", q)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warnings")
	}
}

func TestRewriteGmailQueryAllowsAnyLabel(t *testing.T) {
	p := &Policy{AllowedActions: []string{"gmail.search"}, Gmail: &GmailPolicy{AllowedLabels: []string{"Label_123"}}}
	if err := p.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	params := map[string]interface{}{"query": "label:OTHER"}
	_, _, err := p.ValidateAndRewrite(context.Background(), "gmail.search", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRewriteGmailGetForcesMetadata(t *testing.T) {
	p := &Policy{AllowedActions: []string{"gmail.get"}, Gmail: &GmailPolicy{}}
	if err := p.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	params := map[string]interface{}{"message_id": "msg123"}
	out, _, err := p.ValidateAndRewrite(context.Background(), "gmail.get", params)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if out["format"] != "metadata" {
		t.Fatalf("expected metadata format")
	}
}

func TestRewriteGmailGetDropsHeaders(t *testing.T) {
	p := &Policy{AllowedActions: []string{"gmail.get"}, Gmail: &GmailPolicy{}}
	if err := p.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	params := map[string]interface{}{"message_id": "msg123", "headers": "From,To"}
	out, warnings, err := p.ValidateAndRewrite(context.Background(), "gmail.get", params)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if _, ok := out["headers"]; ok {
		t.Fatalf("expected headers to be removed")
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warnings")
	}
}

func TestRewriteGmailSendDraftOnlyRejectsThreadID(t *testing.T) {
	p := &Policy{AllowedActions: []string{"gmail.send"}, Gmail: &GmailPolicy{DraftOnly: true}}
	if err := p.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	params := map[string]interface{}{"to": "a@b.com", "subject": "hi", "body": "yo", "thread_id": "t1"}
	_, _, err := p.ValidateAndRewrite(context.Background(), "gmail.send", params)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestRewriteGmailSendRejectsAttachmentsByDefault(t *testing.T) {
	p := &Policy{AllowedActions: []string{"gmail.send"}, Gmail: &GmailPolicy{}}
	if err := p.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	params := map[string]interface{}{"to": "a@b.com", "subject": "hi", "body": "yo", "attach": []interface{}{"file.txt"}}
	_, _, err := p.ValidateAndRewrite(context.Background(), "gmail.send", params)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestRewriteGmailSendAllowlistDraftsUnknownRecipients(t *testing.T) {
	p := &Policy{AllowedActions: []string{"gmail.send"}, Gmail: &GmailPolicy{AllowedSendRecipients: []string{"allowed@example.com"}}}
	if err := p.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	params := map[string]interface{}{"to": "other@example.com", "subject": "hi", "body": "yo"}
	_, warnings, err := p.ValidateAndRewrite(context.Background(), "gmail.send", params)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warnings")
	}
}

func TestRewriteGmailLabelsGetAllowsMappedName(t *testing.T) {
	p := &Policy{AllowedActions: []string{"gmail.labels.get"}, Gmail: &GmailPolicy{AllowedLabels: []string{"Label_123"}}}
	if err := p.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	p.SetLabelMap(map[string]string{"Label_123": "My Label"})
	params := map[string]interface{}{"label": "My Label"}
	_, _, err := p.ValidateAndRewrite(context.Background(), "gmail.labels.get", params)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
}

func TestRewriteGmailLabelsModifyRejectsDisallowed(t *testing.T) {
	p := &Policy{AllowedActions: []string{"gmail.labels.modify"}, Gmail: &GmailPolicy{AllowedLabels: []string{"Label_123"}}}
	if err := p.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	params := map[string]interface{}{"thread_ids": []string{"t1"}, "add": "Other"}
	_, _, err := p.ValidateAndRewrite(context.Background(), "gmail.labels.modify", params)
	if err == nil {
		t.Fatalf("expected error")
	}
}
