# gateway-svc

Edge HTTP gateway in front of `wallet-svc` and `order-svc`.

## Role

- Single public entry on **`:8081`**
- Validates **Bearer JWT (HS256)** + **`X-Scopes`** + **`X-App-Id`**
- Redis token-bucket rate limit (`gateway-svc:ratelimit:{ip}`)
- Assigns / echoes **`X-Request-ID`**
- Path-transparent reverse proxy (same `/api/v1/...` paths)
- Per-upstream timeout, safe GET/HEAD retries, circuit breaker
- Local `/healthz`, `/readyz`, `/metrics`, `/swagger`

## Upstream route table

| Match | Upstream |
|---|---|
| `/api/v1/internal/wallets`, `/api/v1/internal/apps`, `/api/v1/admin` | `WALLET_UPSTREAM` (default `http://localhost:8080`) |
| `/api/v1/internal/orders`, `/api/v1/internal/payments`, `/api/v1/webhooks` | `ORDER_UPSTREAM` (default `http://localhost:8082`) |

`POST /api/v1/webhooks/payment` skips JWT / scopes / app-id (still rate-limited + request-id).

## Run

```bash
cp .env.example .env
# ensure Redis is up; wallet-svc :8080 and order-svc :8082 optional until you proxy
go run ./cmd
```

Swagger: http://localhost:8081/swagger  
Metrics: http://localhost:8081/metrics  

## Frontend

Point the demo UI at this gateway:

```env
VITE_API_URL=http://localhost:8081
```

API paths stay `/api/v1/internal/...`.
