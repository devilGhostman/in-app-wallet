# order-svc

Purchase and spend orders for in-app coins.

### Purchase
1. `POST /api/v1/internal/orders` → `type=purchase`, `status=pending`
2. `POST /api/v1/internal/payments/simulate` → signed webhook
3. Webhook → `paid` + outbox `TOKEN_PURCHASE` credit command
4. `wallet.credited` → `completed`; `wallet.credit_failed` → `failed`

### Spend (no payment gateway)
1. `POST /api/v1/internal/orders/spend` → `type=spend`, `status=pending` + outbox `SPEND` debit
2. `wallet.debited` → `completed`; `wallet.debit_failed` → `failed`

Poll `GET /api/v1/internal/orders/{order_id}` until `completed` or `failed`.

## Quick start

```bash
cp .env.example .env
docker compose up -d postgres redis rabbitmq   # use wallet-svc compose for shared RabbitMQ defs
go run ./cmd/migrate
go test ./...
go run ./cmd/main.go
```

API: `http://localhost:8082` · Swagger: `http://localhost:8082/swagger`

## Auth

Internal routes: `Authorization: Bearer <HS256 JWT>`, `X-Scopes`, `X-App-Id`.

| Scope | Routes |
|---|---|
| `order:create` | create purchase |
| `order:spend` | create spend |
| `order:read` | get order |
| `order:pay_simulate` | simulate payment |

Webhook uses `X-Payment-Signature` = hex HMAC-SHA256(`PAYMENT_WEBHOOK_SECRET`, raw body). No JWT.

## Messaging

| Direction | Resource | When |
|---|---|---|
| order → wallet | `wallet.commands` (RK=`user_id`) | spend create / purchase paid |
| wallet → order | queue `order.wallet.lifecycle` (from `wallet.events`) | credited / debited / *_failed |

## Env highlights

See `.env.example`. Important: `PAYMENT_WEBHOOK_URL`, `RABBITMQ_EXCHANGE=wallet.commands`, `RABBITMQ_EVENTS_QUEUE=order.wallet.lifecycle`, `RABBITMQ_ENABLED`.
