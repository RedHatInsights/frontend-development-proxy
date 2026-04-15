@AGENTS.md

# Claude Code Configuration

## Build Commands

```bash
# Build the custom Caddy binary (requires xcaddy)
cd rh_identity_transform && go build ./...

# Run Go tests (when they exist)
cd rh_identity_transform && go test ./...

# Build the Docker image
podman build -t frontend-development-proxy .

# Verify Go module dependencies
cd rh_identity_transform && go mod tidy && go mod verify

# Lint Go code (if golangci-lint is available)
cd rh_identity_transform && golangci-lint run
```

## Pre-commit Checks

- Run `go vet ./...` in `rh_identity_transform/` before committing Go changes
- Run `shellcheck entrypoint.sh` before committing shell script changes
- Ensure `go mod tidy` leaves no changes before committing
