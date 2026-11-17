# wallet-svc

Closed-loop in-app coin wallet for the OTT platform. **Not crypto, not cash-out, not P2P.**

Coins are integers. PostgreSQL is the source of truth. Redis is for rate limiting (and an optional display cache). The mobile app never credits or debits.

Layout and conventions follow [Zodiac](../mogiss/zodiac): Gin, Zap, Bun + pgx, `pkg/response`, constructor DI. **Bun owns migrations** (`cmd/migrate`). The API process is DML-only.

## Run locally

```bash
cp .env.example .env
docker compose up -d postgres redis rabbitmq
go run ./cmd/migrate
go test -v -race ./...
TEST_DATABASE_URL='postgresql://wallet_migrate:wallet_migrate_password@localhost:5432/wallet?sslmode=disable' \
  go test -v -race ./internal/repository/...
go run ./cmd/main.go
```

Default port is `8080`. Probes: `GET /healthz`, `GET /readyz` (Postgres required; Redis down is still ready).

RabbitMQ management UI: `http://localhost:15672` (guest/guest). Set `RABBITMQ_ENABLED=true` to consume credit/debit commands.

Swagger: `http://localhost:8080/swagger` after `swag init -g cmd/main.go -o docs --parseDependency --parseInternal` (includes **messaging** tag for AMQP queue contracts under `/amqp/queues/...` — docs only, not HTTP).

## Auth

Internal callers send an **HS256 JWT** signed with `JWT_SECRET` (min 32 bytes). Optional `JWT_SECRET_PREV` is also accepted during key rotation.

```http
Authorization: Bearer <jwt>
```

Required JWT claim: `exp`.

Permissions come from header (not the token):

```http
X-Scopes: wallet:admin
X-App-Id: my-ott-app
```

`X-App-Id` is required on all wallet mutation/read routes (and admin freeze/unfreeze/close). Wallets are scoped per `(app_id, user_id)`.

Comma or space separated scopes. Known scopes: `wallet:create`, `wallet:read`, `wallet:credit`, `wallet:debit`, `wallet:lookup`, `wallet:admin`.

An end-user app JWT is never enough to credit or debit. Do not expose `/internal` or `/admin` on a public ingress.

## Double-entry model

Each app has three wallet types:

| Type | Role |
|---|---|
| `USER` | End-user balance for that `app_id` |
| `APP` | Pre-funded coin pool shards for that `app_id` (balance ≥ 0). Count = `APP_WALLET_SHARDS` (default 16). Users stick to `hash(user_id) % N` |
| `MINT` | Ops counterparty used only to fund the app pool (may go negative) |

`APP_FUND` splits the amount evenly across all APP shards. Purchases/spends lock only the user's shard, so concurrent users on different shards do not block each other on the pool.

Every mutation writes **two** ledger legs with the same `transaction_id` so `SUM(CREDIT) − SUM(DEBIT) = 0`:

- `TOKEN_PURCHASE` → CREDIT user + DEBIT app (requires funded app pool)
- `SPEND` → DEBIT user + CREDIT app
- `APP_FUND` → CREDIT app + DEBIT mint (`POST /internal/apps/fund`)

## Wallet APIs

Prefix `/internal` (also mounted at `/api/v1/internal`).

| Method | Path | Purpose |
|---|---|---|
| POST | `/internal/apps/fund` | Fund app pool (`APP_FUND`); scope `wallet:admin` |
| POST | `/internal/wallets` | Create user wallet, balance 0, unique on `(app_id, user_id)` |
| GET | `/internal/wallets/{user_id}` | Balance from Postgres |
| POST | `/internal/wallets/{user_id}/credit` | Reason must be `TOKEN_PURCHASE` (also via RabbitMQ) |
| POST | `/internal/wallets/{user_id}/debit` | Reason must be `SPEND` (also via RabbitMQ) |
| GET | `/internal/wallets/{user_id}/ledger` | Cursor + limit; optional `entry_type`, `reason`, `sort=asc\|desc` |
| GET | `/internal/ledger/by-order/{order_id}` | Reconciliation lookup (scoped by `X-App-Id`) |

Credit/debit body: `{ amount, reason, order_id, idempotency_key, metadata? }`.

Fund body: `{ amount, order_id, idempotency_key, metadata? }` with key `order:{orderId}:fund`.

## RabbitMQ AMQP 1.0 (Orders ↔ wallet)

HTTP credit/debit stay available. When `RABBITMQ_ENABLED=true`, wallet-svc consumes **sharded** command queues behind a consistent-hash exchange so same-user credit/debit stay ordered, and publishes lifecycle result events for order-svc.

| Resource | Address (AMQP 1.0) | Role |
|---|---|---|
| Exchange `wallet.commands` (`x-consistent-hash`) | `/exchanges/wallet.commands/{user_id}` | Orders publishes credit/debit; RK = `user_id` |
| `wallet.commands.0` … `wallet.commands.15` | `/queues/wallet.commands.{i}` | Shard queues (default N=16) |
| `wallet.commands.dlq` | `/queues/wallet.commands.dlq` | Permanent command failures |
| Exchange `wallet.events` (`topic`) | `/exchanges/wallet.events/{event_type}` | Success + failure lifecycle events |
| `order.wallet.lifecycle` | `/queues/order.wallet.lifecycle` | Bound `wallet.*` for order-svc |

Replica assignment: this process consumes queues where `shardID % RABBITMQ_SHARD_COUNT == RABBITMQ_SHARD_INDEX` (default `INDEX=0`, `COUNT=1` → all shards).

Message JSON (same fields Orders would send over HTTP, plus `app_id` / `user_id`):

```json
{
  "app_id": "my-ott-app",
  "user_id": "aaaaaaaaaaaaaaaaaaaaaaaa",
  "amount": 100,
  "reason": "TOKEN_PURCHASE",
  "order_id": "ord-1",
  "idempotency_key": "order:ord-1:credit",
  "request_id": "corr-optional",
  "metadata": {}
}
```

- Credit: `reason=TOKEN_PURCHASE`, key `order:{id}:credit`
- Debit: `reason=SPEND`, key `order:{id}:debit`
- Op is derived from `reason` (not from queue name)
- Success: **accept** + DB outbox `wallet.credited` / `wallet.debited` published to `wallet.events`
- Transient failures (lock timeout / 5xx): **release** (requeue); no order status change
- Permanent failures: publish `wallet.credit_failed` / `wallet.debit_failed` to `wallet.events`, then DLQ the command. If failure-event publish fails, **release** the command (retry) so orders are not stuck without a signal.
- Env: `RABBITMQ_URL`, `RABBITMQ_EXCHANGE`, `RABBITMQ_QUEUE_PREFIX`, `RABBITMQ_SHARD_TOTAL`, `RABBITMQ_SHARD_INDEX`, `RABBITMQ_SHARD_COUNT`, `RABBITMQ_COMMANDS_DLQ`, `RABBITMQ_EVENTS_EXCHANGE`, `RABBITMQ_OUTBOX_POLL_INTERVAL`, `RABBITMQ_PUBLISH_TIMEOUT`, `RABBITMQ_PREFETCH`
- Requires RabbitMQ plugin `rabbitmq_consistent_hash_exchange` (baked in via `scripts/Dockerfile.rabbitmq` + `rabbitmq.conf` definitions load)

Response: `{ app_id, user_id?, amount, balance, ledger_id, transaction_id, duplicate }`.

| Status | Meaning |
|---|---|
| 201 | first apply |
| 200 | duplicate; original `balance` |
| 409 `KEY_PAYLOAD_MISMATCH` | same key/order, different body |
| 422 `INSUFFICIENT_FUNDS` | user debit or unfunded app pool on purchase |
| 422 `WALLET_FROZEN` | credit or debit on a frozen wallet |
| 422 `WALLET_CLOSED` | credit or debit on a closed wallet |
| 422 `LIMIT_EXCEEDED` | max amount / max balance / daily cap |
| 422 `INVALID_REQUEST` | bad amount, reason, key, or missing `X-App-Id` |
| 503 `LOCK_TIMEOUT` | retry the same key |

Idempotency key: `order:{orderId}:credit` or `order:{orderId}:debit`. `order_id` must be that order id.

Admin (separate credential; freeze/unfreeze/close also need `X-App-Id`):

- `POST /admin/wallets/{user_id}/freeze` `{ reason }`
- `POST /admin/wallets/{user_id}/unfreeze` `{ reason }`
- `POST /admin/wallets/{user_id}/close` `{ reason }`
- `GET /admin/reconciliation`
- `GET /admin/metrics`

Frozen (`status=0`) and closed (`status=2`) wallets reject both credit and debit. Idempotent replay of pre-freeze/pre-close rows still returns the original entry. A closed wallet can be reopened only via create-wallet.

## Caller contract (Orders / Media / User)

Wallet does not implement these. Existing Mongo services must.

### Buy tokens

```
App → Orders.create(pack_id) → App pays Payment → Payment webhook PAID
  → Orders persists PAID, returns 200
  → worker: Wallet.credit(order:{id}:credit)
  → App polls Orders until CREDITED
```

- Pack price/coins come from the Orders catalog, never the client.
- Verify webhook signature, dedupe PSP event id, **re-fetch the payment from the PSP**.
- Coins = catalog mapping of the PSP-confirmed amount. Send `metadata.price_version`.
- If Wallet is down: order stays `PAID` / `CREDIT_FAILED` and retries the **same** key.
- Statuses: `CREATED → PAYMENT_PENDING → PAID → CREDITED` plus `CREDIT_FAILED`, `FAILED`, `EXPIRED`.

### Spend coins

```
App → Orders.spend(sku)
  → Orders validates SKU / price / entitlement / Media health
  → Wallet.debit(order:{id}:debit)
  → Media.grant entitlement (idempotent on order_id)
  → play_token
```

- **No refund API.** After debit the order must become `FULFILLED`.
- Retry the grant. Sweeper alerts on orders stuck in `DEBITED`.
- Permanent grant failure is an ops incident, not a Wallet call.
- Playback: Media checks entitlement only. Do not call Wallet on play.

Statuses: `CREATED → DEBITED → FULFILLED`, or `REJECTED` before debit.

### User

On signup, best-effort `POST /internal/wallets`. If that fails, Wallet lazy-creates on first credit. Do not store coins on the user document.

### History

| Question | Source |
|---|---|
| Coin +/- | Wallet ledger |
| Which video | Orders `CONTENT_SPEND` |
| Can they play | Media entitlements |
| Title on coin list | BFF joins `ledger.order_id` to Orders |

## Consistency

Credit, debit, and fund run in one `READ COMMITTED` transaction: `SET LOCAL` timeouts, lock both wallets in stable `id` order, look up idempotency/`(reason, order_id)`, insert **two** ledger rows with one `transaction_id`, update both balances, insert outbox.

Redis is never part of correctness. `readyz` stays green if Redis is down.

## Open policy (not implemented)

Chargebacks, failed-grant runbook, legacy `ottwallet` import, and which PSP/IAP channel Orders uses.
