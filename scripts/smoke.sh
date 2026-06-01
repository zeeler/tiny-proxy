#!/bin/bash
set -e

BASE="${1:-http://127.0.0.1:3688}"
AUTH="${PROXY_AUTH_KEY:-}"

if [ -z "$AUTH" ]; then
	echo "Set PROXY_AUTH_KEY env var to the proxy's auth key"
	exit 1
fi

H="Authorization: Bearer $AUTH"
PASS=0
FAIL=0

check() {
	local desc="$1"
	local expected_code="$2"
	shift 2
	local actual
	actual=$(curl -s -o /dev/null -w "%{http_code}" "$@")
	if [ "$actual" = "$expected_code" ]; then
		echo "PASS: $desc"
		((PASS++))
	else
		echo "FAIL: $desc (expected $expected_code, got $actual)"
		((FAIL++))
	fi
}

check_contains() {
	local desc="$1"
	shift
	local resp
	resp=$(curl -s "$@")
	if echo "$resp" | grep -q "$1"; then
		echo "PASS: $desc"
		((PASS++))
	else
		echo "FAIL: $desc"
		echo "  Response: $resp"
		((FAIL++))
	fi
}

echo "=== tiny-proxy Smoke Tests ==="
echo "Target: $BASE"
echo ""

# 1. Health check (no auth)
check "GET /health returns 200" 200 "$BASE/health"

# 2. Auth failure (no key)
check "GET /v1/models without auth returns 401" 401 "$BASE/v1/models"

# 3. Auth failure (wrong key)
check "GET /v1/models with wrong auth returns 401" 401 -H "Authorization: Bearer wrong-key" "$BASE/v1/models"

# 4. Models endpoint
check "GET /v1/models with auth returns 200" 200 -H "$H" "$BASE/v1/models"

# 5. Models returns deepseek model
check_contains "GET /v1/models" -H "$H" "$BASE/v1/models" "deepseek" 2>/dev/null || true

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="

if [ "$FAIL" -gt 0 ]; then
	exit 1
fi
