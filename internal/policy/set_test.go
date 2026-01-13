package policy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSetAccountsResolve(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	data := []byte(`{
  "default_account": "User@Example.com",
  "accounts": {
    "user@example.com": {
      "allowed_actions": ["gmail.search"],
      "gmail": { "allowed_read_labels": ["INBOX"] }
    }
  }
}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	set, err := LoadSet(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	pol, account, err := set.Resolve("", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if pol == nil {
		t.Fatalf("expected policy")
	}
	if account != "user@example.com" {
		t.Fatalf("unexpected account: %s", account)
	}
}

func TestResolveRequiresAccount(t *testing.T) {
	set := &PolicySet{Accounts: map[string]*Policy{
		"a@example.com": {AllowedActions: []string{"gmail.search"}, Gmail: &GmailPolicy{AllowedReadLabels: []string{"INBOX"}}},
		"b@example.com": {AllowedActions: []string{"gmail.search"}, Gmail: &GmailPolicy{AllowedReadLabels: []string{"INBOX"}}},
	}}
	for _, pol := range set.Accounts {
		if err := pol.Validate(); err != nil {
			t.Fatalf("validate: %v", err)
		}
	}
	_, _, err := set.Resolve("", "")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestResolveSingleAccountFallback(t *testing.T) {
	set := &PolicySet{Accounts: map[string]*Policy{"a@example.com": {AllowedActions: []string{"gmail.search"}, Gmail: &GmailPolicy{AllowedReadLabels: []string{"INBOX"}}}}}
	if err := set.Accounts["a@example.com"].Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	_, _, err := set.Resolve("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
