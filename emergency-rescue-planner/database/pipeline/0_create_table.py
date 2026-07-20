import os
from dotenv import load_dotenv

load_dotenv()

from sqlalchemy import create_engine, text

engine = create_engine(
    os.environ["DATABASE_URL"]
)


with engine.begin() as conn:
    # 1. Startpostgis
    conn.execute(text("CREATE EXTENSION IF NOT EXISTS postgis;"))


    # 2. geo_sa1
    conn.execute(text("""
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
    """))

    # 3. population_g01
    conn.execute(text("""
    CREATE TABLE population_g01 (
        sa1_code21          VARCHAR(11),
        population_count    INTEGER,
        age_group           VARCHAR(50),
        gender              VARCHAR(20),
        data_date           DATE,
        PRIMARY KEY (sa1_code21, age_group, gender, data_date),
        FOREIGN KEY (sa1_code21) REFERENCES sa1_geo(sa1_code21)
    );
    """))

    # 5. population_language_g09
    conn.execute(text("""
    CREATE TABLE population_language_g09 (
        sa1_code21          VARCHAR(11),
        population_count    INTEGER,
        country             VARCHAR(100),
        primary_language    VARCHAR(100),
        data_date           DATE,
        PRIMARY KEY (sa1_code21, country, primary_language, data_date),
        FOREIGN KEY (sa1_code21) REFERENCES sa1_geo(sa1_code21)
    );
    """))


    # 6. disability_g18
    conn.execute(text("""
    CREATE TABLE disability_g18 (
        sa1_code21          VARCHAR(11),
        population_count    INTEGER,
        gender              VARCHAR(20),
        age_group           VARCHAR(50),
        data_date           DATE,
        PRIMARY KEY (sa1_code21, gender, age_group, data_date),
        FOREIGN KEY (sa1_code21) REFERENCES sa1_geo(sa1_code21)
    );
    """))


    # 7. assistance_g25
    conn.execute(text("""
    CREATE TABLE assistance_g25 (
        sa1_code21          VARCHAR(11),
        population_count    INTEGER,
        gender              VARCHAR(20),
        data_date           DATE,
        PRIMARY KEY (sa1_code21, gender, data_date),
        FOREIGN KEY (sa1_code21) REFERENCES sa1_geo(sa1_code21)
    );
    """))


    # 8. family_vehicles_g34
    conn.execute(text("""
    CREATE TABLE family_vehicles_g34 (
        sa1_code21      VARCHAR(11),
        count           INTEGER,
        num_vehicles    VARCHAR(20),
        data_date       DATE,
        PRIMARY KEY (sa1_code21, num_vehicles, data_date),
        FOREIGN KEY (sa1_code21) REFERENCES sa1_geo(sa1_code21)
    );
    """))


    # =========================
    # Emergency facility
    # =========================

    # 1. emergency_facility
    conn.execute(text("""
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
    """))

    # 2. OSM support facilities
    conn.execute(text("""
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
    """))

    # Fire historical data
    conn.execute(text("""
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
        )
    """))

    # Flood
    conn.execute(text("""
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
        )
    """))

    # Earthquake
    conn.execute(text("""
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
        )
    """))

    # Fire SA1 mapping
    conn.execute(text("""
        CREATE TABLE IF NOT EXISTS fire_sa1_mapping (
            id          SERIAL PRIMARY KEY,
            history_id  INTEGER NOT NULL REFERENCES bushfire_history(id),
            sa1_code21  TEXT    NOT NULL REFERENCES sa1_geo(sa1_code21)
        )
    """))

    # Flood SA1 mapping
    conn.execute(text("""
        CREATE TABLE IF NOT EXISTS flood_sa1_mapping (
            id          SERIAL PRIMARY KEY,
            history_id  INTEGER NOT NULL REFERENCES flood_history(id),
            sa1_code21  TEXT    NOT NULL REFERENCES sa1_geo(sa1_code21)
        )
    """))

    # Earthquake SA1 mapping
    conn.execute(text("""
        CREATE TABLE IF NOT EXISTS earthquake_sa1_mapping (
            id          SERIAL PRIMARY KEY,
            history_id  INTEGER NOT NULL REFERENCES earthquake_history(id),
            sa1_code21  TEXT    NOT NULL REFERENCES sa1_geo(sa1_code21)
        )
    """))

    # =========================
    # Disaster bush fire
    # =========================


    # 1. bushfire_events
    conn.execute(text("""
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
    """))

    # 2. Bushfire and sa1 mapping
    conn.execute(text("""
    CREATE TABLE IF NOT EXISTS bushfire_sa1_mapping (
        fire_id TEXT,
        sa1_code21 VARCHAR(11),
        FOREIGN KEY (fire_id) REFERENCES bushfire_events(fire_id),
        FOREIGN KEY (sa1_code21) REFERENCES sa1_geo(sa1_code21)

    );
    """))

    # 3. bushfire snapshots
    conn.execute(text("""
    CREATE TABLE IF NOT EXISTS bushfire_snapshots (
        fire_id TEXT,
        snapshot_time TIMESTAMP,
        area_ha DOUBLE PRECISION,
        state TEXT,
        geometry GEOMETRY(MULTIPOLYGON, 4326),
        FOREIGN KEY (fire_id) REFERENCES bushfire_events(fire_id)
    );
    """))








