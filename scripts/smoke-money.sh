#!/usr/bin/env bash
# End-to-end money path through gateway-svc.
# Prerequisites: docker-compose stack up + migrated; gateway on :8081; node + jose for JWT.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
API="${API_URL:-http://localhost:8081}"
APP_ID="${APP_ID:-6a6899644aef7603d8dd7e1a}"
JWT_SECRET="${JWT_SECRET:-dev-only-jwt-secret-change-me-now!!!!!!}"
SCOPES="${SCOPES:-wallet:admin wallet:create wallet:read order:create order:spend order:read order:pay_simulate}"

USER_ID="$(python3 - <<'PY'
import secrets
print(secrets.token_hex(12))
PY
)"

log() { printf '==> %s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "missing dependency: $1"; }
need curl
need python3
need node

log "minting JWT (frontend jose)"
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
  local order_id="$1" want="$2" i status
  for i in $(seq 1 40); do
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

FUND_ORDER="fund-smoke-$(date +%s)"
log "fund APP pool (${FUND_ORDER})"
fund_body="$(api POST /api/v1/internal/apps/fund "{\"amount\":1000000,\"order_id\":\"${FUND_ORDER}\",\"idempotency_key\":\"order:${FUND_ORDER}:fund\"}")"
if [[ "$(json_get "$fund_body" "success")" != "true" ]]; then
  log "fund response (continuing if pool already usable): $fund_body"
fi

log "ensure wallet user=${USER_ID}"
api POST /api/v1/internal/wallets "{\"user_id\":\"${USER_ID}\"}" >/dev/null || true

BUY_KEY="smoke-buy-${USER_ID}"
BUY_AMT=50
log "create purchase amount=${BUY_AMT}"
buy_body="$(api POST /api/v1/internal/orders "{\"user_id\":\"${USER_ID}\",\"amount\":${BUY_AMT},\"idempotency_key\":\"${BUY_KEY}\"}")"
[[ "$(json_get "$buy_body" "success")" == "true" ]] || die "create purchase failed: $buy_body"
ORDER_ID="$(json_get "$buy_body" "data.order_id")"
[[ -n "$ORDER_ID" ]] || die "missing order_id: $buy_body"

log "simulate payment order=${ORDER_ID}"
sim_body="$(api POST /api/v1/internal/payments/simulate "{\"order_id\":\"${ORDER_ID}\"}")"
[[ "$(json_get "$sim_body" "success")" == "true" ]] || die "simulate failed: $sim_body"

log "poll purchase completed"
poll_order "$ORDER_ID" completed >/dev/null
BAL_AFTER_BUY="$(wallet_balance)"
[[ "$BAL_AFTER_BUY" -ge "$BUY_AMT" ]] || die "expected balance >= ${BUY_AMT}, got ${BAL_AFTER_BUY}"
log "balance after purchase=${BAL_AFTER_BUY}"

log "replay purchase idempotency (no new order)"
buy_again="$(api POST /api/v1/internal/orders "{\"user_id\":\"${USER_ID}\",\"amount\":${BUY_AMT},\"idempotency_key\":\"${BUY_KEY}\"}")"
[[ "$(json_get "$buy_again" "data.order_id")" == "$ORDER_ID" ]] || die "idempotent create returned different order"
BAL_REPLAY="$(wallet_balance)"
[[ "$BAL_REPLAY" == "$BAL_AFTER_BUY" ]] || die "balance changed after purchase replay: ${BAL_AFTER_BUY} -> ${BAL_REPLAY}"

# Re-POST simulate should not double-credit (webhook idempotent at order layer; wallet key unique).
log "re-simulate payment (idempotent)"
api POST /api/v1/internal/payments/simulate "{\"order_id\":\"${ORDER_ID}\"}" >/dev/null || true
sleep 1
BAL_RESIM="$(wallet_balance)"
[[ "$BAL_RESIM" == "$BAL_AFTER_BUY" ]] || die "balance changed after re-simulate: ${BAL_AFTER_BUY} -> ${BAL_RESIM}"

SPEND_KEY="smoke-spend-${USER_ID}"
SPEND_AMT=20
log "create spend amount=${SPEND_AMT}"
spend_body="$(api POST /api/v1/internal/orders/spend "{\"user_id\":\"${USER_ID}\",\"amount\":${SPEND_AMT},\"idempotency_key\":\"${SPEND_KEY}\"}")"
[[ "$(json_get "$spend_body" "success")" == "true" ]] || die "create spend failed: $spend_body"
SPEND_ID="$(json_get "$spend_body" "data.order_id")"

log "poll spend completed"
poll_order "$SPEND_ID" completed >/dev/null
BAL_AFTER_SPEND="$(wallet_balance)"
expect=$((BAL_AFTER_BUY - SPEND_AMT))
[[ "$BAL_AFTER_SPEND" == "$expect" ]] || die "expected balance ${expect}, got ${BAL_AFTER_SPEND}"

log "OK smoke-money user=${USER_ID} buy=${ORDER_ID} spend=${SPEND_ID} balance=${BAL_AFTER_SPEND}"
