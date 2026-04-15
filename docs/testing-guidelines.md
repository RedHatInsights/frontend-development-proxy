# Testing Guidelines — frontend-development-proxy

## Current State

The repository currently has **no automated tests**. New contributions should include tests where applicable.

## Go Module Testing

### Unit Tests for `rh_identity_transform/`

Tests should cover:

1. **Token extraction** (`ExtractToken`):
   - JWT from `Authorization: Bearer <token>` header
   - JWT from `cs_jwt` cookie
   - Cookie takes priority over header when both present
   - Missing token returns `("", false)`
   - Empty/whitespace-only token returns `("", false)`

2. **Identity building** (`BuildIdentity`):
   - Complete claims map produces correct `EntitledIdentity`
   - Missing claims default to zero values (empty string, false, 0)
   - Numeric claims stored as `float64` in JSON are correctly converted
   - Default entitlements are always present

3. **HTTP handler** (`ServeHTTP`):
   - Request with valid JWT gets `x-rh-identity` header added
   - Request without JWT proceeds without modification
   - Request with malformed JWT proceeds without modification
   - `x-rh-identity` header contains valid base64-encoded JSON

### Test File Location

Place test files alongside source files:
- `rh_identity_transform/main_test.go`
- `rh_identity_transform/identity_test.go`

### Test Patterns

```go
func TestExtractToken(t *testing.T) {
    tests := []struct {
        name     string
        request  *http.Request
        wantToken string
        wantOk    bool
    }{
        // test cases
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            token, ok := ExtractToken(tt.request)
            // assertions
        })
    }
}
```

Use table-driven tests. Use `net/http/httptest` for HTTP handler tests. Do not add external test dependencies — use the standard library.

## Integration Testing

### Docker Build Verification

Verify the Docker image builds and runs correctly:

```bash
# Build
podman build -t frontend-development-proxy:test .

# Verify Caddy starts with default config
podman run --rm -e HCC_ENV=test frontend-development-proxy:test caddy version

# Verify entrypoint with sample routes
podman run --rm \
  -v "$(pwd)/config:/config:ro" \
  frontend-development-proxy:test
```

### Route Generation Testing

Test `entrypoint.sh` route generation by providing sample JSON and verifying the generated Caddy config blocks. Key scenarios:
- Main routes only
- Custom routes only
- Merged routes (custom overrides main)
- API routes get `rh_identity_transform` automatically
- Non-API routes do not get identity transform
- `is_chrome` flag generates HTML fallback handler
- `"rh-identity-headers": false` disables identity transform on API routes
