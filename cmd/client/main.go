package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"gogcli-sandbox/internal/types"
)

const defaultSocket = "/run/gogcli-sandbox.sock"

func main() {
	cfg, args, err := parseGlobal(os.Args[1:])
	if err != nil {
		fatal(err)
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printUsage("")
		return
	}

	cmd := args[0]
	cmdArgs := args[1:]

	action, params, err := parseCommand(cmd, cmdArgs)
	if err != nil {
		if errors.Is(err, errHelp) {
			return
		}
		fatal(err)
	}

	if cfg.ID == "" {
		cfg.ID, err = newID()
		if err != nil {
			fatal(err)
		}
	}

	resp, raw, err := doRequest(cfg, action, params)
	if err != nil {
		fatal(err)
	}

	writeResponse(cfg, resp, raw)
	if resp != nil && !resp.Ok {
		os.Exit(1)
	}
}

var errHelp = errors.New("help requested")

type config struct {
	Socket  string
	Account string
	Timeout time.Duration
	Pretty  bool
	ID      string
}

func parseGlobal(args []string) (config, []string, error) {
	cfg := config{}
	defaultSock := os.Getenv("GOGCLI_SANDBOX_SOCKET")
	if defaultSock == "" {
		defaultSock = defaultSocket
	}
	defaultAccount := os.Getenv("GOGCLI_SANDBOX_ACCOUNT")
	fs := flag.NewFlagSet("gogcli-sandbox", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.Socket, "socket", defaultSock, "unix socket path")
	fs.StringVar(&cfg.Account, "account", defaultAccount, "gog account (optional)")
	fs.DurationVar(&cfg.Timeout, "timeout", 15*time.Second, "request timeout")
	fs.BoolVar(&cfg.Pretty, "pretty", false, "pretty-print JSON output")
	fs.StringVar(&cfg.ID, "id", "", "request id (optional)")
	if err := fs.Parse(args); err != nil {
		return config{}, nil, err
	}
	return cfg, fs.Args(), nil
}

func parseCommand(cmd string, args []string) (string, map[string]interface{}, error) {
	switch cmd {
	case "gmail.search":
		return parseGmailSearch(args)
	case "gmail.thread.get":
		return parseGmailThreadGet(args)
	case "gmail.thread.modify":
		return parseGmailThreadModify(args)
	case "gmail.get":
		return parseGmailGet(args)
	case "gmail.send":
		return parseGmailSend(args)
	case "gmail.labels.list":
		return parseGmailLabelsList(args)
	case "gmail.labels.get", "gmail.lables.get":
		return parseGmailLabelsGet(args)
	case "gmail.labels.modify":
		return parseGmailLabelsModify(args)
	case "policy.actions":
		return parsePolicyActions(args)
	case "calendar.list":
		return parseCalendarList(args)
	case "calendar.events":
		return parseCalendarEvents(args)
	case "calendar.freebusy":
		return parseCalendarFreebusy(args)
	case "help":
		printUsage("")
		return "", nil, errHelp
	case "help.gmail", "gmail.help":
		printUsage("gmail")
		return "", nil, errHelp
	case "help.calendar", "calendar.help":
		printUsage("calendar")
		return "", nil, errHelp
	case "help.policy", "policy.help":
		printUsage("policy")
		return "", nil, errHelp
	default:
		return "", nil, fmt.Errorf("unknown command: %s", cmd)
	}
}

func parseGmailSearch(args []string) (string, map[string]interface{}, error) {
	fs := flag.NewFlagSet("gmail.search", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	query := fs.String("query", "", "Gmail search query (required)")
	max := fs.Int("max", 0, "max results")
	page := fs.String("page", "", "page token")
	oldest := fs.Bool("oldest", false, "show oldest message date")
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}
	if *query == "" && fs.NArg() > 0 {
		*query = strings.Join(fs.Args(), " ")
	}
	if strings.TrimSpace(*query) == "" {
		return "", nil, fmt.Errorf("--query is required")
	}
	params := map[string]interface{}{"query": *query}
	if *max > 0 {
		params["max"] = *max
	}
	if *page != "" {
		params["page"] = *page
	}
	if *oldest {
		params["oldest"] = true
	}
	return "gmail.search", params, nil
}

func parseGmailThreadGet(args []string) (string, map[string]interface{}, error) {
	fs := flag.NewFlagSet("gmail.thread.get", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	id := fs.String("id", "", "thread id (required)")
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}
	if *id == "" && fs.NArg() > 0 {
		*id = fs.Arg(0)
	}
	if strings.TrimSpace(*id) == "" {
		return "", nil, fmt.Errorf("--id is required")
	}
	return "gmail.thread.get", map[string]interface{}{"thread_id": *id}, nil
}

func parseGmailThreadModify(args []string) (string, map[string]interface{}, error) {
	fs := flag.NewFlagSet("gmail.thread.modify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	id := fs.String("id", "", "thread id (required)")
	add := fs.String("add", "", "labels to add (comma-separated)")
	remove := fs.String("remove", "", "labels to remove (comma-separated)")
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}
	if *id == "" && fs.NArg() > 0 {
		*id = fs.Arg(0)
	}
	if strings.TrimSpace(*id) == "" {
		return "", nil, fmt.Errorf("--id is required")
	}
	if strings.TrimSpace(*add) == "" && strings.TrimSpace(*remove) == "" {
		return "", nil, fmt.Errorf("--add or --remove is required")
	}
	params := map[string]interface{}{"thread_id": *id}
	if strings.TrimSpace(*add) != "" {
		params["add"] = *add
	}
	if strings.TrimSpace(*remove) != "" {
		params["remove"] = *remove
	}
	return "gmail.thread.modify", params, nil
}

func parseGmailGet(args []string) (string, map[string]interface{}, error) {
	fs := flag.NewFlagSet("gmail.get", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	id := fs.String("id", "", "message id (required)")
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}
	if *id == "" && fs.NArg() > 0 {
		*id = fs.Arg(0)
	}
	if strings.TrimSpace(*id) == "" {
		return "", nil, fmt.Errorf("--id is required")
	}
	return "gmail.get", map[string]interface{}{"message_id": *id}, nil
}

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

func parseGmailSend(args []string) (string, map[string]interface{}, error) {
	fs := flag.NewFlagSet("gmail.send", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	to := fs.String("to", "", "recipients (comma-separated)")
	cc := fs.String("cc", "", "cc recipients")
	bcc := fs.String("bcc", "", "bcc recipients")
	subject := fs.String("subject", "", "subject")
	body := fs.String("body", "", "body (plain)")
	bodyHTML := fs.String("body-html", "", "body (HTML)")
	replyToMessageID := fs.String("reply-to-message-id", "", "reply to Gmail message ID")
	threadID := fs.String("thread-id", "", "reply within a thread")
	replyAll := fs.Bool("reply-all", false, "reply all")
	replyTo := fs.String("reply-to", "", "reply-to header")
	from := fs.String("from", "", "send-as address")
	track := fs.Bool("track", false, "enable tracking")
	trackSplit := fs.Bool("track-split", false, "send tracked messages separately")
	var attach stringList
	fs.Var(&attach, "attach", "attachment file path (repeatable)")
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}

	params := map[string]interface{}{}
	if *to != "" {
		params["to"] = *to
	}
	if *cc != "" {
		params["cc"] = *cc
	}
	if *bcc != "" {
		params["bcc"] = *bcc
	}
	if *subject != "" {
		params["subject"] = *subject
	}
	if *body != "" {
		params["body"] = *body
	}
	if *bodyHTML != "" {
		params["body_html"] = *bodyHTML
	}
	if *replyToMessageID != "" {
		params["reply_to_message_id"] = *replyToMessageID
	}
	if *threadID != "" {
		params["thread_id"] = *threadID
	}
	if *replyAll {
		params["reply_all"] = true
	}
	if *replyTo != "" {
		params["reply_to"] = *replyTo
	}
	if *from != "" {
		params["from"] = *from
	}
	if *track {
		params["track"] = true
	}
	if *trackSplit {
		params["track_split"] = true
	}
	if len(attach) > 0 {
		params["attach"] = []string(attach)
	}
	return "gmail.send", params, nil
}

func parseGmailLabelsList(args []string) (string, map[string]interface{}, error) {
	fs := flag.NewFlagSet("gmail.labels.list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}
	return "gmail.labels.list", map[string]interface{}{}, nil
}

func parseGmailLabelsGet(args []string) (string, map[string]interface{}, error) {
	fs := flag.NewFlagSet("gmail.labels.get", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	label := fs.String("label", "", "label id or name (required)")
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}
	if *label == "" && fs.NArg() > 0 {
		*label = fs.Arg(0)
	}
	if strings.TrimSpace(*label) == "" {
		return "", nil, fmt.Errorf("--label is required")
	}
	return "gmail.labels.get", map[string]interface{}{"label": *label}, nil
}

func parseGmailLabelsModify(args []string) (string, map[string]interface{}, error) {
	fs := flag.NewFlagSet("gmail.labels.modify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var threadIDs stringList
	fs.Var(&threadIDs, "thread-id", "thread id (repeatable)")
	add := fs.String("add", "", "labels to add (comma-separated)")
	remove := fs.String("remove", "", "labels to remove (comma-separated)")
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}
	ids := append([]string{}, threadIDs...)
	ids = append(ids, fs.Args()...)
	if len(ids) == 0 {
		return "", nil, fmt.Errorf("--thread-id or positional thread ids are required")
	}
	if strings.TrimSpace(*add) == "" && strings.TrimSpace(*remove) == "" {
		return "", nil, fmt.Errorf("--add or --remove is required")
	}
	params := map[string]interface{}{"thread_ids": []string(ids)}
	if strings.TrimSpace(*add) != "" {
		params["add"] = *add
	}
	if strings.TrimSpace(*remove) != "" {
		params["remove"] = *remove
	}
	return "gmail.labels.modify", params, nil
}

func parseCalendarList(args []string) (string, map[string]interface{}, error) {
	fs := flag.NewFlagSet("calendar.list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	max := fs.Int("max", 0, "max results")
	page := fs.String("page", "", "page token")
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}
	params := map[string]interface{}{}
	if *max > 0 {
		params["max"] = *max
	}
	if *page != "" {
		params["page"] = *page
	}
	return "calendar.list", params, nil
}

func parseCalendarEvents(args []string) (string, map[string]interface{}, error) {
	fs := flag.NewFlagSet("calendar.events", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	calendarID := fs.String("calendar-id", "", "calendar id (required)")
	from := fs.String("from", "", "start time (RFC3339)")
	to := fs.String("to", "", "end time (RFC3339)")
	timeMin := fs.String("time-min", "", "start time (RFC3339)")
	timeMax := fs.String("time-max", "", "end time (RFC3339)")
	today := fs.Bool("today", false, "today only")
	tomorrow := fs.Bool("tomorrow", false, "tomorrow only")
	week := fs.Bool("week", false, "this week")
	days := fs.Int("days", 0, "next N days")
	weekStart := fs.String("week-start", "", "week start day (sun, mon, ...)")
	max := fs.Int("max", 0, "max results")
	page := fs.String("page", "", "page token")
	query := fs.String("query", "", "search query")
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}

	if *from == "" {
		*from = *timeMin
	}
	if *to == "" {
		*to = *timeMax
	}
	if strings.TrimSpace(*calendarID) == "" {
		return "", nil, fmt.Errorf("--calendar-id is required")
	}

	params := map[string]interface{}{
		"calendar_id": *calendarID,
	}
	if strings.TrimSpace(*from) != "" {
		params["from"] = *from
	}
	if strings.TrimSpace(*to) != "" {
		params["to"] = *to
	}
	if *today {
		params["today"] = true
	}
	if *tomorrow {
		params["tomorrow"] = true
	}
	if *week {
		params["week"] = true
	}
	if *days > 0 {
		params["days"] = *days
	}
	if *weekStart != "" {
		params["week_start"] = *weekStart
	}
	if *max > 0 {
		params["max"] = *max
	}
	if *page != "" {
		params["page"] = *page
	}
	if *query != "" {
		params["query"] = *query
	}
	return "calendar.events", params, nil
}

func parseCalendarFreebusy(args []string) (string, map[string]interface{}, error) {
	fs := flag.NewFlagSet("calendar.freebusy", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var calendarIDs stringList
	var calendarID stringList
	from := fs.String("from", "", "start time (RFC3339)")
	to := fs.String("to", "", "end time (RFC3339)")
	timeMin := fs.String("time-min", "", "start time (RFC3339)")
	timeMax := fs.String("time-max", "", "end time (RFC3339)")
	fs.Var(&calendarIDs, "calendar-ids", "calendar IDs (comma-separated)")
	fs.Var(&calendarID, "calendar-id", "calendar ID (repeatable)")
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}

	if *from == "" {
		*from = *timeMin
	}
	if *to == "" {
		*to = *timeMax
	}
	ids := append([]string{}, calendarIDs...)
	ids = append(ids, calendarID...)
	if len(ids) == 0 {
		return "", nil, fmt.Errorf("--calendar-id or --calendar-ids is required")
	}
	if strings.TrimSpace(*from) == "" || strings.TrimSpace(*to) == "" {
		return "", nil, fmt.Errorf("--from and --to are required")
	}
	params := map[string]interface{}{
		"calendar_ids": []string(ids),
		"time_min":     *from,
		"time_max":     *to,
	}
	return "calendar.freebusy", params, nil
}

func parsePolicyActions(args []string) (string, map[string]interface{}, error) {
	fs := flag.NewFlagSet("policy.actions", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}
	if fs.NArg() > 0 {
		return "", nil, fmt.Errorf("policy.actions does not accept arguments")
	}
	return "policy.actions", map[string]interface{}{}, nil
}

func doRequest(cfg config, action string, params map[string]interface{}) (*types.Response, []byte, error) {
	reqPayload := &types.Request{ID: cfg.ID, Action: action, Account: cfg.Account, Params: params}
	body, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", cfg.Socket)
			},
		},
	}
	url := "http://unix/v1/request"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	var parsed types.Response
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, raw, fmt.Errorf("invalid response json: %w", err)
	}
	return &parsed, raw, nil
}

func writeResponse(cfg config, resp *types.Response, raw []byte) {
	if resp == nil {
		return
	}
	if cfg.Pretty {
		pretty, err := json.MarshalIndent(resp, "", "  ")
		if err == nil {
			fmt.Println(string(pretty))
		} else {
			fmt.Println(string(raw))
		}
	} else {
		fmt.Println(string(raw))
	}

	if !resp.Ok && resp.Error != nil {
		fmt.Fprintf(os.Stderr, "error: %s: %s\n", resp.Error.Code, resp.Error.Message)
		if resp.Error.Details != "" {
			fmt.Fprintf(os.Stderr, "details: %s\n", resp.Error.Details)
		}
	}
	if len(resp.Warnings) > 0 {
		fmt.Fprintf(os.Stderr, "warnings: %s\n", strings.Join(resp.Warnings, ", "))
	}
}

func newID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(2)
}

func printUsage(section string) {
	switch section {
	case "gmail":
		fmt.Println("gmail commands:")
		fmt.Println("  gmail.search        Search threads")
		fmt.Println("  gmail.thread.get    Get a thread (metadata)")
		fmt.Println("  gmail.thread.modify Modify labels on a thread")
		fmt.Println("  gmail.get           Get a message (metadata)")
		fmt.Println("  gmail.send          Send or draft an email (policy controlled)")
		fmt.Println("  gmail.labels.list   List labels")
		fmt.Println("  gmail.labels.get    Get label details")
		fmt.Println("  gmail.labels.modify Modify labels on multiple threads")
		return
	case "calendar":
		fmt.Println("calendar commands:")
		fmt.Println("  calendar.list       List calendars")
		fmt.Println("  calendar.events     List events from a calendar")
		fmt.Println("  calendar.freebusy   Get free/busy blocks")
		return
	case "policy":
		fmt.Println("policy commands:")
		fmt.Println("  policy.actions      List allowed actions")
		return
	}

	fmt.Println("Usage:")
	fmt.Println("  gogcli-sandbox-client [global flags] <command> [command flags]")
	fmt.Println("")
	fmt.Println("Global flags:")
	fmt.Println("  --socket PATH     unix socket path (default: /run/gogcli-sandbox.sock)")
	fmt.Println("  --account EMAIL   gog account email (optional; env: GOGCLI_SANDBOX_ACCOUNT)")
	fmt.Println("  --timeout DUR     request timeout (default: 15s)")
	fmt.Println("  --pretty          pretty-print JSON output")
	fmt.Println("  --id ID           request id (optional)")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  gmail.search")
	fmt.Println("  gmail.thread.get")
	fmt.Println("  gmail.thread.modify")
	fmt.Println("  gmail.get")
	fmt.Println("  gmail.send")
	fmt.Println("  gmail.labels.list")
	fmt.Println("  gmail.labels.get")
	fmt.Println("  gmail.labels.modify")
	fmt.Println("  calendar.list")
	fmt.Println("  calendar.events")
	fmt.Println("  calendar.freebusy")
	fmt.Println("  policy.actions")
	fmt.Println("  policy.actions")
	fmt.Println("")
	fmt.Println("Help:")
	fmt.Println("  gogcli-sandbox-client help")
	fmt.Println("  gogcli-sandbox-client help.gmail")
	fmt.Println("  gogcli-sandbox-client help.calendar")
	fmt.Println("  gogcli-sandbox-client help.policy")
}
