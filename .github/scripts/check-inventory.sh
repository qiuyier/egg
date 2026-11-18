#!/usr/bin/env bash
set -euo pipefail

base_url=${1%/}/api/v1
body=$(mktemp)
trap 'rm -f "$body"' EXIT

check_status() {
  local expected=$1 actual
  shift
  actual=$(curl -sS -o "$body" -w '%{http_code}' "$@")
  if [[ "$actual" != "$expected" ]]; then
    printf 'Expected HTTP %s, got %s\n' "$expected" "$actual" >&2
    cat "$body" >&2
    return 1
  fi
}

item='{"sku":"SMOKE-001","name":"USB-C cable"}'
check_status 400 -H 'Content-Type: application/json' -d '{"sku":" "}' "$base_url/items"
jq -e '.code == "invalid_item" and (.request_id | length > 0)' "$body" > /dev/null

check_status 201 -H 'Content-Type: application/json' -d "$item" "$base_url/items"
jq -e '.code == "ok" and .data.stock == 0 and .data.sku == "SMOKE-001"' "$body" > /dev/null
id=$(jq -er '.data.id' "$body")

check_status 409 -H 'Content-Type: application/json' -d "$item" "$base_url/items"
jq -e '.code == "duplicate_sku"' "$body" > /dev/null

stock_url="$base_url/items/$id/stock"
check_status 409 -X PATCH -H 'Content-Type: application/json' -d '{"delta":-1}' "$stock_url"
jq -e '.code == "stock_out_of_range"' "$body" > /dev/null

check_status 404 -X PATCH -H 'Content-Type: application/json' -d '{"delta":1}' "$base_url/items/9223372036854775807/stock"
jq -e '.code == "item_not_found"' "$body" > /dev/null

check_status 200 -X PATCH -H 'Content-Type: application/json' -d '{"delta":10}' "$stock_url"
jq -e '.data.stock == 10' "$body" > /dev/null

pids=()
for ((i = 0; i < 8; i++)); do
  curl -fsS -o /dev/null -X PATCH -H 'Content-Type: application/json' -d '{"delta":1}' "$stock_url" &
  pids+=("$!")
done
for pid in "${pids[@]}"; do
  wait "$pid"
done

check_status 409 -X PATCH -H 'Content-Type: application/json' -d '{"delta":9223372036854775807}' "$stock_url"
jq -e '.code == "stock_out_of_range"' "$body" > /dev/null

check_status 200 "$base_url/items?page=1&per_page=1"
jq -e --argjson id "$id" '.data.page == 1 and .data.per_page == 1 and .data.items == [{id: $id, sku: "SMOKE-001", name: "USB-C cable", stock: 18}]' "$body" > /dev/null

check_status 200 "$base_url/items?page=2&per_page=1"
jq -e '.data.items == []' "$body" > /dev/null
