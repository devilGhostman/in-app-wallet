# Multi-stage production build
FROM golang:1.27-alpine AS builder

WORKDIR /app

RUN apk update && apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /app/bin/api \
    ./cmd

FROM alpine:3.20

WORKDIR /app

RUN apk update && apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /app/bin/api /app/api

USER appuser:appgroup

EXPOSE 8081

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8081/healthz || exit 1

ENTRYPOINT ["/app/api"]
