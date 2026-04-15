# Security Guidelines — frontend-development-proxy

## Scope

This is a **development/testing proxy** — it is NOT deployed in production. Security guidelines focus on preventing accidental misuse and maintaining safe defaults.

## JWT Handling

- JWT tokens are parsed **without signature verification** (`jwt.Parse(tokenStr, nil)`) — this is intentional for a dev proxy. Do not add verification unless the proxy's scope changes to production use.
- Token extraction checks two sources in order: `cs_jwt` cookie first, then `Authorization: Bearer` header. Maintain this priority order.
- If no token is found or parsing fails, the request proceeds without an `x-rh-identity` header — do not block requests on auth failure.

## Identity Headers

- The `x-rh-identity` header contains base64-encoded JSON with user info and entitlements. This header format matches what HCC platform services expect.
- Default entitlements in `identity.go` grant access to all services. Do not remove entitlements without understanding downstream impact on testing.
- Sensitive fields (email, username, org_id) come directly from JWT claims — do not log identity headers at debug level in production-adjacent contexts.

## Proxy Configuration

- The `HTTPS_PROXY` env var controls the HTTP proxy used to reach stage/prod environments. This typically points to a Red Hat VPN proxy. Do not hardcode proxy URLs.
- `HCC_ENV_URL` defaults to `console.stage.redhat.com` — never default to a production URL.
- Route configurations (`routes.json`, `custom_routes.json`) should not contain credentials or tokens. Routes only specify path patterns and upstream URLs.

## Container Security

- The Dockerfile installs minimal packages (`bash`, `jq`, `nss-tools`, `tini`). Do not add unnecessary packages.
- Use `tini` as init process to handle signal forwarding and zombie reaping.
- Mount route configs as read-only (`:ro`) when running the container.

## Dependencies

- Keep `caddy` and `golang-jwt` dependencies up to date — these are the primary attack surface.
- Dependency updates are managed by Renovate (`renovate.json`). Review Renovate PRs for breaking changes.
- The `smallstep/certificates` dependency comes transitively through Caddy — monitor for CVEs.
