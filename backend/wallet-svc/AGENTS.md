# wallet-svc — Go Backend Service

Always load `samber/cc-skills-golang@golang-how-to` for Go work in this repo.

## Tool preference

- Prefer `read` and `edit` for file inspection and modification.
- Use terminal/shell for `go test`, `go build`, `gofmt`, `go vet`, `go mod tidy`, or `swag init`.

## Security & Environment Variable Rules

- NEVER READ `.env` or ANY REAL ENVIRONMENT FILE.
- READ `.env.example` ONLY.
- NO SECRET LEAKAGE.

## Commands

| Command | Description |
|---|---|
| `go test -v -race ./...` | Unit tests. Integration tests run when `TEST_DATABASE_URL` is set. |
| `gofmt -w .` | Format |
| `go vet ./...` | Static analysis |
| `go build -o bin/api ./cmd` | Build API |
| `go run ./cmd/main.go` | Run API (port 8080) |
| `go run ./cmd/migrate` | Apply Bun migrations |
| `air` | Live reload |
| `swag init -g cmd/main.go -o docs --parseDependency --parseInternal` | Swagger |
| `docker compose up -d --build` | Local stack |

## Architecture

- `internal/handler` HTTP only
- `internal/services` business logic, depends on repository interfaces
- `internal/repository` DML only; credit/debit/fund use double-entry pairs in a Bun transaction
- Wallets are scoped by `X-App-Id` (`USER` / `APP` / `MINT` types)
- Bun owns schema via `internal/migrations` and `cmd/migrate`. The API process is DML-only.

## Logging

Use Zap. Never `log.Print` or `fmt.Println` for runtime logs. Propagate `X-Request-ID` and `X-App-Id`.
