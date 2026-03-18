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
- [Container service discovery](#container-service-discovery)
- [Address selection](#address-selection)
- [Docker deployment](#docker-deployment)
- [Host service lifecycle](#host-service-lifecycle)
- [How reconciliation works](#how-reconciliation-works)
- [Log rotation](#log-rotation)
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
| **Multi-provider** | Cloudflare (API v4), Technitium DNS Server, PowerDNS Authoritative, AWS Route53, Azure DNS. New providers plug into a common `Provider` interface. Multiple instances of the same provider type are supported. |
| **Address detection** | Priority-ranked sources: public IPv4/IPv6, RFC 1918, CGNAT/overlay, per-interface with CIDR allow/deny filtering. |
| **Variable expansion** | 14 variables (`${HOSTNAME}`, `${NODE_ID}`, `${SELECTED_IPV4}`, etc.) expanded in record names, content, comments, and tags. |
| **Instance-based providers** | Providers are configured as an array with UUIDs and friendly names. Records reference providers by ID or name. |
| **Defaults inheritance** | Three-level merge chain: built-in → provider → per-record. |
| **Ownership model** | `perNode`, `singleton`, `manual`, or `disabled` — a node only touches records it owns. Default is `perNode`. |
| **Cloudflare plan detection** | Automatically detects free-plan zones and falls back to comment-based ownership (tags require a paid plan). Cached with 24h TTL. |
| **Config hot-reload** | Polling-based file watcher detects `config.json` modifications and reloads configuration without restarting the service. |
| **Idempotent reconciliation** | Fingerprint-based diffing; no changes applied if local state matches desired state. |
| **Graceful cleanup** | Optional: delete all owned records on `SIGINT`/`SIGTERM` shutdown. |
| **Cron scheduler** | 6-field cron expressions (seconds enabled) with timezone support and configurable jitter. |
| **Schedule override** | Change the reconcile schedule via `-schedule` CLI flag or `RECONCILE_SCHEDULE` env var without editing config. |
| **Auto-generated config** | If the config file doesn't exist on first run, a working default is written automatically. |
| **Service lifecycle** | Idempotent `install`, `uninstall`, `start`, `stop`, and `init` (install + start) via `sc.exe` (Windows), `systemctl` (Linux), `launchctl` (macOS). |
| **Centralized logging** | All HTTP requests, provider operations, plan detection, and config reloads are logged with timing and status information. |
| **Log file rotation** | Automatic rotating log files (`<binary>.yyyy.mm.dd.log`) in the binary's directory. Max 3 files at 10 MB each; oldest pruned automatically. |
| **Remote configuration** | Fetch config from a URL with `--config-url`. Supports authenticated GET/POST with identity payload for node-specific configuration. |
| **Container service discovery** | Automatic DNS registration for containers on L2-routable networks (IPVLAN/MACVLAN). Queries Docker and Podman Unix sockets directly — no CLI dependency. Label-based hostname expansion via `${LABEL:key}`. |
| **Docker-native** | Multi-stage Alpine build with `PUID`/`PGID` support and auto-permission entrypoint. |
| **Credential flexibility** | Direct values, `env:VAR_NAME`, or `file:/path/to/secret`. Secrets are never logged. |
| **Dry-run mode** | Log every planned change without touching the provider API. |
| **Cross-platform binaries** | Pre-built for Linux (amd64/arm64), macOS (amd64/arm64), and Windows (amd64). |

---

## Quick start

Pick your platform, run the commands, edit the config, and go.

The base URL for all downloads is:

```
https://raw.githubusercontent.com/Grace-Solutions/DNSReconciler/main
```

### Linux (x86_64)

```bash
mkdir -p DNSReconciler && cd DNSReconciler
curl -LO https://raw.githubusercontent.com/Grace-Solutions/DNSReconciler/main/Binaries/dnsreconciler-linux-amd64
curl -L  https://raw.githubusercontent.com/Grace-Solutions/DNSReconciler/main/example/config.cloudflare.json -o config.json
chmod +x dnsreconciler-linux-amd64
# Edit config.json — set your API token, zone ID, zone name, and record names
./dnsreconciler-linux-amd64 -config ./config.json --once
```

### Linux (ARM64)

```bash
mkdir -p DNSReconciler && cd DNSReconciler
curl -LO https://raw.githubusercontent.com/Grace-Solutions/DNSReconciler/main/Binaries/dnsreconciler-linux-arm64
curl -L  https://raw.githubusercontent.com/Grace-Solutions/DNSReconciler/main/example/config.cloudflare.json -o config.json
chmod +x dnsreconciler-linux-arm64
# Edit config.json — set your API token, zone ID, zone name, and record names
./dnsreconciler-linux-arm64 -config ./config.json --once
```

### macOS (Apple Silicon)

```bash
mkdir -p DNSReconciler && cd DNSReconciler
curl -LO https://raw.githubusercontent.com/Grace-Solutions/DNSReconciler/main/Binaries/dnsreconciler-darwin-arm64
curl -L  https://raw.githubusercontent.com/Grace-Solutions/DNSReconciler/main/example/config.cloudflare.json -o config.json
chmod +x dnsreconciler-darwin-arm64
# Edit config.json — set your API token, zone ID, zone name, and record names
./dnsreconciler-darwin-arm64 -config ./config.json --once
```

### macOS (Intel)

```bash
mkdir -p DNSReconciler && cd DNSReconciler
curl -LO https://raw.githubusercontent.com/Grace-Solutions/DNSReconciler/main/Binaries/dnsreconciler-darwin-amd64
curl -L  https://raw.githubusercontent.com/Grace-Solutions/DNSReconciler/main/example/config.cloudflare.json -o config.json
chmod +x dnsreconciler-darwin-amd64
# Edit config.json — set your API token, zone ID, zone name, and record names
./dnsreconciler-darwin-amd64 -config ./config.json --once
```

### Windows (x86_64)

```powershell
New-Item -ItemType Directory -Force -Path DNSReconciler | Set-Location
$wc = New-Object System.Net.WebClient
$wc.DownloadFile("https://raw.githubusercontent.com/Grace-Solutions/DNSReconciler/main/Binaries/dnsreconciler-windows-amd64.exe", "$PWD\dnsreconciler.exe")
$wc.DownloadFile("https://raw.githubusercontent.com/Grace-Solutions/DNSReconciler/main/example/config.cloudflare.json", "$PWD\config.json")
# Edit config.json — set your API token, zone ID, zone name, and record names
.\dnsreconciler.exe -config .\config.json --once
```

### Docker

```bash
cd iac/docker
cp .env.example .env        # fill in credentials and PUID/PGID
mkdir -p config
# edit config/config.json (or let it auto-generate on first run)
docker compose up --build
```

> **First run?** Edit `config.json` before running — set your provider credentials, zone, and record names. The downloaded config is a Cloudflare template with `env:` references; replace them with your actual values or set the environment variables. See [Configuration](#configuration) for the full schema.

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
| `-config-url <url>` | *(none)* | Fetch configuration from a remote URL instead of (or before) a local file |
| `-config-header <name>` | *(none)* | HTTP header name for remote config authentication (e.g. `Authorization`) |
| `-config-token <value>` | *(none)* | HTTP header value for remote config authentication |
| `-config-method <method>` | `GET` | HTTP method for remote config (`GET` or `POST`) |
| `-state <path>` | *(from config)* | Override the state file path |
| `-node-id <id>` | *(auto-detected)* | Explicit node identity |
| `-schedule <cron>` | *(from config)* | Override the cron schedule (6-field, seconds enabled) |
| `-once` | `false` | Run a single reconciliation pass and exit |

When `-config-method POST` is used, the reconciler sends a JSON identity payload to the remote endpoint:

```json
{
  "hostname": "web-01",
  "nodeId": "ba9298a1-c5b2-4431-b3a4-7f0347e971b5",
  "osInfo": { "os": "linux", "architecture": "amd64" },
  "ipInfo": { "rfc1918IPv4": "192.168.1.10" }
}
```

The remote endpoint can use this information to return node-specific configuration.

**Environment variable overrides:**

| Variable | Equivalent flag |
|----------|----------------|
| `RECONCILE_SCHEDULE` | `-schedule` |
| `CONFIG_URL` | `-config-url` |
| `CONFIG_HEADER` | `-config-header` |
| `CONFIG_TOKEN` | `-config-token` |

Priority: CLI flag > environment variable > config file.

### Service mode

```
dnsreconciler service <action> [flags]
```

| Action | Description |
|--------|-------------|
| `install` | Register as a system service (idempotent) |
| `uninstall` | Unregister the system service |
| `start` | Start the registered service |
| `stop` | Stop the registered service |
| `init` | Install and start in one step (idempotent) |

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
  "settings": {
    "runtime": { "schedule": "0 0 */4 * * *", "jitter": "auto", "timezone": "UTC" }
  },
  "providers": [
    {
      "id": "your-uuid-here",
      "friendlyName": "cloudflare-primary",
      "type": "cloudflare",
      "apiToken": "env:CF_API_TOKEN",
      "zoneId": "env:CF_ZONE_ID",
      "zone": "example.com"
    }
  ],
  "records": [
    {
      "providerId": "cloudflare-primary",
      "recordId": "your-record-uuid",
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
| `settings` | `object` | No | Runtime and network configuration |
| `providers` | `array` | Yes | Provider instances with credentials and zone-level defaults |
| `records` | `array` | Yes | Record templates to reconcile |
| `containerRecords` | `array` | No | Container-aware record templates (see [Container service discovery](#container-service-discovery)) |

#### `providers[]`

Each entry defines a provider instance. Multiple instances of the same type are supported (e.g. two Cloudflare accounts).

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `id` | `string` | Yes | Unique UUID for this provider instance |
| `friendlyName` | `string` | No | Human-readable alias (can be used in `providerId`) |
| `type` | `string` | Yes | `cloudflare`, `technitium`, `powerdns`, `route53`, or `azure` |
| `enabled` | `bool` | No | Default `true` |
| `zone` | `string` | No | Default zone inherited by records |
| `ttl` | `int` | No | Default TTL inherited by records |
| `proxied` | `bool` | No | Default proxied flag (Cloudflare only) |
| `comment` | `string` | No | Default comment template (supports variable expansion) |
| `tags` | `array` | No | Default tag list (supports variable expansion) |

Provider-specific credential keys are included directly on the provider entry:

**Cloudflare:** `apiToken`, `zoneId`, `baseUrl` (optional)
**Technitium:** `apiToken`, `baseUrl`
**PowerDNS:** `apiKey`, `baseUrl`, `serverId` (optional, default: `localhost`)
**Route53:** `accessKeyId`, `secretAccessKey`, `region`, `hostedZoneId`
**Azure:** `tenantId`, `clientId`, `clientSecret`, `subscriptionId`, `resourceGroup`, `zoneName`

**Credential resolution:**

All credential fields support three syntaxes:

| Prefix | Example | Behaviour |
|--------|---------|-----------|
| *(none)* | `"sk-abc123"` | Used as-is (plaintext) |
| `env:` | `"env:CF_API_TOKEN"` | Resolved from environment variable |
| `file:` | `"file:/run/secrets/cf_token"` | Read from file (trailing newline stripped) |

#### `settings.runtime`

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `schedule` | `string` | `0 0 */4 * * *` | 6-field cron expression (seconds enabled). Default: every 4 hours. |
| `jitter` | `string` | `auto` | `auto` (10% of interval), `disabled`, or a Go duration (e.g. `30s`, `5m`). |
| `timezone` | `string` | `UTC` | Unix timezone name (e.g. `America/New_York`). `auto` reads from `TZ` env var or `/etc/timezone`. |
| `statePath` | `string` | `./state.json` | Path to the local state persistence file |
| `cleanupOnShutdown` | `bool` | `false` | Delete all owned records on graceful shutdown |
| `logLevel` | `string` | `Information` | One of: `Trace`, `Debug`, `Information`, `Warning`, `Error`, `Critical` |
| `dryRun` | `bool` | `false` | Log planned changes without applying them |

#### `settings.network.addressSources`

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

#### `records[]`

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `providerId` | `string` | Yes | References a provider by `id` or `friendlyName` |
| `recordId` | `string` | Yes | Unique UUID for this record template |
| `enabled` | `bool` | No | Default `true` |
| `type` | `string` | Yes | `A` or `AAAA` |
| `name` | `string` | Yes | FQDN (supports variable expansion) |
| `content` | `string` | Yes | Record value (supports variable expansion) |
| `zone` | `string` | No | DNS zone (inherited from provider if not set) |
| `ttl` | `int` | No | Override provider TTL |
| `proxied` | `bool` | No | Override provider proxied flag |
| `ownership` | `string` | No | `perNode` (default), `singleton`, `manual`, `disabled` |
| `comment` | `string` | No | Override provider comment |
| `tags` | `array` | No | Override provider tags |
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

### Container variables

The following variables are available **only** inside `containerRecords[]` templates. They are populated per-container during discovery.

| Variable | Source | Example value |
|----------|--------|---------------|
| `${CONTAINER_NAME}` | Container name | `web-frontend` |
| `${CONTAINER_HOSTNAME}` | Container hostname (from inspect) | `web-frontend` |
| `${CONTAINER_ID}` | Short 12-char container ID | `a1b2c3d4e5f6` |
| `${CONTAINER_IP}` | IP on the routable network | `172.16.16.200` |
| `${CONTAINER_IMAGE}` | Container image name | `nginx:alpine` |
| `${LABEL:key}` | Value of container label `key` | `${LABEL:dns.hostname}` → `webserver` |

All standard variables (`${HOSTNAME}`, `${NODE_ID}`, `${ZONE}`, etc.) are also available in container record templates.

**Example:**

```json
{
  "name": "${HOSTNAME}.internal.example.com",
  "content": "${SELECTED_IPV4}",
  "comment": "Managed by dnsreconciler on ${HOSTNAME} (${OS}/${ARCH})"
}
```

---

## Container service discovery

Containers running on L2-routable networks (IPVLAN or MACVLAN) can be automatically discovered and registered in DNS. The reconciler queries Docker and Podman Unix sockets directly — no CLI binaries are required on the host.

### How it works

1. **Socket detection** — the reconciler probes `/var/run/docker.sock` and `/run/podman/podman.sock` for available container runtimes.
2. **Network filtering** — only networks with `ipvlan` or `macvlan` drivers are considered. Bridge, overlay, and host networks are ignored because their addresses are not directly routable on the LAN.
3. **Container enumeration** — running containers attached to a qualifying network are discovered via the Engine REST API.
4. **Template expansion** — each `containerRecords[]` template is expanded once per discovered container, substituting container-specific variables.
5. **Deterministic IDs** — a stable record ID is generated from `SHA-256(providerId|type|name + containerId)`, ensuring consistent ownership tracking across restarts.
6. **Ownership injection** — three tags (`nodeId`, `containerName`, `containerId`) are auto-injected into every generated record. Providers with structured tag support (Cloudflare, Azure) use these for ownership matching. Providers without tag support (Technitium, PowerDNS) fall back to comment-based ownership; the auto-injected tags are stripped before the API call.

### `containerRecords[]` schema

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `description` | `string` | No | Human-readable description of this template |
| `providerId` | `string` | Yes | References a provider by `id` or `friendlyName` |
| `enabled` | `bool` | No | Default `true` |
| `type` | `string` | Yes | `A` or `AAAA` |
| `name` | `string` | Yes | FQDN template (supports variable expansion including `${LABEL:key}`) |
| `content` | `string` | Yes | Record value — typically `${CONTAINER_IP}` |
| `zone` | `string` | No | DNS zone (inherited from provider if not set) |
| `ttl` | `int` | No | Override provider TTL |
| `proxied` | `bool` | No | Override provider proxied flag |
| `comment` | `string` | No | Ownership comment (supports variable expansion) |
| `tags` | `array` | No | Additional tags (supports variable expansion) |
| `ownership` | `string` | No | `perNode` (default), `singleton`, `manual`, `disabled` |
| `include` | `array` | No | Regex patterns — a container must match at least one to be included (empty = include all) |
| `exclude` | `array` | No | Regex patterns — a container matching any pattern is excluded (takes precedence over include) |
| `matchFields` | `array` | No | Container fields to match against: `auto` (default), `containername`, `hostname`, `image`, `containerid`. `auto` expands to `containername` + `hostname`. |

### Container label convention

Containers opt-in to DNS registration by setting labels. The recommended convention is:

```bash
docker run -d --name web \
  --network lan-macvlan \
  --label dns.hostname=webserver \
  nginx:alpine
```

Then reference it in the template:

```json
{
  "type": "A",
  "name": "${LABEL:dns.hostname}.${ZONE}",
  "content": "${CONTAINER_IP}",
  "comment": "{'containerName':'${CONTAINER_NAME}','hostname':'${HOSTNAME}','nodeId':'${NODE_ID}'}"
}
```

This creates an A record `webserver.example.com → 172.16.16.200` with ownership metadata in the comment.

### Docker socket access

When running the reconciler in Docker, mount the host socket read-only:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
```

See the [`docker-compose.yml`](iac/docker/docker-compose.yml) for the full example with the mount commented out by default.

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
  "providerId": "powerdns-mesh",
  "recordId": "mesh-node-uuid",
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
├── .env.example           # Non-secret environment template
├── config/                # Bind-mounted as /config in the container
│   └── config.json        # Your configuration file
├── secrets/               # Docker secrets (gitignored) — use any filenames you want
│   └── (your secret files)   # Referenced via file:/run/secrets/<name> in config
├── state/                 # Bind-mounted as /state in the container
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

> **Docker is the service manager.** The container runs the binary directly in a reconciliation loop — there is no `service install`. The `restart: unless-stopped` policy keeps it running across reboots, effectively acting as the service supervisor.

### Environment variables

All variables in `.env` are injected into the container and can be referenced in `config.json` using `env:VAR_NAME` syntax. The variable names are **not fixed** — you can use any name you like as long as it matches the `env:` reference in your config. The table below shows common conventions, but they are just examples:

| Variable | Default | Description |
|----------|---------|-------------|
| `PUID` | `1000` | Container user ID — match your host user |
| `PGID` | `1000` | Container group ID — match your host group |
| `RECONCILE_SCHEDULE` | *(unset)* | Override the config file's cron schedule |

For example, if your `.env` contains `MY_CF_TOKEN=sk-abc123`, reference it in config as `"apiToken": "env:MY_CF_TOKEN"`. This works for every credential field across all providers — Cloudflare, Technitium, PowerDNS, Route53, and Azure.

### Docker secrets (optional)

The same flexibility applies to Docker secrets. Secret file names are **not prescribed** — use any name you want, mount them however you prefer, and reference the path in your config with `file:` syntax:

```json
"clientSecret": "file:/run/secrets/my_azure_secret"
```

For example, you could create a secret file called `route53_key` and reference it as `"secretAccessKey": "file:/run/secrets/route53_key"`. Any credential field on any provider supports this.

The `secrets/` directory is gitignored — secret files never enter version control.

### PUID / PGID

The container entrypoint runs as root, then:

1. Re-creates the internal `dnsreconciler` user/group to match `PUID`/`PGID`.
2. Runs `chown -R` on `/config` and `/state` to fix ownership.
3. Drops privileges via `su-exec` — the application runs as PID 1 under the target user with transparent signal handling.

This ensures bind-mounted directories have correct permissions regardless of the host user.

```bash
# Find your host UID/GID
id -u    # → 1000
id -g    # → 1000
```

### Volumes

| Container path | Type | Description |
|----------------|------|-------------|
| `/config` | Bind mount (`./config`) | Configuration directory (contains `config.json`) |
| `/state` | Bind mount (`./state`) | State file storage — inspect on host at `iac/docker/state/` |

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
# Install and start in one step (idempotent)
dnsreconciler service init

# Or separately
dnsreconciler service install
dnsreconciler service start

# Stop and unregister
dnsreconciler service stop
dnsreconciler service uninstall

# Use a custom service name
dnsreconciler service init -name my-dns-agent
```

---

## How reconciliation works

Each cycle follows these steps:

1. **Load config** — read and validate `config.json` (auto-create if missing). In continuous mode, a file watcher detects modifications and hot-reloads the config.
2. **Refresh capabilities** — providers that implement `CapabilityRefresher` (e.g. Cloudflare plan detection) are checked for staleness.
3. **Resolve runtime context** — detect hostname, OS, architecture, and all network interface addresses.
4. **Merge defaults** — apply the three-level inheritance chain (built-in → provider → per-record).
5. **Select addresses** — for each record, walk the priority-sorted source list and pick the winning address.
6. **Expand variables** — replace `${VAR}` placeholders in all string fields.
7. **Compute fingerprints** — hash the desired state for each record.
8. **Diff against provider** — query the DNS provider API for existing records.
9. **Apply changes** — create, update, or delete records as needed (skip if fingerprint matches).
10. **Persist state** — save updated fingerprints and metadata to the state file.

In `--once` mode, a single cycle runs and the process exits. In continuous mode, the scheduler repeats the cycle on the configured cron schedule with optional jitter. Config changes are detected automatically and applied before the next pass.

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

## Log rotation

The reconciler automatically writes logs to rotating files alongside stderr output.

| Setting | Value |
|---------|-------|
| **File name** | `<binary>.yyyy.mm.dd.log` (e.g. `dnsreconciler.2026.03.18.log`) |
| **Max file size** | 10 MB per file |
| **Max file count** | 3 (oldest pruned automatically) |
| **Default directory** | Same directory as the running binary |

Log files are created automatically on startup. If the log directory is not writable, a warning is logged to stderr and the reconciler continues without file logging.

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
├── example/                       # Sample configuration files
│   ├── config.json                # Multi-provider example
│   ├── config.cloudflare.json
│   ├── config.technitium.json
│   ├── config.powerdns.json
│   ├── config.route53.json
│   └── config.azure.json
├── iac/
│   └── docker/
│       ├── .env.example           # Non-secret environment template
│       ├── secrets/               # Docker secrets (gitignored)
│       ├── state/                 # Bind-mounted state directory
│       ├── docker-compose.yml
│       ├── Dockerfile
│       └── entrypoint.sh          # PUID/PGID auto-permission script
├── Releases/
├── src/
│   ├── cmd/dnsreconciler/         # Binary entrypoint (main.go)
│   ├── go.mod
│   └── internal/
│       ├── address/               # Priority-based address resolver
│       ├── app/                   # Application wiring, CLI parsing, config hot-reload
│       ├── cleanup/               # Graceful shutdown record cleanup
│       ├── config/                # Config loading, validation, defaults, auto-generation
│       ├── containerrt/           # Container runtime discovery (Docker/Podman via Unix socket)
│       ├── core/                  # Domain types, Provider interface, CapabilityRefresher
│       ├── expansion/             # ${VAR} expansion engine
│       ├── logging/               # Centralized structured logger
│       ├── provider/              # Provider registry, credential resolver, HTTP client
│       │   ├── azure/             # Azure DNS implementation
│       │   ├── cloudflare/        # Cloudflare API v4 with plan detection
│       │   ├── powerdns/          # PowerDNS Authoritative implementation
│       │   ├── route53/           # AWS Route53 with SigV4 authentication
│       │   └── technitium/        # Technitium DNS Server implementation
│       ├── reconcile/             # Reconciliation pipeline
│       ├── runtimectx/            # Host runtime context resolver
│       ├── scheduler/             # Cron-based scheduler with jitter
│       ├── service/               # Platform service managers (Windows/Linux/macOS)
│       ├── state/                 # Local state persistence (JSON)
│       └── watcher/               # Polling-based config file watcher
└── README.md
```

---

## License

See [LICENSE](LICENSE).