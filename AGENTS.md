# Agent Guide — frontend-development-proxy

## Project Overview

A configurable container proxy for HCC (Hybrid Cloud Console) UI and E2E testing. Built on [Caddy](https://caddyserver.com/) with a custom Go plugin (`rh_identity_transform`) that extracts JWT tokens from incoming requests and generates `x-rh-identity` headers for upstream services.

## Tech Stack

- **Go** (1.25+) — Custom Caddy module in `rh_identity_transform/`
- **Caddy** (2.11.x) — Reverse proxy server, configured via `Caddyfile`
- **Bash** — Entrypoint script for route merging and Caddy startup
- **Docker/Podman** — Container build and runtime
- **xcaddy** — Caddy build tool for compiling custom plugins

## Project Structure

```
.
├── Caddyfile                        # Caddy server configuration
├── Dockerfile                       # Multi-stage build (xcaddy builder + caddy runtime)
├── entrypoint.sh                    # Route JSON → Caddy config generation + startup
├── config/
│   └── routes.json                  # Example route configuration
├── rh_identity_transform/           # Custom Caddy Go module
│   ├── go.mod                       # Go module definition
│   ├── go.sum                       # Dependency checksums
│   ├── main.go                      # Caddy module registration + HTTP handler
│   └── identity.go                  # JWT extraction + identity struct building
├── .tekton/                         # Konflux CI/CD pipeline definitions
└── renovate.json                    # Automated dependency updates
```

## Key Conventions

### Go Module (`rh_identity_transform/`)

- The Go module is a **Caddy plugin** — it implements `caddyhttp.MiddlewareHandler`
- Module ID: `http.handlers.rh_identity_transform`
- Registered via `init()` in `main.go` using `caddy.RegisterModule()` and `httpcaddyfile.RegisterHandlerDirective()`
- The module intercepts HTTP requests, extracts JWT from `Authorization: Bearer` header or `cs_jwt` cookie, parses claims, builds an `EntitledIdentity` struct, and sets the `x-rh-identity` header (base64-encoded JSON)
- JWT parsing is intentionally done **without signature verification** (`jwt.Parse(tokenStr, nil)`) — this is a development proxy, not a production service
- Default entitlements are hardcoded in `identity.go` — all set to `IsEntitled: true`

### Caddyfile

- Uses Caddy's global options block for directive ordering and caching
- `{$LOCAL_ROUTES}` is a placeholder populated by `entrypoint.sh` at runtime
- TLS uses Caddy's internal CA with on-demand certificates
- The `@html_fallback` matcher is used for Chrome UI SPA routing (rewrite to `index.html`)

### entrypoint.sh

- Merges two JSON route files: main routes (`ROUTES_JSON_PATH`) and optional custom routes (`LOCAL_CUSTOM_ROUTES_PATH`)
- Custom routes override main routes (jq merge with `.[0] * .[1]`)
- Generates Caddy `handle` blocks from JSON using `jq` and bash string formatting
- Routes starting with `/api/` automatically get `rh_identity_transform` unless explicitly disabled with `"rh-identity-headers": false`
- The `is_chrome` flag adds an HTML fallback handler for SPA routing

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HCC_ENV` | `stage` | Environment name (used in URL) |
| `PROXY_PORT` | `1337` | Port the proxy listens on |
| `HCC_ENV_URL` | `https://console.stage.redhat.com` | Upstream HCC environment URL |
| `ROUTES_JSON_PATH` | `/config/routes.json` | Path to main routes config |
| `LOCAL_CUSTOM_ROUTES_PATH` | `/config/custom_routes.json` | Path to custom route overrides |
| `PROXY_LOGGING` | _(enabled)_ | Set to `false` to disable all log output |
| `HTTPS_PROXY` | _(none)_ | HTTP proxy for reaching stage/prod |

## Common Pitfalls

1. **Go module path**: The Go module is named `rh_identity_transform` (no domain prefix) — it's built locally via xcaddy `--with rh_identity_transform=$(pwd)`, not fetched from a registry
2. **No tests**: The repo currently has no Go tests or integration tests. Any new functionality should include tests
3. **Route generation is bash**: The route-to-Caddyfile conversion happens in `entrypoint.sh` via `jq` + `printf` — changes to route format require updating both the JSON schema understanding in the script and the Caddy config generation
4. **Caddy directive order matters**: The global options block sets `order rewrite before reverse_proxy` and `order rh_identity_transform before respond` — changing this can break request handling
5. **Docker build requires xcaddy**: The custom Caddy binary is built in a multi-stage Docker build using `xcaddy build` — you cannot use a stock Caddy image

## Docs Index

| Document | Description |
|----------|-------------|
| [Security Guidelines](docs/security-guidelines.md) | JWT handling, identity transformation, proxy security |
| [Testing Guidelines](docs/testing-guidelines.md) | Go module testing, integration testing patterns |
| [Architecture](docs/ARCHITECTURE.md) | System design, request flow, component relationships |
