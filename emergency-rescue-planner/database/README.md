# Salus Bridge — Database Pipeline

Build-time Python scripts that create the PostgreSQL/PostGIS schema and load the
geographic, census, facility, disaster and risk data that the Go BFF queries at
runtime. This is **not** a running service — it is a one-off (and periodically
re-run) data pipeline. The Go BFF only ever talks to the resulting database.

> Directory was previously named `expermental/`; renamed to `pipeline/` since
> these scripts build the production database.

## Prerequisites

- PostgreSQL with the **PostGIS** extension available.
- Python 3.12.
- Source data files (ABS census G-tables, SA1 boundaries, facility lists,
  disaster histories). These are **not** included in the repo — the scripts
  expect them locally; see each script's `read_csv` / `read_excel` / `read_file`
  calls for the expected paths.

## Setup

```bash
cd emergency-rescue-planner/database/pipeline
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt          # slim runtime deps
cp .env.example .env                      # then fill in DATABASE_URL
```

`.env` must contain an Azure-style PostgreSQL DSN with TLS required:

```
DATABASE_URL=postgresql+psycopg2://<user>:<password>@<host>:5432/<database>?sslmode=require
```

Verify connectivity: `python connectDB.py` (prints the server version).

## Run order

The numeric prefixes are the intended execution order. `sa1_geo` must exist and
be populated before anything that foreign-keys to it.

| Step | Script | What it does |
|---|---|---|
| 0 | `0_create_table.py` | Enables PostGIS; creates the core tables (see `schema.sql`). |
| 0 | `0_sa1_geodata.py` | Loads SA1 boundary geometry into `sa1_geo`. |
| 1 | `1_census_data_wrangling.py` | Wrangles + loads ABS census G-tables (population, disability, assistance, vehicles). |
| 2 | `2_emergency_facility.py` | Loads official + OSM emergency/support facilities. |
| 3 | `3_1upload_historical_data.py` | Loads historical fire / flood / earthquake events and their SA1 mappings. |
| 3 | `3_2realtime_bushfire.py` | Loads the realtime bushfire feed (events / snapshots / mappings). |
| 4 | `Risk Score.py` | Computes and loads per-SA1 risk scores into `sa1_risk`. |
| 5 | `making_BTree_index.py` | Creates B-Tree indexes on `sa1_geo` hierarchy columns for drill-down queries. |

Helpers: `connectDB.py` (connectivity smoke test).

## Files

- `schema.sql` — consolidated read-only DDL reference (all tables in one place),
  extracted from the scripts. Includes a **known gap** note (below).
- `requirements.txt` — slim runtime dependencies.
- `requirements.freeze.txt` — original full environment freeze, kept verbatim.
- `.env.example` — DSN template.

## Known gap (pre-existing)

The live Go query `service/go/bff/internal/database/sql/groupgeov2.sql`
(endpoint `/api/overview/geogroup`) reads several tables that **no script in
this pipeline creates**: the simplified-geometry tables `geo_sa1`, `geo_sa2`,
`geo_sa3`, `geo_sa4`, `geo_state`, and `sa1_disease` + `needs_mapping`. The
checklist endpoint likewise needs a `pre_checklist` table
(`get_checklist.sql`). These must be provided separately for those endpoints to
return data. This gap existed in the original repository and is documented here
rather than silently ignored.
