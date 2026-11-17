# CoinWallet demo frontend

React + Vite + TypeScript demo for the in-app wallet stack via **gateway-svc**.

- First visit generates a **24-char hex** user id (Mongo ObjectId style) in `localStorage` (`inapp_wallet_user_id`)
- **Home** — balance, add tokens (dummy gateway success/fail), recent activity
- **Market** — spend coins on dummy items via `POST /orders/spend`
- **Activity** — ledger credits / debits separately

Purchase never hits wallet credit/debit from the browser: order-svc + RabbitMQ do that.

## Source layout

```
src/
  app/           # App shell, providers, router, global styles
  pages/         # Route screens (home, marketplace, transactions)
  widgets/       # Composite UI (layout, transaction list)
  features/      # User flows (wallet session, top-up, spend)
  entities/      # Domain API + models (wallet, order, ledger)
  shared/        # Config, HTTP client, utils, primitives
```

Imports use the `@/` alias (e.g. `@/entities/wallet/api`).

## Prerequisites

1. **gateway-svc on `:8081`** (public entry), with wallet-svc `:8080` and order-svc `:8082` behind it, RabbitMQ enabled
2. Same `JWT_SECRET` as `VITE_JWT_SECRET` on gateway + backends
3. Keep default `USER_ID_PATTERN=^[a-fA-F0-9]{24}$` on both services (matches the demo user id).

4. Fund the app coin pool once (purchase credits need APP pool balance) — via gateway or wallet directly:

```bash
cd frontend
TOKEN=$(node --input-type=module -e "
import { SignJWT } from 'jose';
const secret = new TextEncoder().encode('dev-only-jwt-secret-change-me-now!!!!!!');
console.log(await new SignJWT({}).setProtectedHeader({ alg: 'HS256' }).setExpirationTime('1h').sign(secret));
")

curl -s -X POST http://localhost:8081/api/v1/internal/apps/fund \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-App-Id: 6a6899644aef7603d8dd7e1a" \
  -H "X-Scopes: wallet:admin" \
  -H "Content-Type: application/json" \
  -d '{"amount":1000000,"order_id":"fund-demo-1","idempotency_key":"order:fund-demo-1:fund"}'
```

Or use wallet Swagger at `http://localhost:8080/swagger` with scope `wallet:admin`. Match `X-App-Id` to your `VITE_APP_ID`.

## Setup

```bash
# start gateway-svc on :8081 first
cd backend/gateway-svc && go run ./cmd

# then frontend
cd frontend
cp .env.example .env
# edit VITE_APP_ID and VITE_JWT_SECRET
npm install
npm run dev
```

Open `http://localhost:5173`. Axios calls **gateway-svc** only (`VITE_API_URL`).

| Env | Required | Purpose |
|---|---|---|
| `VITE_APP_ID` | yes | `X-App-Id` on every request |
| `VITE_JWT_SECRET` | yes | Dev-only HS256 mint (must match gateway/backends) |
| `VITE_API_URL` | no | Default `http://localhost:8081` (gateway) |

## Flows

- **Add tokens:** create purchase → gateway UI → success calls `payments/simulate` → poll order until `completed`/`failed`. Fail skips simulate.
- **Marketplace buy:** `orders/spend` → poll until terminal → refresh balance/ledger.
