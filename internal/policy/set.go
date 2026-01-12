package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

var (
	ErrAccountRequired   = errors.New("account is required")
	ErrAccountNotAllowed = errors.New("account not allowed")
)

type PolicySet struct {
	DefaultAccount string             `json:"default_account,omitempty"`
	Accounts       map[string]*Policy `json:"accounts,omitempty"`
}

func LoadSet(path string) (*PolicySet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var set PolicySet
	if err := json.Unmarshal(data, &set); err != nil {
		return nil, err
	}
	if len(set.Accounts) == 0 {
		return nil, errors.New("accounts must not be empty")
	}

	normalized := map[string]*Policy{}
	for key, pol := range set.Accounts {
		account := normalizeAccount(key)
		if account == "" {
			return nil, errors.New("accounts contains empty key")
		}
		if pol == nil {
			return nil, fmt.Errorf("account %s policy is null", account)
		}
		if _, exists := normalized[account]; exists {
			return nil, fmt.Errorf("duplicate account %s", account)
		}
		if err := pol.Validate(); err != nil {
			return nil, fmt.Errorf("account %s: %w", account, err)
		}
		normalized[account] = pol
	}
	set.Accounts = normalized

	if set.DefaultAccount != "" {
		set.DefaultAccount = normalizeAccount(set.DefaultAccount)
		if set.DefaultAccount == "" {
			return nil, errors.New("default_account is empty")
		}
		if _, ok := set.Accounts[set.DefaultAccount]; !ok {
			return nil, fmt.Errorf("default_account %s not found", set.DefaultAccount)
		}
	}

	return &set, nil
}

func (s *PolicySet) Resolve(account string, fallback string) (*Policy, string, error) {
	if s == nil {
		return nil, "", errors.New("policy is required")
	}

	normalized := normalizeAccount(account)

	if normalized == "" {
		if s.DefaultAccount != "" {
			normalized = s.DefaultAccount
		} else if fb := normalizeAccount(fallback); fb != "" {
			normalized = fb
		} else if len(s.Accounts) == 1 {
			for key := range s.Accounts {
				normalized = key
				break
			}
		}
	}

	if normalized == "" {
		return nil, "", ErrAccountRequired
	}

	pol, ok := s.Accounts[normalized]
	if !ok {
		return nil, "", ErrAccountNotAllowed
	}
	return pol, normalized, nil
}

func normalizeAccount(account string) string {
	return strings.ToLower(strings.TrimSpace(account))
}
