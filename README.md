<div align="center">

# Salus Bridge — Backend

**English** · [简体中文](README.zh-CN.md)

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-Azure%20%2B%20PostGIS-4169E1?logo=postgresql&logoColor=white)
[![CI](https://img.shields.io/github/actions/workflow/status/lotusink/salus-bridge-backend/go_ci.yml?label=CI&branch=main)](https://github.com/lotusink/salus-bridge-backend/actions/workflows/go_ci.yml)

Backend for **Salus Bridge** — an emergency rescue planning PWA (FIT5120 2026 S1).
A Go BFF fronts the React client and owns all business logic; a PostgreSQL/PostGIS
database holds the geographic, census, facility, disaster and risk data, built by a
Python data pipeline.

It supports rescue volunteers in disaster scenarios (bushfire · flood · earthquake)
with map-based situational overview, road-network route planning, real-time hazard-zone
alerts, and locating vulnerable people nearby.

</div>

---

> **About this repository.** This repo contains the backend of a 6-person
> Master's capstone project. I designed and implemented the **Go BFF service**
> end-to-end, and co-developed the **`database/pipeline`** with the project's
> data lead. Frontend and product design were delivered by other team members.

---

## Architecture

```text
React PWA  (:3000)
      │
      │  HTTP / WebSocket
      ▼
Go BFF  (:8080)  ──  main.go  +  internal/<engine>/
      │
      ├──►  Azure PostgreSQL / PostGIS   (pgx + sqlx · TLS required)
      │
      └──►  External APIs      (OpenAI · Anthropic · OpenRouteService · Nominatim)
```

> The BFF is the only entry point for the frontend. It owns all CORS,
> auth/session headers, and request validation — then reads/writes PostgreSQL
> directly or calls the external APIs above.
>
> The database is populated ahead of time by the Python pipeline under
> `database/pipeline/` (a build-time tool, not a running service).

---

## Repository Layout

```
emergency-rescue-planner/
├── database/
│   ├── README.md                     # Pipeline docs + run order
│   └── pipeline/                     # Python build-time DB pipeline
│       ├── schema.sql                # Consolidated DDL (reference)
│       ├── 0_*..3_* / Risk Score.py  # Create schema + load data (numbered order)
│       └── making_BTree_index.py     # Indexes
└── service/
    └── go/
        └── bff/                      # Go BFF — the service
            ├── main.go               # Route registration & startup
            ├── docs/                 # Swagger (swaggo/swag generated)
            └── internal/<engine>/    # One package per feature (info, route, conditions, …)
```

---

## Tech Stack

### Go BFF

| Component | Choice |
|---|---|
| Language | Go 1.26 |
| HTTP / routing | `net/http` |
| WebSocket | `gorilla/websocket` |
| PostgreSQL driver | `jackc/pgx/v5`, `jmoiron/sqlx` |
| Configuration | `joho/godotenv` |
| API documentation | `swaggo/swag` |

### Database pipeline

| Component | Choice |
|---|---|
| Language | Python 3.12 |
| Database | PostgreSQL + PostGIS |
| Access / geo I/O | SQLAlchemy · psycopg2 · geopandas · shapely |

See [`database/README.md`](emergency-rescue-planner/database/README.md) for setup
and run order.

---

## Local Development

### Go BFF

Create a `.env` file in the BFF directory containing the variables listed under
**Environment Variables** below, then:

```bash
cd emergency-rescue-planner/service/go/bff
go run main.go                # serves on $GO_SERVICE_PORT (default 8080)
go vet ./...                  # lint before pushing
go test ./...                 # unit tests
swag init                     # regenerate Swagger docs into ./docs
```

### Database

The BFF expects an already-populated PostgreSQL/PostGIS database. To build one,
follow [`database/README.md`](emergency-rescue-planner/database/README.md)
(`schema.sql` for the DDL; the numbered scripts to load data).

### Environment Variables

Required for the BFF:

| Variable | Purpose |
|---|---|
| `GO_SERVICE_PORT` | TCP port (default 8080) |
| `ENV` | `deployment` disables Swagger UI and switches CORS to `DEPLOYMENT_FRONTEND_URL`; any other value uses `LOCAL_FRONTEND_URL` |
| `DATABASE_URL` | PostgreSQL DSN — **must include `sslmode=require`** |
| `DEPLOYMENT_FRONTEND_URL` / `LOCAL_FRONTEND_URL` | CORS allowlist (exact origin, no wildcards) |

Optional (feature-gated — endpoints become unreachable when absent):

| Variable | Disables on absence |
|---|---|
| `OPENAI_API_KEY` | `/api/ai/*`, `/api/knowledge/{transcribe,tts,voice-search}` → 404 |
| `ANTHROPIC_API_KEY` | `/api/ai/*`, `/api/knowledge/{translate,voice-search}` → 404 |
| `ORS_API_KEY` | `/api/route/calculate` → 503 |

---

## API Surface

Routes are registered in `service/go/bff/main.go`. Grouped by feature:

| Group | Prefix | Notable endpoints |
|---|---|---|
| Health | `/health_check` | `GET /health_check` |
| Overview | `/api/overview/*` | `geogroup` (10-min cache), `facilities` |
| Routing | `/api/route/*` | ORS road-network route calculation |
| Field reports | `/api/field-reports/*` | submit · confirm · list (requires `X-Volunteer-Session`) |
| Conditions | `/api/conditions/risk-zones`, `/ws/hazards` | Risk zones + WS hazard channel (subprotocol `volunteerlink.hazards.v1`) |
| Vulnerable persons | `/api/vulnerable-persons` | Haversine-filtered |
| Active routes | `/api/routes/active/*` | register · delete · accept-reroute (session-scoped) |
| Knowledge | `/api/knowledge/*` | search · articles · STT · TTS · translate · voice-search |
| Checklist | `/api/checklist` | Disaster × disability lookup |
| Geocoding | `/api/geocode/*` | Nominatim proxy (search + reverse) |
| AI | `/api/ai/*` | chat · stream (SSE) |
| Demo | `/api/demo/*` | heartbeat · hazard-demo start/stop |
| WebSocket | `/ws`, `/ws/hazards` | echo · per-session hazard pushes |

**Full schema:** `GET /swagger/` (only when `ENV != "deployment"`).
