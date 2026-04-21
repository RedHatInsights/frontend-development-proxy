#!/bin/bash

# CONFIGURE LOGGING
# Set PROXY_LOGGING=false to disable all log output from the proxy.
# This suppresses both Caddy access/runtime logs and entrypoint messages.
if [ "${PROXY_LOGGING}" = "false" ]; then
  export LOG_OUTPUT="discard"
  log_msg() { :; }
else
  log_msg() { echo "$@"; }
fi

# Errors are always shown on stderr, even when logging is disabled.
# This ensures misconfiguration issues (e.g. invalid JSON) are never silently swallowed.
err_msg() { echo "ERROR: $*" >&2; }

# DEFINE SOURCES
# The built-in default routes (Stage/Prod)
MAIN_ROUTES="$ROUTES_JSON_PATH"

# IOP mode is opt-in via IOP=true only.
IOP_ENABLED=false
if [ "${IOP:-false}" = "true" ]; then
  IOP_ENABLED=true
fi

# The optional developer overrides (Localhost).
# In IOP mode we default to a dedicated override file.
if [ "$IOP_ENABLED" = "true" ]; then
  DEFAULT_CUSTOM_ROUTES_PATH="/config/custom_routes.iop.json"
  CADDY_CONFIG_PATH="/etc/caddy/Caddyfile.iop"
  echo ">>> IOP mode enabled (IOP=true)"
else
  DEFAULT_CUSTOM_ROUTES_PATH="/config/custom_routes.json"
  CADDY_CONFIG_PATH="/etc/caddy/Caddyfile"
  echo ">>> IOP mode disabled (default behavior)"
fi

# Can still be overridden by env var for both modes.
CUSTOM_ROUTES="${LOCAL_CUSTOM_ROUTES_PATH:-$DEFAULT_CUSTOM_ROUTES_PATH}"

# MERGE CONFIGURATION
# Determine which config files exist and merge accordingly
if [ -f "$MAIN_ROUTES" ] && [ -f "$CUSTOM_ROUTES" ]; then
  log_msg ">>> Loading routes with custom overrides from $CUSTOM_ROUTES"
  # jq -s '.[0] * .[1]' merges the two files.
  # File [1] (custom) overwrites File [0] (main).
  if ! JSON_INPUT=$(jq -s '.[0] * .[1]' "$MAIN_ROUTES" "$CUSTOM_ROUTES" 2>&1); then
    err_msg "Failed to parse route files: $MAIN_ROUTES and $CUSTOM_ROUTES"
    err_msg "$JSON_INPUT"
    exit 1
  fi
elif [ -f "$CUSTOM_ROUTES" ]; then
  log_msg ">>> Loading custom routes only from $CUSTOM_ROUTES"
  if ! JSON_INPUT=$(jq '.' "$CUSTOM_ROUTES" 2>&1); then
    err_msg "Failed to parse custom routes file: $CUSTOM_ROUTES"
    err_msg "$JSON_INPUT"
    exit 1
  fi
elif [ -f "$MAIN_ROUTES" ]; then
  log_msg ">>> Loading default routes only"
  if ! JSON_INPUT=$(jq '.' "$MAIN_ROUTES" 2>&1); then
    err_msg "Failed to parse routes file: $MAIN_ROUTES"
    err_msg "$JSON_INPUT"
    exit 1
  fi
else
  log_msg ">>> No routes configured, using fallback only"
  JSON_INPUT="{}"
fi

# GENERATE CADDY CONFIG
# We pipe the merged JSON_INPUT into the existing loop logic
route_tsv=$(echo "$JSON_INPUT" | jq -r 'to_entries[] | [.key, .value.url, (
    if .key | startswith("/api/") then
        if .value."rh-identity-headers" == false then
            false
        else
            true
        end
    else
        .value."rh-identity-headers" // false
    end
), .value."is_chrome"] | @tsv' 2>&1) || {
  err_msg "Failed to generate route config from merged JSON"
  err_msg "$route_tsv"
  exit 1
}

output=$(
  echo "$route_tsv" |
    while IFS=$'\t' read -r path url rh_identity is_chrome; do
      if [ "$is_chrome" = "true" ]; then
        printf "\thandle @html_fallback {\n"
        printf "\t\trewrite * /apps/chrome/index.html\n"
        printf "\t\treverse_proxy %s {\n" "$url"
        if [ "$IOP_ENABLED" != "true" ]; then
          printf "\t\t\theader_up Host {http.reverse_proxy.upstream.hostport}\n"
        fi
        printf '\t\t\theader_up Cache-Control "no-cache, no-store, must-revalidate"\n'
        printf "\t\t}\n"
        printf "\t}\n\n"
      fi

      printf "\thandle %s {\n" "$path"
      printf "\t\treverse_proxy %s {\n" "$url"
      if [ "$IOP_ENABLED" != "true" ]; then
        printf "\t\t\theader_up Host {http.reverse_proxy.upstream.hostport}\n"
      fi
      printf '\t\t\theader_up Cache-Control "no-cache, no-store, must-revalidate"\n'
      printf "\t\t}\n"
      if [ "$rh_identity" = "true" ]; then
        printf "\n\t\trh_identity_transform\n"
      fi
      printf "\t}\n\n"
    done
)

# Add fallback DNS resolvers to prevent resolution failures in Docker Desktop
# Docker Desktop on macOS can have intermittent DNS issues (after sleep, network switch, VPN toggle)
if [ -w /etc/resolv.conf ] 2>/dev/null; then
  if ! grep -q "8.8.8.8" /etc/resolv.conf 2>/dev/null; then
    echo "nameserver 8.8.8.8" >> /etc/resolv.conf
    echo "nameserver 1.1.1.1" >> /etc/resolv.conf
    echo ">>> Added fallback DNS resolvers"
  fi
fi

LOCAL_ROUTES=$output /usr/bin/caddy run --config "$CADDY_CONFIG_PATH" --adapter caddyfile
