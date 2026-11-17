#!/usr/bin/env bash
# Drill: poison wallet command → DLQ; consumer stays healthy.
# Automated proof is unit-level (message shape + routing). Manual compose steps below.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

log() { printf '==> %s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "missing dependency: $1"; }
need go

log "running wallet-svc DLQ unit proofs"
(
  cd "$ROOT/backend/wallet-svc"
  go test ./internal/messaging/ -run 'TestBuildDeadLetterMessage|TestConsumerUsesSharedDLQ|TestClassifyProcessError|TestValidateRejectsUnknownReason' -count=1
)

cat <<'EOF'

Manual compose check (optional):
  1. docker compose up -d  (stack healthy)
  2. Publish a deliberately invalid wallet.commands message (missing app_id / bad amount)
     via RabbitMQ UI :15672 or amqp client into the credit/debit queue.
  3. Expect: message lands in wallet.commands.dlq with x-error / x-original-queue props;
     wallet-svc stays up (curl localhost:8080/healthz or via gateway wallet path).
  4. Do not replay DLQ into prod without fixing the payload.

Automated product proof for poison handling: the unit tests above + docs/failure-modes.md.
EOF

log "OK drill-poison-ack (unit)"
