# DNS Automatic Updater

A single-binary, multi-provider DNS reconciliation agent. Each node publishes its own reachable address to one or more DNS providers without manual record management — ideal for Docker Swarm global services, VPS fleets, bare-metal nodes, and overlay/mesh networks.

## Features

- **Multi-provider** — Cloudflare, Technitium, and PowerDNS out of the box; new providers plug into a common interface.
- **Priority-based address selection** — public, RFC 1918, CGNAT/overlay, and per-interface sources with CIDR allow/deny filtering.
- **Variable expansion** — `${HOSTNAME}`, `${NODE_ID}`, `${SELECTED_IPV4}`, and 11 more variables in record names, content, comments, and tags.
- **Defaults inheritance** — built-in → provider → global → per-record merge chain.
- **Ownership safety** — a node only creates, updates, or deletes records it can prove it owns.
- **Idempotent reconciliation** — repeated runs converge on the same correct state with local-state fingerprinting.
- **Graceful cleanup** — optionally deletes owned records on clean shutdown.
- **Jittered scheduler** — configurable interval with random variance to avoid startup stampedes.
- **Cross-platform service lifecycle** — idempotent `install`, `remove`, `start`, `stop` via `sc.exe` (Windows), `systemctl` (Linux), `launchctl` (macOS).
- **Docker-native** — multi-stage build, `PUID`/`PGID` support, auto-permission entrypoint.
- **Credential flexibility** — direct values, `env:VAR`, or `file:/path` for secrets; secrets never appear in logs.

## Quick start

### Binary

```bash
# Run once
./dnsreconciler -config ./config.json --once

# Run continuously (default 120 s interval)
./dnsreconciler -config ./config.json
```

### Docker

```bash
cd iac/docker
cp .env.example .env   # fill in credentials and PUID/PGID
# edit config.json
docker compose up --build
```

## CLI reference

```
dnsreconciler [flags]
  -config string   Path to JSON config file (default "./config.json")
  -state string    Override the configured state file path
  -node-id string  Explicit node identity
  --once           Run a single reconciliation pass and exit

dnsreconciler service <action> [flags]
  action:  install | remove | start | stop
  -name string     Service name (default "dnsreconciler")

dnsreconciler version
```

## Configuration

Place your config at `config.json` (or pass `-config <path>`). A full example is at [`example/config.json`](example/config.json).

### Provider defaults

Each provider is configured under `providerDefaults`. Credentials support three resolution modes:

| Syntax | Example | Description |
|--------|---------|-------------|
| Direct value | `"sk-abc123"` | Plaintext (not recommended for production) |
| Environment variable | `"env:CF_API_TOKEN"` | Resolved from the container/host environment |
| File path | `"file:/run/secrets/cf_token"` | Read from a mounted secret file |

```json
{
  "providerDefaults": {
    "cloudflare": {
      "apiToken": "env:CF_API_TOKEN",
      "zoneId": "env:CF_ZONE_ID",
      "baseUrl": "https://api.cloudflare.com/client/v4"
    },
    "technitium": {
      "apiToken": "env:TECH_API_TOKEN",
      "baseUrl": "env:TECH_BASE_URL"
    },
    "powerdns": {
      "apiKey": "env:PDNS_API_KEY",
      "baseUrl": "env:PDNS_BASE_URL",
      "serverId": "localhost"
    }
  }
}
```

### Runtime settings

| Key | Default | Description |
|-----|---------|-------------|
| `reconcileIntervalSeconds` | `120` | Seconds between reconciliation cycles |
| `statePath` | `./state.json` | Path to the local state file |
| `cleanupOnShutdown` | `false` | Delete owned records on graceful shutdown |
| `logLevel` | `Information` | `Debug`, `Information`, `Warning`, `Error` |
| `dryRun` | `false` | Log planned changes without applying them |

## Docker

The Docker setup lives in [`iac/docker/`](iac/docker/).

| File | Purpose |
|------|---------|
| `Dockerfile` | Multi-stage build: `golang:1.22-alpine` → `alpine:3.20` |
| `docker-compose.yml` | Service definition with volumes, env, and memory limit |
| `.env.example` | Credential and PUID/PGID template — copy to `.env` |
| `config.json` | Working config file mounted into the container |
| `entrypoint.sh` | Auto-permission entrypoint with privilege drop |

### PUID / PGID

Set `PUID` and `PGID` in your `.env` to match the host user that owns the mounted directories. The entrypoint re-creates the internal user, fixes ownership on `/config` and `/state`, then drops to that user via `su-exec`.

```env
PUID=1000
PGID=1000
```

## Building

All Go commands run from the `src/` directory. Build artifacts go into `Binaries/`.

```bash
cd src

# Single platform
go build -o ../Binaries/dnsreconciler.exe ./cmd/dnsreconciler

# Cross-compile all targets
GOOS=linux   GOARCH=amd64 go build -o ../Binaries/dnsreconciler-linux-amd64       ./cmd/dnsreconciler
GOOS=linux   GOARCH=arm64 go build -o ../Binaries/dnsreconciler-linux-arm64       ./cmd/dnsreconciler
GOOS=darwin  GOARCH=amd64 go build -o ../Binaries/dnsreconciler-darwin-amd64      ./cmd/dnsreconciler
GOOS=darwin  GOARCH=arm64 go build -o ../Binaries/dnsreconciler-darwin-arm64      ./cmd/dnsreconciler
GOOS=windows GOARCH=amd64 go build -o ../Binaries/dnsreconciler-windows-amd64.exe ./cmd/dnsreconciler
```

## Service lifecycle

```bash
# Install as a host service (auto-detects platform)
dnsreconciler service install

# Start / stop / remove
dnsreconciler service start
dnsreconciler service stop
dnsreconciler service remove

# Custom service name
dnsreconciler service install -name my-dns-agent
```

## Project structure

```
├── Binaries/                  # Cross-compiled binaries (gitignored)
├── Artifacts/                 # Build artifacts (gitignored)
├── docs/
│   ├── CHANGELOG.md
│   └── DesignSpecification.md
├── example/
│   └── config.json            # Full sample configuration
├── iac/
│   └── docker/
│       ├── .env.example
│       ├── config.json
│       ├── docker-compose.yml
│       ├── Dockerfile
│       └── entrypoint.sh
├── Releases/
├── src/
│   ├── cmd/dnsreconciler/     # Binary entrypoint
│   ├── go.mod
│   └── internal/
│       ├── address/           # Priority-based address resolver
│       ├── app/               # Application wiring and CLI
│       ├── cleanup/           # Graceful shutdown record cleanup
│       ├── config/            # Config loading, defaults, types
│       ├── core/              # Domain types and provider interface
│       ├── expansion/         # Variable expansion engine
│       ├── logging/           # Centralized structured logger
│       ├── provider/          # Provider registry and helpers
│       │   ├── cloudflare/
│       │   ├── powerdns/
│       │   └── technitium/
│       ├── reconcile/         # Reconciliation pipeline
│       ├── runtimectx/        # Host runtime context resolver
│       ├── scheduler/         # Jittered interval scheduler
│       ├── service/           # Platform service managers
│       └── state/             # Local state persistence
└── README.md
```

## License

See [LICENSE](LICENSE).