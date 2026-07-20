-- ============================================================
--   :bbox_xmin   FLOAT
--   :bbox_ymin   FLOAT
--   :bbox_xmax   FLOAT
--   :bbox_ymax   FLOAT
-- ============================================================

WITH bbox AS (
    SELECT ST_MakeEnvelope(
        CAST(:bbox_xmin AS float8), CAST(:bbox_ymin AS float8),
        CAST(:bbox_xmax AS float8), CAST(:bbox_ymax AS float8),
        4326
    ) AS geom
)

-- official facilities
SELECT
    'official'                      AS source,
    f.category                      AS category,
    f.facility_name                AS name,
    ST_AsGeoJSON(ST_SetSRID(ST_MakePoint(f.lon, f.lat), 4326))    AS geometry

FROM emergency_facilities f, bbox b
WHERE ST_Intersects(
    ST_SetSRID(ST_MakePoint(f.lon, f.lat), 4326),
    b.geom
)

UNION ALL

-- OSM facilities
SELECT
    'osm'                           AS source,
    o.category                      AS category,
    o.name                          AS name,
    ST_AsGeoJSON(ST_SetSRID(ST_MakePoint(o.lon, o.lat), 4326)) AS geometry

FROM osm_support_facilities o, bbox b
WHERE ST_Intersects(
    ST_SetSRID(ST_MakePoint(o.lon, o.lat), 4326),
    b.geom
);