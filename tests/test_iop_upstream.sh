#!/bin/bash
set -euo pipefail

test_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
helper="$test_root/configure-upstream.sh"
if [ ! -f "$helper" ]; then
  helper=/usr/local/bin/configure-upstream.sh
fi
source "$helper"
test_dir="$(mktemp -d)"
trap 'rm -f "$test_dir/http.caddy" "$test_dir/https.caddy"; rmdir "$test_dir"' EXIT

write_upstream_config http://host.docker.internal:3000 "$test_dir/http.caddy"
if grep -Eq 'tls_|header_up Host' "$test_dir/http.caddy"; then
  echo 'FAIL: HTTP upstream enabled TLS or rewrote the browser Host' >&2
  exit 1
fi
grep -q 'lb_policy first' "$test_dir/http.caddy"
echo 'PASS: HTTP Foreman preserves the browser Host and uses plain HTTP'

write_upstream_config https://foreman.example.test "$test_dir/https.caddy"
grep -q 'header_up Host' "$test_dir/https.caddy"
grep -q 'tls_insecure_skip_verify' "$test_dir/https.caddy"
echo 'PASS: HTTPS upstream uses its virtual host and development TLS transport'
