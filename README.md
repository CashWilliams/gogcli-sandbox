# gogcli sandbox

> [!WARNING]
> This project is very early in development and not fully tested. Use at your own risk.

A small Go broker that exposes a strict, policy-enforced API to [gogcli](https://github.com/steipete/gogcli) for AI agents. The broker is meant to run as a separate OS user and is the only interface available to the agent.

The idea is to remove direct access to `gog` from the agent, and provide a layer of protection against hallucinations and prompt injections. Running the broker as a separate user (or on a separate machine) prevents agents from accessing OAuth credentials.

## Docs

- Installation & setup: `docs/install.md`

## Key behaviors

- The broker refuses to start if the policy file is missing or invalid.
- Denied actions return `ok: false` with a structured error.
- All responses are redacted according to policy.
- `gmail.send` can be forced into draft-only mode, with allowlisted recipients.
- Gmail label filtering happens **after** the query to avoid false negatives.

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
