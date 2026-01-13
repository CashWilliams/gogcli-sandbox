package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"gogcli-sandbox/internal/config"
)

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	parts := strings.Split(value, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			*s = append(*s, part)
		}
	}
	return nil
}

type policy struct {
	AllowedActions []string        `json:"allowed_actions"`
	Gmail          *gmailPolicy    `json:"gmail,omitempty"`
	Calendar       *calendarPolicy `json:"calendar,omitempty"`
}

type gmailPolicy struct {
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

type calendarPolicy struct {
	AllowedCalendars []string `json:"allowed_calendars"`
	AllowDetails     bool     `json:"allow_details"`
	MaxDays          int      `json:"max_days"`
}

func main() {
	var readLabels stringList
	var addLabels stringList
	var removeLabels stringList
	var calendars stringList
	var senders stringList
	var sendRecipients stringList
	var outPath string
	var stdout bool
	var writeConfig bool
	var configOut string
	var includeThreadGet bool
	var allowSend bool
	var draftOnly bool
	var allowAttachments bool
	var maxGmailDays int
	var maxCalendarDays int
	var account string

	flag.Var(&readLabels, "label", "Allowed Gmail read label ID/name (repeat or comma-separated). Default: INBOX")
	flag.Var(&readLabels, "read-label", "Allowed Gmail read label ID/name (repeat or comma-separated). Default: INBOX")
	flag.Var(&addLabels, "add-label", "Allowed Gmail label ID/name to add (repeat or comma-separated). Optional")
	flag.Var(&removeLabels, "remove-label", "Allowed Gmail label ID/name to remove (repeat or comma-separated). Optional")
	flag.Var(&calendars, "calendar", "Allowed calendar ID (repeat or comma-separated). Default: primary")
	flag.Var(&senders, "sender", "Allowed sender domain (repeat or comma-separated). Optional")
	flag.Var(&sendRecipients, "allow-send-recipient", "Allowed email address for direct send (repeat or comma-separated). Optional")
	flag.BoolVar(&includeThreadGet, "include-thread-get", false, "Include gmail.thread.get in allowed actions")
	flag.BoolVar(&allowSend, "allow-send", false, "Include gmail.send in allowed actions")
	flag.BoolVar(&draftOnly, "draft-only", true, "When true, gmail.send always creates drafts instead of sending")
	flag.BoolVar(&allowAttachments, "allow-attachments", false, "Allow gmail.send/gmail.drafts.create to attach files")
	flag.IntVar(&maxGmailDays, "max-gmail-days", 7, "Max Gmail query window in days")
	flag.IntVar(&maxCalendarDays, "max-calendar-days", 7, "Max calendar query window in days")
	flag.StringVar(&account, "account", "", "Account email for multi-account policy output")
	flag.StringVar(&outPath, "out", "", "Write policy to file path (default: $XDG_CONFIG_HOME/gogcli-sandbox/policy.json)")
	flag.BoolVar(&stdout, "stdout", false, "Write policy to stdout instead of a file")
	flag.BoolVar(&writeConfig, "write-config", true, "Write config file to default path")
	flag.StringVar(&configOut, "config-out", "", "Write config file to path (default: $XDG_CONFIG_HOME/gogcli-sandbox/config.json)")
	flag.Parse()

	if stdout && outPath != "" {
		fmt.Fprintln(os.Stderr, "use only one of --stdout or --out")
		os.Exit(1)
	}
	account = strings.TrimSpace(account)
	if account == "" {
		fmt.Fprintln(os.Stderr, "--account is required for policy output")
		os.Exit(1)
	}

	if len(readLabels) == 0 {
		readLabels = append(readLabels, "INBOX")
	}
	if len(calendars) == 0 {
		calendars = append(calendars, "primary")
	}

	actions := []string{
		"policy.actions",
		"gmail.search",
		"gmail.thread.list",
		"gmail.get",
		"calendar.list",
		"calendar.events",
		"calendar.freebusy",
	}
	if includeThreadGet {
		actions = append(actions, "gmail.thread.get")
	}
	if allowSend {
		actions = append(actions, "gmail.send")
	}
	sort.Strings(actions)

	pol := policy{
		AllowedActions: actions,
		Gmail: &gmailPolicy{
			AllowedReadLabels:     readLabels,
			AllowedAddLabels:      addLabels,
			AllowedRemoveLabels:   removeLabels,
			AllowedSenders:        senders,
			AllowedSendRecipients: sendRecipients,
			MaxDays:               maxGmailDays,
			AllowBody:             false,
			AllowLinks:            false,
			DraftOnly:             draftOnly,
			AllowAttachments:      allowAttachments,
		},
		Calendar: &calendarPolicy{
			AllowedCalendars: calendars,
			AllowDetails:     false,
			MaxDays:          maxCalendarDays,
		},
	}

	var err error
	type policySet struct {
		DefaultAccount string            `json:"default_account,omitempty"`
		Accounts       map[string]policy `json:"accounts"`
	}
	set := policySet{
		DefaultAccount: account,
		Accounts:       map[string]policy{account: pol},
	}
	payload, err := json.MarshalIndent(set, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode policy: %v\n", err)
		os.Exit(1)
	}
	payload = append(payload, '\n')

	if stdout {
		_, _ = os.Stdout.Write(payload)
	} else {
		if outPath == "" {
			defaultPath, err := config.DefaultPolicyPath()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to resolve default policy path: %v\n", err)
				os.Exit(1)
			}
			outPath = defaultPath
		}
		if err := config.EnsurePolicyDir(outPath); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create policy dir: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(outPath, payload, 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write policy: %v\n", err)
			os.Exit(1)
		}
	}

	if !writeConfig || (stdout && configOut == "") {
		return
	}

	policyPathUsed := outPath
	if policyPathUsed == "" {
		defaultPolicy, err := config.DefaultPolicyPath()
		if err == nil {
			policyPathUsed = defaultPolicy
		}
	}

	if configOut == "" {
		defaultConfig, err := config.DefaultConfigPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to resolve default config path: %v\n", err)
			os.Exit(1)
		}
		configOut = defaultConfig
	}

	fileCfg := config.DefaultFileConfig()
	if policyPathUsed != "" {
		fileCfg.Policy = policyPathUsed
	}
	if strings.TrimSpace(account) != "" {
		fileCfg.GogAccount = account
	}
	if err := config.WriteFile(configOut, fileCfg); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write config file: %v\n", err)
		os.Exit(1)
	}
}
