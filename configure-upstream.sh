#!/bin/bash

# Generate only the transport directives appropriate to the catch-all upstream.
write_upstream_config() {
  local upstream_url="$1" config_file="$2"
  if [[ "$upstream_url" == https://* ]]; then
    printf '%s\n' \
      'header_up Host {http.reverse_proxy.upstream.hostport}' \
      'transport http {' \
      '  tls_insecure_skip_verify' \
      '}' > "$config_file"
  else
    printf '%s\n' 'lb_policy first' > "$config_file"
  fi
}
