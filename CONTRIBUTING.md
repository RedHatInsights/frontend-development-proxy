# Contributing to frontend-development-proxy

## Development Workflow

1. Fork the repository and clone your fork
2. Create a feature branch from `main`
3. Make your changes following the conventions below
4. Test your changes locally (build the Docker image, run the proxy)
5. Submit a pull request against `main`

## Commit Conventions

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): short description

Optional body with details.
```

**Types**: `feat`, `fix`, `chore`, `docs`, `refactor`, `test`

**Scopes**: `proxy`, `identity`, `routes`, `docker`, `ci`

Keep the title under 50 characters. Include the Jira ticket key in the body when applicable.

## Code Style

### Go

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep the Caddy module interface implementations together in `main.go`
- Keep identity/JWT logic in `identity.go`
- Export only types and functions needed by the Caddy framework
- Use descriptive variable names — avoid single-letter names except in short closures

### Bash

- Use `#!/bin/bash` shebang
- Quote all variable expansions (`"$VAR"`, not `$VAR`)
- Use `printf` for formatted output (not `echo` with escape sequences)
- Add error handling for file operations

### Caddyfile

- Use tabs for indentation
- Keep the global options block minimal
- Document non-obvious directives with inline comments

## Testing

- Go changes should include unit tests in `*_test.go` files
- Test JWT extraction with various token formats (Bearer header, cookie, missing)
- Test identity building with different claim sets
- For entrypoint changes, verify route generation with sample JSON inputs

## Pull Request Guidelines

- Keep PRs focused on a single change
- Include a description of what changed and why
- Reference the Jira ticket if applicable
- Ensure the Docker image builds successfully
