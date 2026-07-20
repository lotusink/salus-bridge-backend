-- ============================================================================
-- Salus Bridge — consolidated database schema (PostgreSQL + PostGIS)
--
-- This file is a READ-ONLY REFERENCE consolidating the DDL that is otherwise
-- scattered across the pipeline scripts, so the full table layout can be seen
-- in one place. Source of truth remains the Python scripts (they also load
-- data); this mirror is extracted from:
--   - 0_create_table.py     (extension + core tables)
--   - Risk Score.py         (sa1_risk)
--   - 3_2realtime_bushfire.py (bushfire_events ALTER)
--   - making_BTree_index.py (sa1_geo indexes)
--
-- You CAN run this to create an empty schema, then run the numbered scripts to
-- load data. Order matters: sa1_geo must exist before the tables that FK to it.
--
-- ⚠️ KNOWN GAP (pre-existing, not introduced by cleanup): the live Go query
--    internal/database/sql/groupgeov2.sql references these tables that NO
--    pipeline script creates — you must provide them separately before that
--    endpoint (/api/overview/geogroup) returns data:
--      geo_sa1, geo_sa2, geo_sa3, geo_sa4, geo_state  (simplified geometry)
--      sa1_disease, needs_mapping                     (disease breakdown)
--    The checklist endpoint additionally needs a `pre_checklist` table
--    (see internal/database/sql/get_checklist.sql).
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS postgis;

-- ----------------------------------------------------------------------------
-- Geography backbone (every other table FKs to sa1_geo)
-- ----------------------------------------------------------------------------
CREATE TABLE sa1_geo (
    sa1_code21      VARCHAR(11) PRIMARY KEY,
    sa2_code21      VARCHAR(9),
    sa2_name        VARCHAR(200),
    sa3_code21      VARCHAR(7),
    sa3_name        VARCHAR(200),
    sa4_code21      VARCHAR(5),
    sa4_name        VARCHAR(200),
    gccsa_code21    VARCHAR(5),
    gccsa_name      VARCHAR(200),
    state_code21    VARCHAR(2),
    state_name      VARCHAR(100),
    aus_code21      VARCHAR(10),
    aus_name        VARCHAR(100),
    area_sqkm       DOUBLE PRECISION,
    center_lat      DOUBLE PRECISION,
    center_lon      DOUBLE PRECISION,
    geometry        geometry(MULTIPOLYGON, 4326)
);

-- ----------------------------------------------------------------------------
-- Census / population (ABS G-tables)
-- ----------------------------------------------------------------------------
CREATE TABLE population_g01 (
    sa1_code21          VARCHAR(11),
    population_count    INTEGER,
    age_group           VARCHAR(50),
    gender              VARCHAR(20),
    data_date           DATE,
    PRIMARY KEY (sa1_code21, age_group, gender, data_date),
    FOREIGN KEY (sa1_code21) REFERENCES sa1_geo(sa1_code21)
);

CREATE TABLE population_language_g09 (
    sa1_code21          VARCHAR(11),
    population_count    INTEGER,
    country             VARCHAR(100),
    primary_language    VARCHAR(100),
    data_date           DATE,
    PRIMARY KEY (sa1_code21, country, primary_language, data_date),
    FOREIGN KEY (sa1_code21) REFERENCES sa1_geo(sa1_code21)
);

CREATE TABLE disability_g18 (
    sa1_code21          VARCHAR(11),
    population_count    INTEGER,
    gender              VARCHAR(20),
    age_group           VARCHAR(50),
    data_date           DATE,
    PRIMARY KEY (sa1_code21, gender, age_group, data_date),
    FOREIGN KEY (sa1_code21) REFERENCES sa1_geo(sa1_code21)
);

CREATE TABLE assistance_g25 (
    sa1_code21          VARCHAR(11),
    population_count    INTEGER,
    gender              VARCHAR(20),
    data_date           DATE,
    PRIMARY KEY (sa1_code21, gender, data_date),
    FOREIGN KEY (sa1_code21) REFERENCES sa1_geo(sa1_code21)
);

CREATE TABLE family_vehicles_g34 (
    sa1_code21      VARCHAR(11),
    count           INTEGER,
    num_vehicles    VARCHAR(20),
    data_date       DATE,
    PRIMARY KEY (sa1_code21, num_vehicles, data_date),
    FOREIGN KEY (sa1_code21) REFERENCES sa1_geo(sa1_code21)
);

-- ----------------------------------------------------------------------------
-- Emergency facilities
-- ----------------------------------------------------------------------------
CREATE TABLE emergency_facilities (
    facility_id     SERIAL PRIMARY KEY,
    category        VARCHAR(50),
    facility_name   VARCHAR(200),
    state           VARCHAR(50),
    address         VARCHAR(300),
    postcode        VARCHAR(4),
    suburb          VARCHAR(100),
    lat             DOUBLE PRECISION,
    lon             DOUBLE PRECISION,
    sa1_code21      VARCHAR(11),
    FOREIGN KEY (sa1_code21) REFERENCES sa1_geo(sa1_code21)
);

CREATE TABLE osm_support_facilities (
    id              BIGINT PRIMARY KEY,
    type            VARCHAR(50),
    lat             DOUBLE PRECISION NOT NULL,
    lon             DOUBLE PRECISION NOT NULL,
    category        VARCHAR(50) NOT NULL,
    name            VARCHAR(255),
    phone           VARCHAR(50),
    fax             VARCHAR(50),
    source          VARCHAR(100) NOT NULL,
    sa1_code21      VARCHAR(11) NOT NULL,
    web             VARCHAR(500),
    FOREIGN KEY (sa1_code21) REFERENCES sa1_geo(sa1_code21)
);

-- ----------------------------------------------------------------------------
-- Historical disaster events + SA1 mappings
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS bushfire_history (
    id              SERIAL PRIMARY KEY,
    fire_id         TEXT,
    fire_name       TEXT,
    ignition_date   TIMESTAMPTZ,
    capture_date    TIMESTAMPTZ,
    extinguish_date TIMESTAMPTZ,
    fire_type       TEXT,
    ignition_cause  TEXT,
    capt_method     TEXT,
    area_ha         INTEGER,
    perim_km        INTEGER,
    state           TEXT,
    agency          TEXT,
    year            INTEGER,
    geometry        GEOMETRY(GEOMETRY, 4326)
);

CREATE TABLE IF NOT EXISTS flood_history (
    id          SERIAL PRIMARY KEY,
    flood_id    TEXT,
    name        TEXT,
    year        INTEGER,
    commission  TEXT,
    lead_consu  TEXT,
    rivers      TEXT,
    state       TEXT,
    abstract    TEXT,
    geometry    GEOMETRY(POINT, 4326)
);

CREATE TABLE IF NOT EXISTS earthquake_history (
    id       SERIAL PRIMARY KEY,
    eq_id    TEXT,
    time     TIMESTAMP,
    mag      FLOAT,
    place    TEXT,
    lon      DOUBLE PRECISION NOT NULL,
    lat      DOUBLE PRECISION NOT NULL,
    depth    FLOAT,
    tsunami  INTEGER,
    year     INTEGER,
    geometry GEOMETRY(POINT, 4326)
);

CREATE TABLE IF NOT EXISTS fire_sa1_mapping (
    id          SERIAL PRIMARY KEY,
    history_id  INTEGER NOT NULL REFERENCES bushfire_history(id),
    sa1_code21  TEXT    NOT NULL REFERENCES sa1_geo(sa1_code21)
);

CREATE TABLE IF NOT EXISTS flood_sa1_mapping (
    id          SERIAL PRIMARY KEY,
    history_id  INTEGER NOT NULL REFERENCES flood_history(id),
    sa1_code21  TEXT    NOT NULL REFERENCES sa1_geo(sa1_code21)
);

CREATE TABLE IF NOT EXISTS earthquake_sa1_mapping (
    id          SERIAL PRIMARY KEY,
    history_id  INTEGER NOT NULL REFERENCES earthquake_history(id),
    sa1_code21  TEXT    NOT NULL REFERENCES sa1_geo(sa1_code21)
);

-- ----------------------------------------------------------------------------
-- Realtime bushfire feed (events / mappings / snapshots)
-- ----------------------------------------------------------------------------
CREATE TABLE bushfire_events (
    fire_id         TEXT PRIMARY KEY,
    fire_name       TEXT,
    fire_type       TEXT,
    ignition_date   TIMESTAMPTZ,
    capt_date       TIMESTAMPTZ,
    capt_method     TEXT,
    area_ha         FLOAT,
    perim_km        FLOAT,
    state           TEXT,
    agency          TEXT,
    snapshot_time   TIMESTAMPTZ,
    extinguish_date TIMESTAMPTZ,
    geometry        GEOMETRY(MULTIPOLYGON, 4326)
);
-- 3_2realtime_bushfire.py additionally ensures this column exists:
ALTER TABLE bushfire_events ADD COLUMN IF NOT EXISTS extinguish_date TIMESTAMP;

CREATE TABLE IF NOT EXISTS bushfire_sa1_mapping (
    fire_id     TEXT,
    sa1_code21  VARCHAR(11),
    FOREIGN KEY (fire_id) REFERENCES bushfire_events(fire_id),
    FOREIGN KEY (sa1_code21) REFERENCES sa1_geo(sa1_code21)
);

CREATE TABLE IF NOT EXISTS bushfire_snapshots (
    fire_id         TEXT,
    snapshot_time   TIMESTAMP,
    area_ha         DOUBLE PRECISION,
    state           TEXT,
    geometry        GEOMETRY(MULTIPOLYGON, 4326),
    FOREIGN KEY (fire_id) REFERENCES bushfire_events(fire_id)
);

-- ----------------------------------------------------------------------------
-- Risk scoring (Risk Score.py)
-- ----------------------------------------------------------------------------
CREATE TABLE sa1_risk (
    id                     SERIAL PRIMARY KEY,
    sa1_code21             TEXT,
    fire_aep               FLOAT,
    flood_aep              FLOAT,
    eq_aep                 FLOAT,
    svi                    FLOAT,
    lack_resilience        FLOAT,
    bushfire_risk          FLOAT,
    flood_risk             FLOAT,
    earthquake_risk        FLOAT,
    bushfire_risk_norm     FLOAT,
    flood_risk_norm        FLOAT,
    earthquake_risk_norm   FLOAT,
    bushfire_risk_level    TEXT,
    flood_risk_level       TEXT,
    earthquake_risk_level  TEXT,
    overall_risk_norm      FLOAT,
    overall_risk_level     TEXT,
    CONSTRAINT fk_sa1 FOREIGN KEY (sa1_code21)
        REFERENCES sa1_geo(sa1_code21) ON DELETE CASCADE
);

-- ----------------------------------------------------------------------------
-- Indexes (making_BTree_index.py) — drill-down hierarchy lookups on sa1_geo
-- ----------------------------------------------------------------------------
CREATE INDEX IF NOT EXISTS idx_sa1_geo_state_code21 ON sa1_geo (state_code21);
CREATE INDEX IF NOT EXISTS idx_sa1_geo_sa4_code21   ON sa1_geo (sa4_code21);
CREATE INDEX IF NOT EXISTS idx_sa1_geo_sa3_code21   ON sa1_geo (sa3_code21);
CREATE INDEX IF NOT EXISTS idx_sa1_geo_sa2_code21   ON sa1_geo (sa2_code21);
