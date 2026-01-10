# gogcli sandbox

> [!WARNING]
> This project is very early in development and not fully tested. Use at your own risk.

A small Go broker that exposes a strict, policy-enforced API to (gogcli)[https://github.com/steipete/gogcli] for AI agents. The broker is meant to run as a separate OS user and is the only interface available to the agent.

The idea is to remove direct access to `gog` from the agent, and provide a level of projection from LLM hallucinations and prompt injections. Setting up `gog` as either a separate user, or on a separate machine, stops the agent from being able to access to the oauth credentials.

## Quickstart

1. Create a policy + config file (see `configs/policy.sample.json` and `configs/config.sample.json`) or bootstrap them to the default XDG config location:

```sh
go build -o gogcli-sandbox-init ./cmd/bootstrap
./gogcli-sandbox-init
```

2. Default config paths (for the service user, e.g. `gogd`):

- Config: `$XDG_CONFIG_HOME/gogcli-sandbox/config.json` (fallback: `~/.config/gogcli-sandbox/config.json`)
- Policy: `$XDG_CONFIG_HOME/gogcli-sandbox/policy.json` (fallback: `~/.config/gogcli-sandbox/policy.json`)

3. Use Gmail label IDs (e.g. `INBOX` or `Label_123...`) and calendar IDs (e.g. `primary`). Update the policy as needed.
4. Build and run the broker (config is auto-loaded; flags override config):

```sh
go build -o gogcli-sandbox ./cmd/broker
./gogcli-sandbox
```

5. Build the client CLI (for agents):

```sh
go build -o gogcli-sandbox-client ./cmd/client
```

6. Send a request:

```sh
./gogcli-sandbox-client gmail.search --query "label:Label_1234567890 newer_than:7d" --max 5
```

## API

- `POST /v1/request`
- `GET /healthz`

### Request

```json
{
  "id": "uuid",
  "action": "gmail.search",
  "params": {
    "query": "label:Label_1234567890 newer_than:7d",
    "max": 20
  }
}
```

### Response

```json
{
  "id": "uuid",
  "ok": true,
  "data": {"...": "..."},
  "warnings": ["redacted:links"],
  "error": null
}
```

## Notes

- The broker refuses to start if the policy file is missing or invalid.
- Denied actions return `ok: false` with a structured error.
- All responses are redacted according to policy.
- `gmail.get` always uses gogcli defaults for metadata headers (custom headers are ignored).
- `gmail.send` can be forced to create drafts only via `gmail.draft_only`.
- If `gmail.allowed_send_recipients` is set, `gmail.send` will only send directly when **all** recipients are in that allowlist; otherwise it creates a draft.
- For `gmail.search` results, label filtering happens **after** the query; the broker resolves label IDs to names via `gog gmail labels list`.
- Config is loaded from `$XDG_CONFIG_HOME/gogcli-sandbox/config.json` if present (or `~/.config/gogcli-sandbox/config.json`).

## Config file

`config.json` fields (all optional; flags override):

```json
{
  "socket": "/run/gogcli-sandbox.sock",
  "policy": "/home/<user>/.config/gogcli-sandbox/policy.json",
  "gog_path": "gog",
  "gog_account": "",
  "timeout": "30s",
  "log_json": true,
  "verbose": false
}
```

Tip: for non-root usage, set `socket` to a user-writable path such as `$XDG_RUNTIME_DIR/gogcli-sandbox.sock` or `/tmp/gogcli-sandbox.sock`.

Override config path:

```sh
./gogcli-sandbox --config /etc/gogcli-sandbox/config.json
```

Enable verbose logging (metadata only):

```sh
./gogcli-sandbox --verbose
```

## Getting label + calendar IDs

Use the helper script (requires `gog` and `python` in PATH):

```sh
scripts/print-ids.sh
```

Or manually:

```sh
gog gmail labels list --json
gog calendar calendars --json
```

## Client CLI usage

The client is the interface exposed to agents. It prints JSON responses and exits non‑zero on errors.

```sh
./gogcli-sandbox-client help
./gogcli-sandbox-client gmail.search --query "label:INBOX newer_than:7d" --max 10
./gogcli-sandbox-client gmail.get --id <messageId>
./gogcli-sandbox-client gmail.send --to user@example.com --subject "Hello" --body "Hi"
./gogcli-sandbox-client calendar.list --max 5
./gogcli-sandbox-client calendar.events --calendar-id primary --days 7
./gogcli-sandbox-client calendar.freebusy --calendar-id primary --from 2025-01-01T00:00:00Z --to 2025-01-02T00:00:00Z
```

Override socket path:

```sh
./gogcli-sandbox-client --socket /run/gogcli-sandbox.sock gmail.search --query "label:INBOX newer_than:7d"
```

Calendar time flags (client supports gog-style relative dates):

```sh
./gogcli-sandbox-client calendar.events --calendar-id primary --today
./gogcli-sandbox-client calendar.events --calendar-id primary --week
./gogcli-sandbox-client calendar.events --calendar-id primary --from monday --to friday
```

## Bootstrap options

The bootstrap tool is intentionally strict by default:

- Only read-only actions are enabled.
- Gmail and calendar windows default to 7 days.
- Body/details/links are disabled.

Override defaults as needed, for example:

```sh
./gogcli-sandbox-init --label Label_123 --calendar primary --include-thread-get --max-calendar-days 14
```

Bootstrap also writes `config.json` by default. Disable with:

```sh
./gogcli-sandbox-init --write-config=false
```

Enable send in draft-only mode (recommended):

```sh
./gogcli-sandbox-init --allow-send --draft-only
```

Allow direct send only to specific recipients:

```sh
./gogcli-sandbox-init --allow-send --allow-send-recipient alice@example.com --allow-send-recipient bob@example.com --draft-only=false
```

Allow direct send (less restrictive):

```sh
./gogcli-sandbox-init --allow-send --draft-only=false
```

Preview a policy without writing a file:

```sh
./gogcli-sandbox-init --stdout
```

Write config to a custom location:

```sh
./gogcli-sandbox-init --config-out /etc/gogcli-sandbox/config.json
```

## Repo layout

- `cmd/broker`: broker entry point
- `internal/policy`: policy parsing and validation
- `internal/gog`: gogcli command runner
- `internal/redact`: response filtering and redaction
- `internal/server`: Unix socket HTTP server
- `deploy/systemd`: example systemd unit

## Testing

```sh
go test ./...
```

## Disclaimer

⚠️ THIS SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND.

This tool is meant to be a security mitigation only, and not a guarantee of safety.

- Do not rely solely on this tool for security decisions in production environments.
- The authors are not responsible for any damage caused.
- Use at your own risk.
