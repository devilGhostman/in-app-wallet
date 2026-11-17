# Money correctness

Contract for closed-loop coin movements across `order-svc` and `wallet-svc`. Wallet Postgres is the **source of truth** for balances; orders track payment/settlement state and drive commands via the outbox.

## Effects (double-entry)

| Effect | User / system legs | Trigger |
|---|---|---|
| **Purchase credit** | USER `CREDIT` + APP shard `DEBIT` (same `transaction_id`) | After purchase is `paid`, outbox `TOKEN_PURCHASE` → wallet credit |
| **Spend debit** | USER `DEBIT` + APP shard `CREDIT` | Spend create outbox `SPEND` → wallet debit |
| **APP fund** | MINT `DEBIT` + N APP shard `CREDIT`s (sum = fund amount) | Ops `POST /internal/apps/fund` |

Every successful mutation writes balanced legs so per-`app_id` signed ledger sum stays 0 and each wallet `balance` equals the signed sum of its legs.

## Idempotency keys

| Layer | Key | Behavior |
|---|---|---|
| Order create / spend | Client `idempotency_key` per `app_id` | Same key + same payload → return existing order. Different amount/user/type → `KEY_PAYLOAD_MISMATCH` |
| Wallet credit | `order:{order_id}:credit` | Replay returns `Duplicate` with original legs; hash mismatch → conflict |
| Wallet debit | `order:{order_id}:debit` | Same as credit |
| Wallet fund | `order:{order_id}:fund` | Same on mint wallet |
| Webhook | Order already `paid`/`completed` | No second outbox credit command; amount must match order |

Also unique on `(wallet_id, reason, order_id)` as a second line of defense.

## Delivery semantics

- RabbitMQ (and outbox publish) is **at-least-once**.
- Wallet DB uniqueness + `request_hash` make the **ledger effect exactly-once**.
- Order lifecycle consumer treats terminal status as idempotent; illegal transitions are **ignored** (acked), not retried forever.

## Order state machine

### Purchase

```
pending --(webhook paid)--> paid --(wallet.credited)--> completed
                         \--(wallet.credit_failed)--> failed
```

- Create does **not** enqueue wallet work.
- Only `status=paid` webhooks; amount must match; spend orders cannot be paid.

### Spend

```
pending --(wallet.debited)--> completed
       \--(wallet.debit_failed)--> failed
```

- Create and outbox debit command are one DB transaction.
- No payment webhook.

## Balance visibility

- UI / clients should **poll** `GET /orders/{id}` until `completed` or `failed`.
- Wallet balance after credit/debit is authoritative once the wallet mutation commits; order status may lag until the lifecycle event is applied.
