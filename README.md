# DompetCerdas Go Backend

Backend rewrite of [dompet_cerdas](https://github.com/hidayat-io/dompet-cerdas) — an Indonesian personal finance app with AI-powered receipt scanning and Telegram bot integration.

## Status

Backend rewrite in progress. Shared utilities, domain types, infrastructure, and
the parsing/NLU layer are ported and covered by parity tests captured from the
running legacy implementation. Business logic for the Telegram bot router,
shared-account collaboration, AI analysis, and reminder jobs is not ported yet.

Endpoints whose logic is unported are routed and validate their input, then
return `501 NOT_IMPLEMENTED` with the phase that will implement them. They never
return a fabricated success payload.

| Area | State |
|---|---|
| Config, logging, graceful shutdown | Ported |
| Firebase Auth middleware, CORS, response envelope | Ported |
| Rate limiter (in-memory, interface-backed) | Ported |
| Asia/Jakarta date ranges | Ported, `last_month` rollover bug fixed |
| IDR parsing and formatting | Ported, verified against legacy output |
| Firestore 3-variant path resolution | Ported |
| Shared-workspace permission model | Ported |
| Category / creator-name caches | Ported |
| Transaction parser and auto-save gate | Ported, fixture-verified; receipt photos auto-save above confidence 90 (ADR-016) |
| Receipt attachments (Telegram) | Ported, stored privately in Storage without public URLs; web resolves display URL from path (ADR-017) |
| NLU intent classification | Ported, fixture-verified |
| Telegram Markdown escaping | Ported, fixture-verified |
| Transaction query, sort, limit | Ported |
| Gemini client (vision, audio, insights) | Ported, uncovered by tests beyond JSON parsing |
| Shared-account conversion job | Partially ported (resumable copy, no cleanup phase) |
| Telegram webhook idempotency | Ported |
| Telegram bot router, commands, sessions | Not ported (phase 8) |
| Web AI analysis prompts and synthesis | Not ported (phase 5) |
| Reminder job bodies | Not ported (phase 9) |

## Tech Stack

| Component | Technology |
|---|---|
| Language | Go 1.26 |
| HTTP Framework | Gin |
| Database | Firestore (Firebase Admin SDK v4) |
| Authentication | Firebase Auth (ID token verification) |
| AI | Google Gemini SDK (`google.golang.org/genai`), model `gemini-2.5-flash` |
| Cron | `github.com/robfig/cron/v3` |
| Config | `github.com/joho/godotenv` + typed struct |
| Logging | `log/slog` (JSON in production) |
| Deployment | Docker (multi-stage, CGO-free, non-root) |

Image preprocessing will use `github.com/disintegration/imaging` (pure Go, no
CGO) when the receipt pipeline lands; it is not a dependency yet.

## Prerequisites

- Go 1.26+
- Firebase project with Firestore and Auth enabled
- Firebase service account JSON key
- Telegram bot token (from @BotFather)
- Gemini API key

## Local Setup

```bash
# Clone
git clone https://github.com/hidayat-io/dompet-cerdas-go.git
cd dompet-cerdas-go

# Copy environment config
cp .env.example .env
# Edit .env with your actual values

# Place your Firebase service account key
cp /path/to/your-service-account.json ./service-account.json

# Install dependencies
go mod tidy

# Run
make run
```

## Running

```bash
# Development
make run

# Build binary
make build
./bin/dompet-cerdas-go

# Docker
make docker-build
make docker-up

# Run tests
make test

# Lint
make lint
```

## Project Layout

```
dompet_cerdas_go/
├── cmd/api/              # Application entry point
│   └── main.go           # Wire config, Firebase, router, cron, graceful shutdown
├── internal/
│   ├── config/           # Typed config from environment variables
│   ├── domain/           # Firestore-mapped domain structs (no business logic)
│   ├── middleware/       # Auth, CORS, logging, recovery
│   ├── shared/
│   │   ├── db/           # Firebase/Firestore client initialization
│   │   ├── response/     # Standard JSON response envelope
│   │   ├── ratelimit/    # Rate limiter interface + in-memory implementation
│   │   ├── datetime/     # Asia/Jakarta timezone helpers + date range resolution
│   │   ├── money/        # IDR amount parsing and formatting
│   │   └── gemini/       # Gemini client (vision, audio, insights)
│   └── modules/
│       ├── health/       # GET /api/v1/health with Firestore connectivity check
│       ├── account/      # Path resolution, permissions, caches, conversion job
│       ├── transaction/  # Indonesian parser, auto-save gate, query layer
│       ├── telegram/     # Webhook idempotency, NLU, message formatting
│       ├── advisor/      # AI analysis quota (rolling 24h)
│       └── reminder/     # Hourly cron with Firestore leader lock
├── testdata/parity/      # Fixtures captured from the running legacy backend
├── docs/                 # Migration plan, ADRs, parity contract
├── Dockerfile            # Multi-stage, CGO-free, non-root
├── docker-compose.yml    # Service definition with healthcheck
├── Makefile              # Development shortcuts
├── .env.example          # Environment variable template
└── go.mod
```

## Parity Testing

`testdata/parity/` holds fixtures generated by executing the legacy TypeScript
backend, not by reading it. The Go implementation must reproduce them. Two
divergences are deliberate and asserted as differences rather than matches:

- `last_month` date ranges, where the legacy `setMonth` arithmetic produced an
  inverted empty range on month-end dates
- `this_month` and `custom_month` boundaries, where the legacy code built local
  dates then formatted them via `toISOString()`, shifting each by one day

Both are recorded in `docs/DECISIONS.md`.

## Migration Planning

- `docs/MIGRATION_PLAN.md` — phased porting plan with per-phase definitions of done
- `docs/DECISIONS.md` — architecture decision records and open questions
- `docs/PARITY_CONTRACT.md` — behavioral contract and fixture strategy
