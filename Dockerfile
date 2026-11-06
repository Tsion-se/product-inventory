# syntax=docker/dockerfile:1

# ---------- Builder ----------
FROM golang:1.22-alpine AS builder
WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata

# Dependency layer (go.sum is optional so a fresh checkout still builds).
COPY go.mod go.sum* ./
RUN go mod download || true

COPY . .
# Ensures go.sum is complete; a no-op when go.sum is already committed.
RUN go mod tidy

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/seeder ./cmd/seeder

# ---------- Runtime ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S -g 10001 app \
 && adduser -S -u 10001 -G app -H -s /sbin/nologin app

WORKDIR /app
COPY --from=builder /out/api /out/seeder /app/

USER app:app
ENV PORT=8080 ENV=production
EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=5 \
  CMD wget -q -O /dev/null "http://127.0.0.1:${PORT}/healthz" || exit 1

ENTRYPOINT ["/app/api"]
