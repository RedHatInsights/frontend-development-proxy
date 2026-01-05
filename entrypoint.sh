#!/bin/bash

# DEFINE SOURCES
# The built-in default routes (Stage/Prod)
MAIN_ROUTES="$ROUTES_JSON_PATH"
# The optional developer overrides (Localhost)
# Defaults to a specific path, but can be overridden by env var
CUSTOM_ROUTES="${LOCAL_CUSTOM_ROUTES_PATH:-/config/custom_routes.json}"

# MERGE CONFIGURATION
# Determine which config files exist and merge accordingly
if [ -f "$MAIN_ROUTES" ] && [ -f "$CUSTOM_ROUTES" ]; then
  echo ">>> Loading routes with custom overrides from $CUSTOM_ROUTES"
  # jq -s '.[0] * .[1]' merges the two files. 
  # File [1] (custom) overwrites File [0] (main).
  JSON_INPUT=$(jq -s '.[0] * .[1]' "$MAIN_ROUTES" "$CUSTOM_ROUTES")
elif [ -f "$CUSTOM_ROUTES" ]; then
  echo ">>> Loading custom routes only from $CUSTOM_ROUTES"
  JSON_INPUT=$(cat "$CUSTOM_ROUTES")
elif [ -f "$MAIN_ROUTES" ]; then
  echo ">>> Loading default routes only"
  JSON_INPUT=$(cat "$MAIN_ROUTES")
else
  echo ">>> No routes configured, using fallback only"
  JSON_INPUT="{}"
fi

# GENERATE CADDY CONFIG
# We pipe the merged JSON_INPUT into the existing loop logic
output=$(
  echo "$JSON_INPUT" | jq -r 'to_entries[] | [.key, .value.url, (
    if .key | startswith("/api/") then
        if .value."rh-identity-headers" == false then
            false
        else
            true
        end
    else
        .value."rh-identity-headers" // false
    end
), .value."is_chrome"] | @tsv' |
    while IFS=$'\t' read -r path url rh_identity is_chrome; do
      if [ "$is_chrome" = "true" ]; then
        printf "\thandle @html_fallback {\n"
        printf "\t\trewrite * /apps/chrome/index.html\n"
        printf "\t\treverse_proxy %s {\n" "$url"
        printf "\t\t\theader_up Host {http.reverse_proxy.upstream.hostport}\n"
        printf '\t\t\theader_up Cache-Control "no-cache, no-store, must-revalidate"\n'
        printf "\t\t}\n"
        printf "\t}\n\n"
      fi

      printf "\thandle %s {\n" "$path"
      printf "\t\treverse_proxy %s {\n" "$url"
      printf "\t\t\theader_up Host {http.reverse_proxy.upstream.hostport}\n"
      printf '\t\t\theader_up Cache-Control "no-cache, no-store, must-revalidate"\n'
      printf "\t\t}\n"
      if [ "$rh_identity" = "true" ]; then
        printf "\n\t\trh_identity_transform\n"
      fi
      printf "\t}\n\n"
    done
)

LOCAL_ROUTES=$output /usr/bin/caddy run --config /etc/caddy/Caddyfile