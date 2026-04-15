# Architecture — frontend-development-proxy

## Purpose

This proxy enables local UI and E2E testing against the Hybrid Cloud Console (HCC) environments (stage/prod) by:

1. Routing specific paths to locally running services (frontend apps, backend APIs)
2. Proxying all other requests to the real HCC environment
3. Automatically injecting `x-rh-identity` headers for API routes (extracted from JWT tokens)
4. Providing internal TLS certificates so browsers accept `*.foo.redhat.com` URLs locally

## System Design

```
Browser (https://stage.foo.redhat.com:1337)
    │
    ▼
┌──────────────────────────────────┐
│  Caddy Reverse Proxy             │
│  ┌────────────────────────────┐  │
│  │ TLS (internal CA)          │  │
│  └────────────────────────────┘  │
│                                  │
│  Route Matching (from JSON)      │
│  ┌────────────────────────────┐  │
│  │ /apps/my-app* → localhost  │  │
│  │ /api/my-app/* → localhost  │──┼──► rh_identity_transform
│  │ /* (fallback) → HCC env   │  │    (adds x-rh-identity header)
│  └────────────────────────────┘  │
└──────────────────────────────────┘
    │                    │
    ▼                    ▼
Local Services      HCC Environment
(host.docker.       (console.stage.
 internal:PORT)      redhat.com)
```

## Request Flow

1. **DNS**: User adds `stage.foo.redhat.com` to `/etc/hosts` pointing to `127.0.0.1`
2. **TLS**: Caddy generates internal certificates on demand for `*.foo.redhat.com`
3. **Startup**: `entrypoint.sh` reads route JSON files, merges them, generates Caddy `handle` blocks, and starts Caddy
4. **Routing**: Each request is matched against configured routes:
   - Matched routes are proxied to the configured URL (typically `host.docker.internal:PORT`)
   - API routes (`/api/*`) automatically pass through `rh_identity_transform`
   - Chrome routes with `is_chrome: true` get HTML fallback handling for SPA routing
   - Unmatched routes fall through to the HCC environment URL
5. **Identity Transform**: For API routes, the `rh_identity_transform` Caddy module:
   - Extracts JWT from `Authorization: Bearer` header or `cs_jwt` cookie
   - Parses JWT claims (without signature verification)
   - Builds an `EntitledIdentity` struct with user info and default entitlements
   - Base64-encodes the identity JSON and sets the `x-rh-identity` header

## Key Components

### rh_identity_transform (Go Caddy Module)

- **main.go**: Module registration, Caddy lifecycle hooks (`Provision`, `Validate`), HTTP handler (`ServeHTTP`), Caddyfile parsing
- **identity.go**: Token extraction logic (cookie + header), identity struct definitions, claim-to-identity mapping

The module implements these Caddy interfaces:
- `caddy.Provisioner` — initialization (no-op)
- `caddy.Validator` — validation (no-op)
- `caddyhttp.MiddlewareHandler` — HTTP request handling
- `caddyfile.Unmarshaler` — Caddyfile directive parsing

### entrypoint.sh (Route Generator)

Converts JSON route configuration into Caddy `handle` blocks at container startup. Supports:
- Main routes (from `ROUTES_JSON_PATH`)
- Custom route overrides (from `LOCAL_CUSTOM_ROUTES_PATH`)
- Route merging via `jq` (custom routes take precedence)
- Automatic `rh_identity_transform` for `/api/` routes
- Chrome SPA fallback handling via `is_chrome` flag

### Dockerfile (Multi-stage Build)

1. **Builder stage**: Uses `caddy:builder` image + `xcaddy` to compile a custom Caddy binary with:
   - `cache-handler` — Response caching
   - `transform-encoder` — Log format transformation
   - `rh_identity_transform` — Custom identity header injection
2. **Runtime stage**: Uses stock `caddy` image with the custom binary, plus `bash`, `jq`, `nss-tools`, and `tini`

## Design Decisions

- **No JWT verification**: The proxy runs in development/testing contexts where tokens come from trusted SSO. Signature verification would require key management that adds complexity without security benefit in this context.
- **Bash route generation**: Routes are defined in JSON for simplicity but converted to Caddyfile syntax at startup. This avoids the complexity of Caddy's JSON config API while keeping route definition user-friendly.
- **Default entitlements**: All entitlements are set to `IsEntitled: true` by default — this enables access to all HCC features in development without requiring real entitlement data.
- **Internal TLS**: Caddy's internal CA with on-demand certificates avoids the need for self-signed certificate setup while ensuring HTTPS works locally.
