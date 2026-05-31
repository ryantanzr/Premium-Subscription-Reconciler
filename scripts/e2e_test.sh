#!/usr/bin/env bash
set -uo pipefail   # no -e: we capture failures manually so the summary always prints

BASE_URL="http://localhost:8080"
PASS=0
FAIL=0

# ── helpers ────────────────────────────────────────────────────────────────────

green() { printf '\033[32m%s\033[0m\n' "$*"; }
red()   { printf '\033[31m%s\033[0m\n' "$*"; }

assert_eq() {
  local test_name="$1" field="$2" expected="$3" actual="$4"
  if [ "$actual" = "$expected" ]; then
    green "  PASS  $test_name: $field = $expected"
    PASS=$((PASS + 1))
  else
    red   "  FAIL  $test_name: $field — expected '$expected', got '$actual'"
    FAIL=$((FAIL + 1))
  fi
}

get_field() {
  local user_id="$1" field="$2"
  curl -sf "$BASE_URL/users/$user_id/entitlement" | jq -r ".$field"
}

psql_exec() {
  docker compose exec -T db psql -U user -d reconciler -c "$1" > /dev/null
}

wait_for_app() {
  printf 'Waiting for app.'
  local i
  for i in $(seq 1 30); do
    if curl -sf "$BASE_URL/users/probe/entitlement" > /dev/null 2>&1; then
      echo ' ready.'
      return 0
    fi
    printf '.'
    sleep 1
  done
  echo ''
  red 'App did not become ready within 30s'
  docker compose logs app
  exit 1
}

seed() {
  local user_id="$1" active="$2" source="$3" reason="$4" expires_sql="$5"
  psql_exec "
    INSERT INTO user_entitlements (user_id, active, source, expires_at, last_changed_at, reason)
    VALUES ('${user_id}', ${active}, '${source}', ${expires_sql}, NOW(), '${reason}')
    ON CONFLICT (user_id) DO UPDATE
      SET active          = EXCLUDED.active,
          source          = EXCLUDED.source,
          expires_at      = EXCLUDED.expires_at,
          reason          = EXCLUDED.reason,
          last_changed_at = EXCLUDED.last_changed_at;
  "
}

cleanup() {
  if [ "$FAIL" -gt 0 ]; then
    echo ''
    red 'One or more tests failed — printing app logs:'
    docker compose logs app
  fi
}
trap cleanup EXIT

# ── setup ──────────────────────────────────────────────────────────────────────

echo '=== E2E: Entitlement Endpoint ==='
echo ''

docker compose up -d --build
wait_for_app
echo ''

# ── test cases ─────────────────────────────────────────────────────────────────

# 1. Unknown user — no DB row, expect free-tier default
echo '[1] Unknown user returns free-tier default'
assert_eq 'unknown_user' 'active' 'false'     "$(get_field u_unknown active)"
assert_eq 'unknown_user' 'source' 'NONE'      "$(get_field u_unknown source)"
assert_eq 'unknown_user' 'reason' 'NO_RECORD' "$(get_field u_unknown reason)"
echo ''

# 2. Active subscription with a future expiry
echo '[2] Active user with future expiry'
seed 'u_active' 'true' 'STORE' 'RENEWAL' "NOW() + INTERVAL '10 days'"
assert_eq 'active_future' 'active' 'true'    "$(get_field u_active active)"
assert_eq 'active_future' 'source' 'STORE'   "$(get_field u_active source)"
assert_eq 'active_future' 'reason' 'RENEWAL' "$(get_field u_active reason)"
echo ''

# 3. Lazy expiration — DB row is active=true but expires_at is in the past
echo '[3] Lazy expiration: stale active row corrected in response'
seed 'u_expired' 'true' 'CARRIER' 'RENEWAL' "NOW() - INTERVAL '1 day'"
assert_eq 'lazy_expiry' 'active' 'false'             "$(get_field u_expired active)"
assert_eq 'lazy_expiry' 'source' 'NONE'              "$(get_field u_expired source)"
assert_eq 'lazy_expiry' 'reason' 'EXPIRATION' "$(get_field u_expired reason)"
echo ''

# 4. Explicitly inactive row — must not be mutated by lazy evaluation
echo '[4] Explicitly inactive user'
seed 'u_inactive' 'false' 'NONE' 'CANCELED' "NOW() - INTERVAL '5 days'"
assert_eq 'inactive' 'active' 'false'    "$(get_field u_inactive active)"
assert_eq 'inactive' 'source' 'NONE'     "$(get_field u_inactive source)"
assert_eq 'inactive' 'reason' 'CANCELED' "$(get_field u_inactive reason)"
echo ''

# 5. Active with no expiry (lifetime / promotional entitlement)
echo '[5] Active user with no expiry date'
psql_exec "
  INSERT INTO user_entitlements (user_id, active, source, expires_at, last_changed_at, reason)
  VALUES ('u_lifetime', true, 'MARKETPLACE', NULL, NOW(), 'PURCHASE')
  ON CONFLICT (user_id) DO UPDATE
    SET active = EXCLUDED.active, source = EXCLUDED.source,
        expires_at = EXCLUDED.expires_at, reason = EXCLUDED.reason;
"
assert_eq 'no_expiry' 'active'    'true'        "$(get_field u_lifetime active)"
assert_eq 'no_expiry' 'source'    'MARKETPLACE' "$(get_field u_lifetime source)"
assert_eq 'no_expiry' 'expiresAt' 'null'        "$(get_field u_lifetime expiresAt)"
echo ''

# 6. Wrong HTTP method returns 405
echo '[6] Non-GET method returns 405'
status=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE_URL/users/u_active/entitlement")
assert_eq 'method_not_allowed' 'http_status' '405' "$status"
echo ''

# ── summary ────────────────────────────────────────────────────────────────────

echo '================================'
printf 'Results: %d passed, %d failed\n' "$PASS" "$FAIL"
echo '================================'

[ "$FAIL" -eq 0 ]
