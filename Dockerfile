# Build customized Caddy binary with cache, transform-encoder and a custom 'rh_identity_transform' plugin.
FROM caddy:2.10.0-builder AS builder

COPY rh_identity_transform .

RUN xcaddy build \
  --with github.com/caddyserver/cache-handler \
  --with github.com/caddyserver/transform-encoder \
  --with rh_identity_transform=$(pwd)

FROM caddy:2.10.0

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
