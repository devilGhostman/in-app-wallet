# Failure modes

Product catalog of how inapp-wallet fails and recovers. Edge policy lives in `gateway-svc`; money settlement in outbox + Rabbit + wallet Postgres.

Defaults (env): `BREAKER_FAILURES=5`, `BREAKER_OPEN_TIMEOUT=10s`, `PROXY_TIMEOUT=5s`, `PROXY_MAX_RETRIES=2`. Gateway retries **GET/HEAD** only — POST is never retried.

## Catalog

| Failure | Expected API / user effect | Recovery |
|---|---|---|
| **wallet-svc down** | Gateway circuit opens after consecutive failures → **503** JSON (`Upstream temporarily unavailable`) + `X-Request-ID` when present; no dial while open | Restart wallet; after `BREAKER_OPEN_TIMEOUT` half-open probe; successful request closes breaker |
| **order-svc down** | Order/payment/webhook paths → **502/503/504** via gateway ErrorHandler (timeout → **504**, circuit open → **503**, other → **502**) | Restart order-svc; same breaker recovery |
| **wallet Postgres down** | Credit/debit fail → wallet publishes `*_failed` (or command stays unacked / DLQ depending on path); order → `failed` or stays non-terminal until retry | Restore DB; republish / ops for stuck `paid` |
| **RabbitMQ down** | Outbox rows stay unpublished; orders stay `pending`/`paid`; no dual ledger from HTTP retries | Restore Rabbit; outbox publisher drains; consumers reconnect |
| **Redis down** | Gateway rate-limit **fail-open** (requests continue); `/readyz` may be red if Redis is a readiness dependency | Restore Redis; limiting resumes |
| **Duplicate webhook / lifecycle** | Idempotent: no second outbox credit; terminal order status unchanged; wallet key uniqueness prevents double credit/debit | None required |
| **Poison wallet command** | Message → DLQ; consumer stays up | Inspect DLQ; fix payload; replay carefully |
| **Illegal lifecycle** (e.g. `wallet.credited` while purchase still `pending`) | Ignored (acked); status unchanged | Fix upstream / wait for valid transition; stuck `paid` needs ops (see money doc) |

## Verified by

| Area | Proof |
|---|---|
| Money contract | [money-correctness.md](money-correctness.md), `scripts/smoke-money.sh` |
| Gateway retry / breaker / errors | `go test -race ./...` in `backend/gateway-svc` |
| Order lifecycle ignore / fail / replay | `go test -race ./internal/services/...` in `backend/order-svc` |
| Outbox recovers after wallet blip | `scripts/drill-outbox-recovers.sh` |
| Poison → DLQ shape | `scripts/drill-poison-ack.sh` + `wallet-svc` `dlq_test.go` |
| Rabbit brief outage | `scripts/drill-rabbit-blip.sh` |
