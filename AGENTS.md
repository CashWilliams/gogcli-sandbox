# Agent Notes

## Mission
Implement a secure wrapper around `gogcli` that enforces a strict policy allowlist. The broker is the only interface available to agents.

## Guardrails
- Do not expand capabilities beyond the allowed actions in the policy.
- All `gog` invocations must include `--json --no-input`.
- Never return message bodies, attachments, or links unless policy explicitly allows it.
- Preserve audit logging (request id, action, allow/deny, duration).
- Gmail label restrictions are enforced via label IDs (e.g. `Label_123...`, `INBOX`).
- When `gmail.draft_only` is true, `gmail.send` is converted to draft creation.
- Default config path uses XDG: `$XDG_CONFIG_HOME/gogcli-sandbox/config.json`.
- Default policy path uses XDG: `$XDG_CONFIG_HOME/gogcli-sandbox/policy.json`.

## Quick Commands
- Build: `go build ./cmd/broker`
- Test: `go test ./...`
- Bootstrap policy: `go build -o gogcli-sandbox-init ./cmd/bootstrap`
- Client CLI: `go build -o gogcli-sandbox-client ./cmd/client`

## Key Paths
- Policy: `internal/policy`
- gog runner: `internal/gog`
- Redaction: `internal/redact`
- Server: `internal/server`
- Systemd: `deploy/systemd`
