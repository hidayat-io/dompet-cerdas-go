# syntax=docker/dockerfile:1

# ============================================================
# Builder stage
# ============================================================
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /build

# Cache dependency download.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a fully static binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /build/dompet-cerdas-go ./cmd/api

# ============================================================
# Final stage — minimal, no CGO, non-root
# ============================================================
FROM alpine:3.21

# ca-certificates for HTTPS (Firebase SDK) and tzdata for Asia/Jakarta.
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=builder /build/dompet-cerdas-go .

# Do NOT bake .env or credentials into the image.
# They are provided at runtime via env_file or volume mounts.

USER app

EXPOSE 8080

ENTRYPOINT ["./dompet-cerdas-go"]
