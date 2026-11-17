#!/usr/bin/env bash
# Drill: brief RabbitMQ outage; spend settles after broker returns (no double debit).
# Prerequisites: compose stack up + migrated; docker compose; node + jose.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
API="${API_URL:-http://localhost:8081}"
APP_ID="${APP_ID:-6a6899644aef7603d8dd7e1a}"
JWT_SECRET="${JWT_SECRET:-dev-only-jwt-secret-change-me-now!!!!!!}"
SCOPES="${SCOPES:-wallet:admin wallet:create wallet:read order:create order:spend order:read order:pay_simulate}"
COMPOSE="${COMPOSE:-}"
BLIP_SECS="${BLIP_SECS:-5}"

log() { printf '==> %s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "missing dependency: $1"; }
need curl
need python3
need node

if [[ -z "$COMPOSE" ]]; then
  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    COMPOSE="docker compose"
  elif command -v docker-compose >/dev/null 2>&1; then
    COMPOSE="docker-compose"
  else
    die "need docker compose or docker-compose (or set COMPOSE=...)"
  fi
fi

cd "$ROOT"

USER_ID="$(python3 - <<'PY'
import secrets
print(secrets.token_hex(12))
PY
)"

log "minting JWT"
TOKEN="$(
  cd "$ROOT/frontend"
  JWT_SECRET="$JWT_SECRET" node --input-type=module -e "
import { SignJWT } from 'jose';
const secret = new TextEncoder().encode(process.env.JWT_SECRET);
console.log(await new SignJWT({}).setProtectedHeader({ alg: 'HS256' }).setExpirationTime('1h').sign(secret));
"
)"

hdr=(-H "Authorization: Bearer ${TOKEN}" -H "X-App-Id: ${APP_ID}" -H "X-Scopes: ${SCOPES}" -H "Content-Type: application/json")

json_get() {
  local body="$1" path="$2"
  python3 - "$body" "$path" <<'PY'
import json, sys
body, path = sys.argv[1], sys.argv[2]
data = json.loads(body)
cur = data
for part in path.split("."):
    if cur is None:
        print("")
        raise SystemExit(0)
    if isinstance(cur, dict):
        cur = cur.get(part)
    else:
        print("")
        raise SystemExit(0)
if cur is None:
    print("")
elif isinstance(cur, bool):
    print("true" if cur else "false")
else:
    print(cur)
PY
}

api() {
  local method="$1" path="$2" data="${3:-}"
  if [[ -n "$data" ]]; then
    curl -sS -X "$method" "${API}${path}" "${hdr[@]}" -d "$data"
  else
    curl -sS -X "$method" "${API}${path}" "${hdr[@]}"
  fi
}

poll_order() {
  local order_id="$1" want="$2" i status body=""
  for i in $(seq 1 90); do
    body="$(api GET "/api/v1/internal/orders/${order_id}")"
    status="$(json_get "$body" "data.status")"
    if [[ "$status" == "$want" ]]; then
      echo "$body"
      return 0
    fi
    if [[ "$status" == "failed" && "$want" != "failed" ]]; then
      die "order ${order_id} failed: $body"
    fi
    sleep 0.5
  done
  die "timeout waiting for order ${order_id} status=${want} last=$body"
}

wallet_balance() {
  local body
  body="$(api GET "/api/v1/internal/wallets/${USER_ID}")"
  json_get "$body" "data.balance"
}

log "health gateway"
curl -sS -f "${API}/healthz" >/dev/null || die "gateway healthz failed at ${API}"

FUND_ORDER="fund-drill-rabbit-$(date +%s)"
log "fund APP pool"
fund_body="$(api POST /api/v1/internal/apps/fund "{\"amount\":1000000,\"order_id\":\"${FUND_ORDER}\",\"idempotency_key\":\"order:${FUND_ORDER}:fund\"}")"
if [[ "$(json_get "$fund_body" "success")" != "true" ]]; then
  log "fund response (continuing if pool already usable): $fund_body"
fi

log "ensure wallet user=${USER_ID}"
api POST /api/v1/internal/wallets "{\"user_id\":\"${USER_ID}\"}" >/dev/null || true

BUY_KEY="drill-rabbit-buy-${USER_ID}"
BUY_AMT=40
log "seed balance via purchase amount=${BUY_AMT}"
buy_body="$(api POST /api/v1/internal/orders "{\"user_id\":\"${USER_ID}\",\"amount\":${BUY_AMT},\"idempotency_key\":\"${BUY_KEY}\"}")"
[[ "$(json_get "$buy_body" "success")" == "true" ]] || die "create purchase failed: $buy_body"
BUY_ID="$(json_get "$buy_body" "data.order_id")"
sim_body="$(api POST /api/v1/internal/payments/simulate "{\"order_id\":\"${BUY_ID}\"}")"
[[ "$(json_get "$sim_body" "success")" == "true" ]] || die "simulate failed: $sim_body"
poll_order "$BUY_ID" completed >/dev/null
BAL_BEFORE="$(wallet_balance)"
[[ "$BAL_BEFORE" -ge "$BUY_AMT" ]] || die "expected balance >= ${BUY_AMT}, got ${BAL_BEFORE}"

log "stop rabbitmq for ${BLIP_SECS}s"
$COMPOSE stop rabbitmq

cleanup() {
  $COMPOSE start rabbitmq >/dev/null 2>&1 || true
}
trap cleanup EXIT

SPEND_KEY="drill-rabbit-spend-${USER_ID}"
SPEND_AMT=15
log "create spend during rabbit blip"
spend_body="$(api POST /api/v1/internal/orders/spend "{\"user_id\":\"${USER_ID}\",\"amount\":${SPEND_AMT},\"idempotency_key\":\"${SPEND_KEY}\"}")"
[[ "$(json_get "$spend_body" "success")" == "true" ]] || die "create spend failed: $spend_body"
SPEND_ID="$(json_get "$spend_body" "data.order_id")"
[[ -n "$SPEND_ID" ]] || die "missing spend order_id: $spend_body"

sleep "$BLIP_SECS"

log "start rabbitmq"
$COMPOSE start rabbitmq
trap - EXIT

# Publishers/consumers may need a moment to reconnect; order apps often stay up.
log "wait for rabbit healthy + services settle"
for i in $(seq 1 30); do
  if $COMPOSE exec -T rabbitmq rabbitmq-diagnostics -q ping >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

log "poll spend completed after rabbit recovery"
poll_order "$SPEND_ID" completed >/dev/null

BAL_AFTER="$(wallet_balance)"
expect=$((BAL_BEFORE - SPEND_AMT))
[[ "$BAL_AFTER" == "$expect" ]] || die "expected balance ${expect} after single spend, got ${BAL_AFTER} (double debit?)"

# Idempotent re-create must not change balance again.
spend_again="$(api POST /api/v1/internal/orders/spend "{\"user_id\":\"${USER_ID}\",\"amount\":${SPEND_AMT},\"idempotency_key\":\"${SPEND_KEY}\"}")"
[[ "$(json_get "$spend_again" "data.order_id")" == "$SPEND_ID" ]] || die "idempotent spend returned different order"
sleep 1
BAL_REPLAY="$(wallet_balance)"
[[ "$BAL_REPLAY" == "$BAL_AFTER" ]] || die "balance changed after spend replay: ${BAL_AFTER} -> ${BAL_REPLAY}"

log "OK drill-rabbit-blip spend=${SPEND_ID} balance=${BAL_AFTER}"
