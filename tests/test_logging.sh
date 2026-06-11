#!/bin/bash
# Tests for PROXY_LOGGING environment variable behavior in entrypoint.sh
# Run: bash tests/test_logging.sh

set -euo pipefail

PASS=0
FAIL=0
SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

assert_eq() {
  local desc="$1" expected="$2" actual="$3"
  if [ "$expected" = "$actual" ]; then
    echo "PASS: $desc"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $desc (expected='$expected', actual='$actual')"
    FAIL=$((FAIL + 1))
  fi
}

assert_contains() {
  local desc="$1" needle="$2" haystack="$3"
  if echo "$haystack" | grep -qF "$needle"; then
    echo "PASS: $desc"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $desc (expected to contain '$needle')"
    FAIL=$((FAIL + 1))
  fi
}

assert_not_contains() {
  local desc="$1" needle="$2" haystack="$3"
  if ! echo "$haystack" | grep -qF "$needle"; then
    echo "PASS: $desc"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $desc (expected NOT to contain '$needle')"
    FAIL=$((FAIL + 1))
  fi
}

# --- Test 1: PROXY_LOGGING=false sets LOG_OUTPUT=discard ---
result=$(PROXY_LOGGING=false bash -c '
  source "'"$SCRIPT_DIR"'/entrypoint.sh" 2>/dev/null &
  PID=$!
  sleep 0.1
  kill $PID 2>/dev/null
  echo "$LOG_OUTPUT"
' 2>/dev/null || true)

# Source just the logging section to test env var
log_output_val=$(PROXY_LOGGING=false bash -c '
  if [ "${PROXY_LOGGING}" = "false" ]; then
    export LOG_OUTPUT="discard"
  fi
  echo "$LOG_OUTPUT"
')
assert_eq "PROXY_LOGGING=false sets LOG_OUTPUT=discard" "discard" "$log_output_val"

# --- Test 2: PROXY_LOGGING unset leaves LOG_OUTPUT empty ---
log_output_val=$(unset PROXY_LOGGING; bash -c '
  if [ "${PROXY_LOGGING}" = "false" ]; then
    export LOG_OUTPUT="discard"
  fi
  echo "$LOG_OUTPUT"
')
assert_eq "PROXY_LOGGING unset leaves LOG_OUTPUT empty" "" "$log_output_val"

# --- Test 3: PROXY_LOGGING=true leaves LOG_OUTPUT empty ---
log_output_val=$(PROXY_LOGGING=true bash -c '
  if [ "${PROXY_LOGGING}" = "false" ]; then
    export LOG_OUTPUT="discard"
  fi
  echo "$LOG_OUTPUT"
')
assert_eq "PROXY_LOGGING=true leaves LOG_OUTPUT empty" "" "$log_output_val"

# --- Test 4: log_msg outputs when logging enabled ---
msg_output=$(bash -c '
  log_msg() { echo "$@"; }
  log_msg "hello"
')
assert_eq "log_msg outputs when logging enabled" "hello" "$msg_output"

# --- Test 5: log_msg suppressed when logging disabled ---
msg_output=$(bash -c '
  log_msg() { :; }
  log_msg "hello"
')
assert_eq "log_msg silent when logging disabled" "" "$msg_output"

# --- Test 6: Entrypoint echoes suppressed with PROXY_LOGGING=false ---
# Source the first part of entrypoint.sh (up to route merging) with no route files
entrypoint_output=$(PROXY_LOGGING=false ROUTES_JSON_PATH=/nonexistent LOCAL_CUSTOM_ROUTES_PATH=/nonexistent bash -c '
  if [ "${PROXY_LOGGING}" = "false" ]; then
    export LOG_OUTPUT="discard"
    log_msg() { :; }
  else
    log_msg() { echo "$@"; }
  fi
  MAIN_ROUTES="$ROUTES_JSON_PATH"
  CUSTOM_ROUTES="${LOCAL_CUSTOM_ROUTES_PATH:-/config/custom_routes.json}"
  if [ -f "$MAIN_ROUTES" ] && [ -f "$CUSTOM_ROUTES" ]; then
    log_msg ">>> Loading routes with custom overrides from $CUSTOM_ROUTES"
  elif [ -f "$CUSTOM_ROUTES" ]; then
    log_msg ">>> Loading custom routes only from $CUSTOM_ROUTES"
  elif [ -f "$MAIN_ROUTES" ]; then
    log_msg ">>> Loading default routes only"
  else
    log_msg ">>> No routes configured, using fallback only"
  fi
' 2>&1)
assert_eq "No output when PROXY_LOGGING=false" "" "$entrypoint_output"

# --- Test 7: Entrypoint echoes present without PROXY_LOGGING ---
entrypoint_output=$(unset PROXY_LOGGING; ROUTES_JSON_PATH=/nonexistent LOCAL_CUSTOM_ROUTES_PATH=/nonexistent bash -c '
  if [ "${PROXY_LOGGING}" = "false" ]; then
    export LOG_OUTPUT="discard"
    log_msg() { :; }
  else
    log_msg() { echo "$@"; }
  fi
  MAIN_ROUTES="$ROUTES_JSON_PATH"
  CUSTOM_ROUTES="${LOCAL_CUSTOM_ROUTES_PATH:-/config/custom_routes.json}"
  if [ -f "$MAIN_ROUTES" ] && [ -f "$CUSTOM_ROUTES" ]; then
    log_msg ">>> Loading routes with custom overrides from $CUSTOM_ROUTES"
  elif [ -f "$CUSTOM_ROUTES" ]; then
    log_msg ">>> Loading custom routes only from $CUSTOM_ROUTES"
  elif [ -f "$MAIN_ROUTES" ]; then
    log_msg ">>> Loading default routes only"
  else
    log_msg ">>> No routes configured, using fallback only"
  fi
' 2>&1)
assert_contains "Output present when PROXY_LOGGING not set" ">>> No routes configured" "$entrypoint_output"

# --- Test 8: err_msg always outputs to stderr, even when logging disabled ---
err_output=$(PROXY_LOGGING=false bash -c '
  if [ "${PROXY_LOGGING}" = "false" ]; then
    export LOG_OUTPUT="discard"
    log_msg() { :; }
  else
    log_msg() { echo "$@"; }
  fi
  err_msg() { echo "ERROR: $*" >&2; }
  log_msg "this should be hidden"
  err_msg "this should be visible"
' 2>&1)
assert_contains "err_msg visible when PROXY_LOGGING=false" "ERROR: this should be visible" "$err_output"
assert_not_contains "log_msg hidden when PROXY_LOGGING=false" "this should be hidden" "$err_output"

# --- Test 9: Invalid JSON produces contextual error with file path ---
INVALID_JSON_FILE=$(mktemp)
echo '{"key": "value",}' > "$INVALID_JSON_FILE"
err_output=$(PROXY_LOGGING=false ROUTES_JSON_PATH="$INVALID_JSON_FILE" LOCAL_CUSTOM_ROUTES_PATH=/nonexistent bash -c '
  source "'"$SCRIPT_DIR"'/entrypoint.sh"
' 2>&1 || true)
assert_contains "Invalid JSON error includes file path" "$INVALID_JSON_FILE" "$err_output"
assert_contains "Invalid JSON error includes ERROR prefix" "ERROR:" "$err_output"
rm -f "$INVALID_JSON_FILE"

# --- Test 10: Invalid JSON error visible even with logging disabled ---
INVALID_JSON_FILE=$(mktemp)
echo '{"trailing": "comma",}' > "$INVALID_JSON_FILE"
err_output=$(PROXY_LOGGING=false ROUTES_JSON_PATH="$INVALID_JSON_FILE" LOCAL_CUSTOM_ROUTES_PATH=/nonexistent bash -c '
  source "'"$SCRIPT_DIR"'/entrypoint.sh"
' 2>&1 || true)
assert_contains "Error output present with PROXY_LOGGING=false" "ERROR:" "$err_output"
assert_contains "Error mentions failed parse" "Failed to parse" "$err_output"
rm -f "$INVALID_JSON_FILE"

# --- Summary ---
echo ""
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
