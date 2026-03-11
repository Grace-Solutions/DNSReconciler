# DNS Automatic Updater

A single-binary, cross-platform DNS reconciliation agent. Each node detects its own reachable address and publishes it to one or more DNS providers — no manual record management required. Designed for Docker Swarm global services, VPS fleets, bare-metal clusters, overlay/mesh networks, and anything else that needs automatic, distributed DNS registration.

---

## Table of contents

- [Why this exists](#why-this-exists)
- [Features](#features)
- [Quick start](#quick-start)
- [Installation](#installation)
- [CLI reference](#cli-reference)
- [Configuration](#configuration)
- [Variable expansion](#variable-expansion)
- [Address selection](#address-selection)
- [Docker deployment](#docker-deployment)
- [Host service lifecycle](#host-service-lifecycle)
- [How reconciliation works](#how-reconciliation-works)
- [Building from source](#building-from-source)
- [Project structure](#project-structure)
- [License](#license)

---

## Why this exists

In distributed environments every node needs to register itself in DNS. Traditional approaches require either a centralised controller that knows about all nodes, or manual record management. Neither scales well and both are fragile.

DNS Automatic Updater takes a different approach: **every node runs the same binary with the same shared config**. Each node independently detects its own address, computes the DNS records it is responsible for, and reconciles them against the provider API. Nodes do not communicate with each other — they converge independently on the correct global state.

This makes it ideal for:

- **Docker Swarm** — run as a global service; each node registers itself for round-robin DNS
- **VPS fleets** — drop the binary on each server and point it at a shared config
- **Homelab / bare metal** — automatic internal DNS for machines joining and leaving a network
- **Overlay networks** — register Tailscale, WireGuard, or ZeroTier addresses in a private DNS zone
- **Hybrid topologies** — publish public addresses to Cloudflare and private addresses to an internal Technitium or PowerDNS instance simultaneously

---

## Features

| Category | Details |
|----------|---------|
| **Multi-provider** | Cloudflare (API v4), Technitium DNS Server, PowerDNS Authoritative. New providers plug into a common `Provider` interface. |
| **Address detection** | Priority-ranked sources: public IPv4/IPv6, RFC 1918, CGNAT/overlay, per-interface with CIDR allow/deny filtering. |
| **Variable expansion** | 14 variables (`${HOSTNAME}`, `${NODE_ID}`, `${SELECTED_IPV4}`, etc.) expanded in record names, content, comments, and tags. |
| **Defaults inheritance** | Four-level merge chain: built-in → provider → global → per-record. |
| **Ownership model** | `perNode`, `singleton`, `manual`, or `disabled` — a node only touches records it owns. |
| **Idempotent reconciliation** | Fingerprint-based diffing; no changes applied if local state matches desired state. |
| **Graceful cleanup** | Optional: delete all owned records on `SIGINT`/`SIGTERM` shutdown. |
| **Jittered scheduler** | Configurable interval with ±10% random jitter to prevent startup stampedes. |
| **Interval override** | Change the reconcile interval via `-interval` CLI flag or `RECONCILE_INTERVAL_SECONDS` env var without editing config. |
| **Auto-generated config** | If the config file doesn't exist on first run, a working default is written automatically. |
| **Service lifecycle** | Idempotent `install`, `remove`, `start`, `stop` via `sc.exe` (Windows), `systemctl` (Linux), `launchctl` (macOS). |
| **Docker-native** | Multi-stage Alpine build with `PUID`/`PGID` support and auto-permission entrypoint. |
| **Credential flexibility** | Direct values, `env:VAR_NAME`, or `file:/path/to/secret`. Secrets are never logged. |
| **Dry-run mode** | Log every planned change without touching the provider API. |
| **Cross-platform binaries** | Pre-built for Linux (amd64/arm64), macOS (amd64/arm64), and Windows (amd64). |

---

## Quick start

### Option A — Download a binary

Grab the appropriate binary from the [`Binaries/`](Binaries/) directory:

| Platform | Binary |
|----------|--------|
| Linux x86_64 | `dnsreconciler-linux-amd64` |
| Linux ARM64 | `dnsreconciler-linux-arm64` |
| macOS Intel | `dnsreconciler-darwin-amd64` |
| macOS Apple Silicon | `dnsreconciler-darwin-arm64` |
| Windows x86_64 | `dnsreconciler-windows-amd64.exe` |

```bash
chmod +x dnsreconciler-linux-amd64
./dnsreconciler-linux-amd64 -config ./config.json --once
```

> **First run?** If `config.json` doesn't exist, a default config is created automatically. Edit it, then run again.

### Option B — Docker

```bash
cd iac/docker
cp .env.example .env        # fill in credentials and PUID/PGID
mkdir -p config
# edit config/config.json (or let it auto-generate on first run)
docker compose up --build
```

---

## Installation

### Binary

1. Download or build the binary for your platform (see [Building from source](#building-from-source)).
2. Place it anywhere on your `PATH`.
3. Run once to generate a default config:
   ```bash
   dnsreconciler -config /etc/dnsreconciler/config.json --once
   ```
4. Edit the generated config and add your provider credentials and records.
5. Run continuously:
   ```bash
   dnsreconciler -config /etc/dnsreconciler/config.json
   ```

### System service

```bash
# Install and start as a background service
dnsreconciler service install
dnsreconciler service start
```

See [Host service lifecycle](#host-service-lifecycle) for details.

### Docker

See [Docker deployment](#docker-deployment).

---

## CLI reference

### Run mode (default)

```
dnsreconciler [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-config <path>` | `./config.json` | Path to the JSON configuration file |
| `-state <path>` | *(from config)* | Override the state file path |
| `-node-id <id>` | *(auto-detected)* | Explicit node identity |
| `-interval <seconds>` | *(from config)* | Override the reconcile interval |
| `-once` | `false` | Run a single reconciliation pass and exit |

**Environment variable overrides:**

| Variable | Equivalent flag |
|----------|----------------|
| `RECONCILE_INTERVAL_SECONDS` | `-interval` |

Priority: CLI flag > environment variable > config file.

### Service mode

```
dnsreconciler service <action> [flags]
```

| Action | Description |
|--------|-------------|
| `install` | Register as a system service (idempotent) |
| `remove` | Unregister the system service |
| `start` | Start the registered service |
| `stop` | Stop the registered service |

| Flag | Default | Description |
|------|---------|-------------|
| `-name <name>` | `dnsreconciler` | Service name |

### Version

```
dnsreconciler version
```

---

## Configuration

The config file is JSON. If the specified file doesn't exist on startup, a sensible default is written to disk automatically so you have a working starting point.

A full annotated example is at [`example/config.json`](example/config.json).

### Minimal config

```json
{
  "version": 1,
  "providerDefaults": {
    "cloudflare": {
      "apiToken": "env:CF_API_TOKEN",
      "zoneId": "env:CF_ZONE_ID"
    }
  },
  "records": [
    {
      "id": "web",
      "provider": "cloudflare",
      "zone": "example.com",
      "type": "A",
      "name": "web.example.com",
      "content": "${SELECTED_IPV4}"
    }
  ]
}
```

That's it. Everything else has sensible defaults.

### Full schema reference

#### Top level

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `version` | `int` | Yes | Must be `1` |
| `providerDefaults` | `object` | No | Per-provider credential and URL configuration |
| `runtime` | `object` | No | Runtime behaviour settings |
| `network` | `object` | No | Global address source configuration |
| `defaults` | `object` | No | Default values inherited by all records |
| `records` | `array` | Yes | Record templates to reconcile |

#### `providerDefaults.<name>`

Each provider has its own key under `providerDefaults`.

**Cloudflare:**

| Key | Required | Description |
|-----|----------|-------------|
| `apiToken` | Yes | Cloudflare API token (Zone:DNS:Edit permission) |
| `zoneId` | Yes | Cloudflare zone ID |
| `baseUrl` | No | API base URL (default: `https://api.cloudflare.com/client/v4`) |

**Technitium:**

| Key | Required | Description |
|-----|----------|-------------|
| `apiToken` | Yes | Technitium API token |
| `baseUrl` | Yes | Server URL (e.g. `http://technitium:5380`) |

**PowerDNS:**

| Key | Required | Description |
|-----|----------|-------------|
| `apiKey` | Yes | PowerDNS API key |
| `baseUrl` | Yes | Server URL (e.g. `http://powerdns:8081`) |
| `serverId` | No | Server ID (default: `localhost`) |

**Credential resolution:**

All credential fields support three syntaxes:

| Prefix | Example | Behaviour |
|--------|---------|-----------|
| *(none)* | `"sk-abc123"` | Used as-is (plaintext) |
| `env:` | `"env:CF_API_TOKEN"` | Resolved from environment variable |
| `file:` | `"file:/run/secrets/cf_token"` | Read from file (trailing newline stripped) |

#### `runtime`

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `reconcileIntervalSeconds` | `int` | `120` | Seconds between reconciliation cycles |
| `statePath` | `string` | `./state.json` | Path to the local state persistence file |
| `cleanupOnShutdown` | `bool` | `false` | Delete all owned records on graceful shutdown |
| `logLevel` | `string` | `Information` | One of: `Trace`, `Debug`, `Information`, `Warning`, `Error`, `Critical` |
| `dryRun` | `bool` | `false` | Log planned changes without applying them |

#### `network.addressSources`

An ordered list of address sources. Each source has a priority (1 = highest) and the first source that returns a valid address wins.

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `priority` | `int` | Yes | 1–4, lower = preferred |
| `type` | `string` | Yes | Source type (see table below) |
| `enabled` | `bool` | Yes | Whether this source is active |
| `interfaceName` | `string` | No | Required for `interfaceIPv4`/`interfaceIPv6` |
| `allowRanges` | `string[]` | No | CIDR ranges to accept |
| `denyRanges` | `string[]` | No | CIDR ranges to reject |
| `addressFamily` | `string` | No | `ipv4` or `ipv6` |
| `explicitValue` | `string` | No | Hard-coded address (for `explicit` type) |

**Source types:**

| Type | Description |
|------|-------------|
| `publicIPv4` | Auto-detected public IPv4 (via external services) |
| `publicIPv6` | Auto-detected public IPv6 |
| `rfc1918IPv4` | First RFC 1918 private address found on interfaces |
| `cgnatIPv4` | First CGNAT (100.64.0.0/10) address — useful for Tailscale/ZeroTier |
| `interfaceIPv4` | IPv4 from a specific named interface |
| `interfaceIPv6` | IPv6 from a specific named interface |
| `explicit` | Hard-coded value from `explicitValue` field |

#### `defaults`

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | `bool` | `true` | Whether records are active by default |
| `ownership` | `string` | `perNode` | `perNode`, `singleton`, `manual`, `disabled` |
| `ttl` | `int` | `120` | DNS TTL in seconds |
| `proxied` | `bool` | `false` | Cloudflare proxy toggle |
| `comment` | `string` | `""` | Comment template (supports variable expansion) |
| `tags` | `array` | `[]` | Tag list (supports variable expansion in values) |

#### `records[]`

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `id` | `string` | Yes | Unique identifier for this record template |
| `enabled` | `bool` | No | Override `defaults.enabled` |
| `provider` | `string` | Yes | `cloudflare`, `technitium`, or `powerdns` |
| `zone` | `string` | Yes | DNS zone name |
| `type` | `string` | Yes | `A` or `AAAA` |
| `name` | `string` | Yes | FQDN (supports variable expansion) |
| `content` | `string` | Yes | Record value (supports variable expansion) |
| `ttl` | `int` | No | Override `defaults.ttl` |
| `proxied` | `bool` | No | Override `defaults.proxied` |
| `ownership` | `string` | No | Override `defaults.ownership` |
| `comment` | `string` | No | Override `defaults.comment` |
| `tags` | `array` | No | Override `defaults.tags` |
| `ipFamily` | `string` | No | `ipv4`, `ipv6`, or `dual` |
| `addressSelection` | `object` | No | Per-record address source override |
| `matchLabels` | `object` | No | Key/value labels for record matching |

---

## Variable expansion

Variables use `${VAR_NAME}` syntax and are expanded in `name`, `content`, `comment`, and tag `value` fields.

| Variable | Source | Example value |
|----------|--------|---------------|
| `${HOSTNAME}` | OS hostname | `web-server-01` |
| `${NODE_ID}` | `-node-id` flag or auto-generated | `abc123def456` |
| `${OS}` | Runtime OS | `linux` |
| `${ARCH}` | Runtime architecture | `amd64` |
| `${PUBLIC_IPV4}` | Detected public IPv4 | `203.0.113.42` |
| `${PUBLIC_IPV6}` | Detected public IPv6 | `2001:db8::1` |
| `${RFC1918_IPV4}` | First RFC 1918 address | `192.168.1.100` |
| `${CGNAT_IPV4}` | First CGNAT address | `100.100.1.1` |
| `${SELECTED_IPV4}` | Winning IPv4 from address selection | `203.0.113.42` |
| `${SELECTED_IPV6}` | Winning IPv6 from address selection | `2001:db8::1` |
| `${SERVICE_NAME}` | Service name (future use) | `web` |
| `${STACK_NAME}` | Stack name (future use) | `production` |
| `${ZONE}` | Record's zone | `example.com` |
| `${RECORD_ID}` | Record's template ID | `public-api` |

**Example:**

```json
{
  "name": "${HOSTNAME}.internal.example.com",
  "content": "${SELECTED_IPV4}",
  "comment": "Managed by dnsreconciler on ${HOSTNAME} (${OS}/${ARCH})"
}
```

---

## Address selection

Each reconciliation cycle resolves the node's address using a priority-ranked source list. The first source that returns a valid, non-filtered address wins.

### How it works

1. Sources are sorted by `priority` (1 = highest).
2. Each enabled source is evaluated in order.
3. `allowRanges` and `denyRanges` CIDR filters are applied.
4. The first match becomes `${SELECTED_IPV4}` (or `${SELECTED_IPV6}`).

### Global vs per-record

By default, all records use `network.addressSources`. A record can override this with its own `addressSelection`:

```json
{
  "id": "mesh-node",
  "provider": "powerdns",
  "zone": "mesh.internal",
  "type": "A",
  "name": "${HOSTNAME}.mesh.internal",
  "content": "${SELECTED_IPV4}",
  "addressSelection": {
    "useGlobalDefaults": false,
    "sources": [
      {
        "priority": 1,
        "type": "interfaceIPv4",
        "enabled": true,
        "interfaceName": "tailscale0",
        "allowRanges": ["100.64.0.0/10"]
      }
    ]
  }
}
```

### Common patterns

**Public-only (Cloudflare CDN):**
```json
"addressSources": [
  { "priority": 1, "type": "publicIPv4", "enabled": true }
]
```

**Tailscale mesh (internal DNS):**
```json
"addressSources": [
  { "priority": 1, "type": "interfaceIPv4", "enabled": true, "interfaceName": "tailscale0", "allowRanges": ["100.64.0.0/10"] }
]
```

**LAN fallback chain:**
```json
"addressSources": [
  { "priority": 1, "type": "publicIPv4", "enabled": true },
  { "priority": 2, "type": "rfc1918IPv4", "enabled": true },
  { "priority": 3, "type": "cgnatIPv4", "enabled": true }
]
```

---

## Docker deployment

All Docker files live in [`iac/docker/`](iac/docker/).

### Directory layout

```
iac/docker/
├── .env.example           # Credential and PUID/PGID template
├── config/                # Bind-mounted as /config in the container
│   └── config.json        # Your configuration file
├── docker-compose.yml     # Service definition
├── Dockerfile             # Multi-stage build
└── entrypoint.sh          # Auto-permission entrypoint
```

### Setup

```bash
cd iac/docker
cp .env.example .env
# Edit .env — set PUID, PGID, and provider credentials
# Edit config/config.json — or let it auto-generate on first run
docker compose up --build -d
```

### Environment variables

All variables in `.env` are injected into the container and can be referenced in `config.json` using `env:VAR_NAME` syntax.

| Variable | Default | Description |
|----------|---------|-------------|
| `PUID` | `1000` | Container user ID — match your host user |
| `PGID` | `1000` | Container group ID — match your host group |
| `RECONCILE_INTERVAL_SECONDS` | *(unset)* | Override the config file's interval |
| `CF_API_TOKEN` | | Cloudflare API token |
| `CF_ZONE_ID` | | Cloudflare zone ID |
| `TECH_API_TOKEN` | | Technitium API token |
| `TECH_BASE_URL` | `http://technitium:5380` | Technitium server URL |
| `PDNS_API_KEY` | | PowerDNS API key |
| `PDNS_BASE_URL` | `http://powerdns:8081` | PowerDNS server URL |

### PUID / PGID

The container entrypoint runs as root, then:

1. Re-creates the internal `dnsreconciler` user/group to match `PUID`/`PGID`.
2. Runs `chown -R` on `/config` and `/state` to fix ownership.
3. Drops privileges via `su-exec` — the application runs as PID 1 under the target user with transparent signal handling.

This ensures bind-mounted volumes have correct permissions regardless of the host user.

```bash
# Find your host UID/GID
id -u    # → 1000
id -g    # → 1000
```

### Volumes

| Container path | Type | Description |
|----------------|------|-------------|
| `/config` | Bind mount | Configuration directory (contains `config.json`) |
| `/state` | Named volume | Persistent state file storage |

### File-based secrets (Docker Swarm / Kubernetes)

Uncomment the secrets volume in `docker-compose.yml` and use `file:` syntax in your config:

```json
"apiToken": "file:/run/secrets/cf_token"
```

---

## Host service lifecycle

The binary can register itself as a background service on Windows, Linux, and macOS.

| Platform | Manager | What happens on `install` |
|----------|---------|--------------------------|
| Windows | `sc.exe` | Creates a Windows service |
| Linux | `systemctl` | Writes a systemd unit file |
| macOS | `launchctl` | Writes a launchd plist |

All operations are idempotent — running `install` twice is safe.

```bash
# Register and start
dnsreconciler service install
dnsreconciler service start

# Stop and unregister
dnsreconciler service stop
dnsreconciler service remove

# Use a custom service name
dnsreconciler service install -name my-dns-agent
dnsreconciler service start -name my-dns-agent
```

---

## How reconciliation works

Each cycle follows these steps:

1. **Load config** — read and validate `config.json` (auto-create if missing).
2. **Resolve runtime context** — detect hostname, OS, architecture, and all network interface addresses.
3. **Merge defaults** — apply the four-level inheritance chain (built-in → provider → global → per-record).
4. **Select addresses** — for each record, walk the priority-sorted source list and pick the winning address.
5. **Expand variables** — replace `${VAR}` placeholders in all string fields.
6. **Compute fingerprints** — hash the desired state for each record.
7. **Diff against provider** — query the DNS provider API for existing records.
8. **Apply changes** — create, update, or delete records as needed (skip if fingerprint matches).
9. **Persist state** — save updated fingerprints and metadata to the state file.

In `--once` mode, a single cycle runs and the process exits. In continuous mode, the scheduler repeats the cycle on the configured interval with jitter.

### Ownership model

| Mode | Behaviour |
|------|-----------|
| `perNode` | Each node owns its own copy of the record (identified by node ID in tags/comments). Multiple nodes can create the same-named record — this enables round-robin DNS. |
| `singleton` | Only one node can own the record. First writer wins; others skip. |
| `manual` | The record is never deleted automatically. |
| `disabled` | Ownership checking is disabled. |

### Cleanup on shutdown

When `cleanupOnShutdown` is `true` and the process receives `SIGINT` or `SIGTERM`:

1. The scheduler stops.
2. A cleanup phase runs with a 30-second timeout on a fresh context.
3. All records owned by this node are deleted from their providers.
4. The state file is updated.

---

## Building from source

All Go commands run from the `src/` directory.

### Prerequisites

- Go 1.22+
- Git

### Build

```bash
cd src

# Local platform
go build -o ../Artifacts/dnsreconciler ./cmd/dnsreconciler

# Run tests
go test ./...

# Cross-compile all targets (static, stripped)
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags="-s -w" -o ../Binaries/dnsreconciler-linux-amd64       ./cmd/dnsreconciler
CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -ldflags="-s -w" -o ../Binaries/dnsreconciler-linux-arm64       ./cmd/dnsreconciler
CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags="-s -w" -o ../Binaries/dnsreconciler-darwin-amd64      ./cmd/dnsreconciler
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags="-s -w" -o ../Binaries/dnsreconciler-darwin-arm64      ./cmd/dnsreconciler
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o ../Binaries/dnsreconciler-windows-amd64.exe ./cmd/dnsreconciler
```

### Docker image

```bash
cd iac/docker
docker compose build
```

---

## Project structure

```
├── Binaries/                      # Pre-built cross-platform binaries
│   ├── dnsreconciler-linux-amd64
│   ├── dnsreconciler-linux-arm64
│   ├── dnsreconciler-darwin-amd64
│   ├── dnsreconciler-darwin-arm64
│   └── dnsreconciler-windows-amd64.exe
├── Artifacts/                     # Local build artifacts (gitignored)
├── docs/
│   ├── CHANGELOG.md
│   └── DesignSpecification.md     # Full design specification
├── example/
│   └── config.json                # Annotated sample configuration
├── iac/
│   └── docker/
│       ├── .env.example           # Environment template
│       ├── config/                # Bind-mounted config directory
│       │   └── config.json
│       ├── docker-compose.yml
│       ├── Dockerfile
│       └── entrypoint.sh          # PUID/PGID auto-permission script
├── Releases/
├── src/
│   ├── cmd/dnsreconciler/         # Binary entrypoint (main.go)
│   ├── go.mod
│   └── internal/
│       ├── address/               # Priority-based address resolver
│       ├── app/                   # Application wiring, CLI parsing
│       ├── cleanup/               # Graceful shutdown record cleanup
│       ├── config/                # Config loading, validation, defaults, auto-generation
│       ├── core/                  # Domain types and Provider interface
│       ├── expansion/             # ${VAR} expansion engine
│       ├── logging/               # Centralized structured logger
│       ├── provider/              # Provider registry, credential resolver, HTTP client
│       │   ├── cloudflare/        # Cloudflare API v4 implementation
│       │   ├── powerdns/          # PowerDNS Authoritative implementation
│       │   └── technitium/        # Technitium DNS Server implementation
│       ├── reconcile/             # Reconciliation pipeline
│       ├── runtimectx/            # Host runtime context resolver
│       ├── scheduler/             # Jittered interval scheduler
│       ├── service/               # Platform service managers (Windows/Linux/macOS)
│       └── state/                 # Local state persistence (JSON)
└── README.md
```

---

## License

See [LICENSE](LICENSE).