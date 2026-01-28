# Build customized Caddy binary with custom plugins
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy go module files first for better caching
COPY go.mod go.sum ./
COPY feo_interceptor/go.mod feo_interceptor/go.sum ./feo_interceptor/
COPY rh_identity_transform/go.mod rh_identity_transform/go.sum ./rh_identity_transform/

# Download dependencies
RUN go mod download

# Copy source code
COPY cmd ./cmd
COPY feo_interceptor ./feo_interceptor
COPY rh_identity_transform ./rh_identity_transform

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /usr/bin/caddy ./cmd/frontend-development-proxy

FROM caddy:2.10.2

COPY --from=builder /usr/bin/caddy /usr/bin/caddy

RUN apk add --no-cache bash
RUN apk add --no-cache jq
RUN apk add --no-cache nss-tools
RUN apk add --no-cache tini

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

COPY Caddyfile /etc/caddy/Caddyfile

ENV HCC_ENV="stage"
ENV PROXY_PORT="1337"
ENV HCC_ENV_URL="https://console.stage.redhat.com"
ENV ROUTES_JSON_PATH="/config/routes.json"

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/entrypoint.sh"]
