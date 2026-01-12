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
gogcli-sandbox-init
```

Override defaults (example):

```sh
gogcli-sandbox-init --label Label_123 --calendar primary --include-thread-get --max-calendar-days 14
```

Enable send in draft-only mode (recommended):

```sh
gogcli-sandbox-init --allow-send --draft-only
```

Allow direct send only to specific recipients:

```sh
gogcli-sandbox-init --allow-send --allow-send-recipient alice@example.com --allow-send-recipient bob@example.com --draft-only=false
```

Preview policy without writing:

```sh
gogcli-sandbox-init --stdout
```

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
sudo usermod -aG gogcli-sandbox cash
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

This is the cleanest approach for production, but **requires socket activation support**
in the broker. If you have not enabled socket activation, use the group-based approach above.

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
