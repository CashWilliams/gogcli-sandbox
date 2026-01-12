# Installation & setup

This guide covers installing `gogcli-sandbox`, bootstrapping config, and running the broker safely.

## Install

### Homebrew (macOS/Linux)

```sh
brew tap cashwilliams/tap
brew install gogcli-sandbox
```

This installs:
- `gogcli-sandbox` (broker)
- `gogcli-sandbox-client` (client CLI)
- `gogcli-sandbox-init` (bootstrap tool)

### Build from source

```sh
go build -o gogcli-sandbox ./cmd/broker
go build -o gogcli-sandbox-client ./cmd/client
go build -o gogcli-sandbox-init ./cmd/bootstrap
```

## Prerequisites

- `gogcli` installed and authorized with Google (see [gogcli](https://github.com/steipete/gogcli)).
- The broker should run as a **separate OS user** from the agent.

## Bootstrap config + policy

Default XDG paths (for the service user, e.g. `gogd`):

- Config: `$XDG_CONFIG_HOME/gogcli-sandbox/config.json` (fallback: `~/.config/gogcli-sandbox/config.json`)
- Policy: `$XDG_CONFIG_HOME/gogcli-sandbox/policy.json` (fallback: `~/.config/gogcli-sandbox/policy.json`)

Create defaults:

```sh
gogcli-sandbox-init --account you@gmail.com
```

`--account` is required because policies are enforced per account.

Override defaults (example):

```sh
gogcli-sandbox-init --account you@gmail.com --label Label_123 --calendar primary --include-thread-get --max-calendar-days 14
```

Generate a multi-account policy with a default account:

```sh
gogcli-sandbox-init --account cashwilliams@gmail.com
```

Enable send in draft-only mode (recommended):

```sh
gogcli-sandbox-init --account you@gmail.com --allow-send --draft-only
```

Allow direct send only to specific recipients:

```sh
gogcli-sandbox-init --account you@gmail.com --allow-send --allow-send-recipient alice@example.com --allow-send-recipient bob@example.com --draft-only=false
```

Preview policy without writing:

```sh
gogcli-sandbox-init --account you@gmail.com --stdout
```

### Multi-account policy

To enforce policy per account, use an `accounts` map in `policy.json` and set a default account:

```json
{
  "default_account": "cashwilliams@gmail.com",
  "accounts": {
    "cashwilliams@gmail.com": {
      "allowed_actions": ["gmail.search", "calendar.list", "calendar.events"],
      "gmail": {
        "allowed_labels": ["INBOX"],
        "allowed_senders": [],
        "allowed_send_recipients": [],
        "max_days": 7,
        "allow_body": false,
        "allow_links": false,
        "draft_only": true,
        "allow_attachments": false
      },
      "calendar": {
        "allowed_calendars": ["primary"],
        "allow_details": false,
        "max_days": 7
      }
    }
  }
}
```

When multiple accounts are configured, the client should pass `--account` (or set
`GOGCLI_SANDBOX_ACCOUNT`). If omitted, the broker falls back to `default_account`,
then `gog_account` from `config.json`, and finally auto-selects the only account
if there is just one.

## Broker: run manually

```sh
gogcli-sandbox
```

Override config path:

```sh
gogcli-sandbox --config /etc/gogcli-sandbox/config.json
```

Enable verbose logging (metadata only):

```sh
gogcli-sandbox --verbose
```

## Socket permissions (recommended)

The broker listens on a Unix socket. If you run it as root, non-root clients will get
`permission denied` unless you adjust the socket permissions.

**Option A: Use a user-writable socket path**

Set in config:

```json
{ "socket": "/tmp/gogcli-sandbox.sock" }
```

**Option B: Group-based permissions**

```sh
sudo groupadd gogcli-sandbox
sudo usermod -aG gogcli-sandbox $USER 
sudo chgrp gogcli-sandbox /run/gogcli-sandbox.sock
sudo chmod 660 /run/gogcli-sandbox.sock
```

If you use systemd, add these to the service:

```ini
[Service]
ExecStartPost=/bin/chgrp gogcli-sandbox /run/gogcli-sandbox.sock
ExecStartPost=/bin/chmod 660 /run/gogcli-sandbox.sock
```

## Systemd (service + socket)

This is the cleanest approach for production. The broker supports systemd socket activation
by detecting `LISTEN_FDS`/`LISTEN_PID` and serving on the pre-opened socket.

`/etc/systemd/system/gogcli-sandbox.socket`:

```ini
[Unit]
Description=gogcli-sandbox socket

[Socket]
ListenStream=/run/gogcli-sandbox.sock
SocketUser=root
SocketGroup=gogcli-sandbox
SocketMode=0660
RemoveOnStop=true

[Install]
WantedBy=sockets.target
```

`/etc/systemd/system/gogcli-sandbox.service`:

```ini
[Unit]
Description=gogcli-sandbox broker
Requires=gogcli-sandbox.socket
After=gogcli-sandbox.socket

[Service]
Type=simple
ExecStart=/usr/local/bin/gogcli-sandbox
User=root
Group=root

[Install]
WantedBy=multi-user.target
```

Enable:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now gogcli-sandbox.socket
sudo systemctl start gogcli-sandbox.service
```

## Client CLI (agent-facing)

```sh
gogcli-sandbox-client help
gogcli-sandbox-client gmail.search --query "label:INBOX newer_than:7d" --max 10
gogcli-sandbox-client calendar.events --calendar-id primary --days 7
```

Override socket path:

```sh
gogcli-sandbox-client --socket /run/gogcli-sandbox.sock gmail.search --query "label:INBOX newer_than:7d"
```

Specify account:

```sh
gogcli-sandbox-client --account cashwilliams@gmail.com gmail.search --query "label:INBOX newer_than:7d"
```

## Getting label + calendar IDs

```sh
scripts/print-ids.sh
```

Or manually:

```sh
gog gmail labels list --json
gog calendar calendars --json
```

## Testing

```sh
go test ./...
```
