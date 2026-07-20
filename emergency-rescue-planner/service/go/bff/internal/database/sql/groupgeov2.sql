-- ============================================================
-- Parameter
--   :target_date      DATE           e.g. '2026-04-16'
--   :agg_level        TEXT           'sa1' | 'sa2' | 'sa3' | 'sa4' | 'state'
--   :parent_level     TEXT | NULL    'state' | 'sa4' | 'sa3' | 'sa2'
--   :parent_code      TEXT | NULL
-- ============================================================

WITH

base_sa1 AS (
    SELECT
        sa1_code21,
        sa2_code21,
        sa2_name,
        sa3_code21,
        sa3_name,
        sa4_code21,
        sa4_name,
        state_code21,
        state_name,

        CASE CAST(:agg_level AS text)
            WHEN 'sa1'   THEN sa1_code21
            WHEN 'sa2'   THEN sa2_code21
            WHEN 'sa3'   THEN sa3_code21
            WHEN 'sa4'   THEN sa4_code21
            WHEN 'state' THEN state_code21
        END AS agg_key,

        CASE CAST(:agg_level AS text)
            WHEN 'sa1'   THEN sa2_name
            WHEN 'sa2'   THEN sa2_name
            WHEN 'sa3'   THEN sa3_name
            WHEN 'sa4'   THEN sa4_name
            WHEN 'state' THEN state_name
        END AS agg_name

    FROM sa1_geo
    WHERE
        :parent_code IS NULL
        OR (
            CASE CAST(:parent_level AS text)
                WHEN 'state' THEN state_code21
                WHEN 'sa4'   THEN sa4_code21
                WHEN 'sa3'   THEN sa3_code21
                WHEN 'sa2'   THEN sa2_code21
            END
        ) = CAST(:parent_code AS text)
),

selected_keys AS (
    SELECT DISTINCT agg_key
    FROM base_sa1
),

geo_source AS (
    SELECT
        'sa1' AS agg_level,
        g.sa1_code21 AS agg_key,
        s.sa2_name AS area_name,
        g.geometry_simplified AS geometry
    FROM geo_sa1 g
    JOIN sa1_geo s
        ON s.sa1_code21 = g.sa1_code21

    UNION ALL

    SELECT
        'sa2' AS agg_level,
        g.sa2_code21 AS agg_key,
        g.sa2_name AS area_name,
        g.geometry_simplified AS geometry
    FROM geo_sa2 g

    UNION ALL

    SELECT
        'sa3' AS agg_level,
        g.sa3_code21 AS agg_key,
        g.sa3_name AS area_name,
        g.geometry_simplified AS geometry
    FROM geo_sa3 g

    UNION ALL

    SELECT
        'sa4' AS agg_level,
        g.sa4_code21 AS agg_key,
        g.sa4_name AS area_name,
        g.geometry_simplified AS geometry
    FROM geo_sa4 g

    UNION ALL

    SELECT
        'state' AS agg_level,
        g.state_code21 AS agg_key,
        g.state_name AS area_name,
        g.geometry_simplified AS geometry
    FROM geo_state g
),

geo_agg AS (
    SELECT
        gs.agg_key,
        gs.area_name,
        gs.geometry
    FROM geo_source gs
    JOIN selected_keys k
        ON k.agg_key = gs.agg_key
    WHERE gs.agg_level = CAST(:agg_level AS text)
),

disability_agg AS (
    SELECT
        b.agg_key,
        SUM(d.population_count) AS tot_disability
    FROM disability_g18 d
    JOIN base_sa1 b USING (sa1_code21)
    WHERE d.age_group = 'Tot'
    GROUP BY b.agg_key
),

facility_official AS (
    SELECT
        b.agg_key,
        COUNT(*) AS official_total,
        COUNT(*) FILTER (WHERE f.category = 'POLICING') AS policing,
        COUNT(*) FILTER (WHERE f.category = 'AMBULANCE') AS ambulance,
        COUNT(*) FILTER (WHERE f.category = 'FIRE SERVICE') AS fire_service,
        COUNT(*) FILTER (WHERE f.category = 'SES') AS ses,
        COUNT(*) FILTER (WHERE f.category = 'OTHER') AS other_official
    FROM emergency_facilities f
    JOIN base_sa1 b USING (sa1_code21)
    GROUP BY b.agg_key
),

facility_osm AS (
    SELECT
        b.agg_key,
        COUNT(*) AS osm_total,
        COUNT(*) FILTER (WHERE o.category = 'hospital') AS hospital,
        COUNT(*) FILTER (WHERE o.category = 'fire_hydrant') AS fire_hydrant,
        COUNT(*) FILTER (WHERE o.category = 'open_space') AS shelter
    FROM osm_support_facilities o
    JOIN base_sa1 b USING (sa1_code21)
    GROUP BY b.agg_key
),

risk_agg AS (
    SELECT
        b.agg_key,
        AVG(r.bushfire_risk_norm) AS avg_bushfire_risk,
        AVG(r.flood_risk_norm) AS avg_flood_risk,
        AVG(r.earthquake_risk_norm) AS avg_earthquake_risk,
        AVG(r.overall_risk_norm) AS avg_overall_risk
    FROM sa1_risk r
    JOIN base_sa1 b USING (sa1_code21)
    GROUP BY b.agg_key
),

target_hist_date AS (
    SELECT
        (CAST(:target_date AS date) - INTERVAL '6 years')::date AS hist_date
),

fire_window AS (
    SELECT
        b.agg_key,
        COUNT(DISTINCT m.history_id) AS fire_count
    FROM fire_sa1_mapping m
    JOIN bushfire_history h ON h.id = m.history_id
    JOIN base_sa1 b ON b.sa1_code21 = m.sa1_code21
    WHERE h.ignition_date::date BETWEEN
        (SELECT hist_date - 3 FROM target_hist_date)
        AND
        (SELECT hist_date + 3 FROM target_hist_date)
    GROUP BY b.agg_key
),

fire_fallback_date AS (
    SELECT MAX(h.ignition_date::date) AS fallback_date
    FROM bushfire_history h
    WHERE h.ignition_date::date <= (SELECT hist_date FROM target_hist_date)
      AND NOT EXISTS (
          SELECT 1
          FROM fire_sa1_mapping m2
          JOIN bushfire_history h2 ON h2.id = m2.history_id
          WHERE h2.ignition_date::date BETWEEN
              (SELECT hist_date - 3 FROM target_hist_date)
              AND
              (SELECT hist_date + 3 FROM target_hist_date)
      )
),

fire_fallback AS (
    SELECT
        b.agg_key,
        COUNT(DISTINCT m.history_id) AS fire_count
    FROM fire_sa1_mapping m
    JOIN bushfire_history h ON h.id = m.history_id
    JOIN base_sa1 b ON b.sa1_code21 = m.sa1_code21
    JOIN fire_fallback_date fd
        ON h.ignition_date::date = fd.fallback_date
    GROUP BY b.agg_key
),

fire_final AS (
    SELECT agg_key, fire_count FROM fire_window
    UNION ALL
    SELECT agg_key, fire_count FROM fire_fallback
    WHERE NOT EXISTS (SELECT 1 FROM fire_window)
),

flood_window AS (
    SELECT
        b.agg_key,
        COUNT(DISTINCT m.history_id) AS flood_count
    FROM flood_sa1_mapping m
    JOIN flood_history h ON h.id = m.history_id
    JOIN base_sa1 b ON b.sa1_code21 = m.sa1_code21
    WHERE h.year = EXTRACT(YEAR FROM (SELECT hist_date FROM target_hist_date))::int - 6
    GROUP BY b.agg_key
),

flood_fallback_date AS (
    SELECT MAX(h.year) AS fallback_year
    FROM flood_history h
    WHERE h.year <= EXTRACT(YEAR FROM (SELECT hist_date FROM target_hist_date))::int - 6
      AND NOT EXISTS (SELECT 1 FROM flood_window)
),

flood_fallback AS (
    SELECT
        b.agg_key,
        COUNT(DISTINCT m.history_id) AS flood_count
    FROM flood_sa1_mapping m
    JOIN flood_history h ON h.id = m.history_id
    JOIN base_sa1 b ON b.sa1_code21 = m.sa1_code21
    JOIN flood_fallback_date fd
        ON h.year = fd.fallback_year
    GROUP BY b.agg_key
),

flood_final AS (
    SELECT agg_key, flood_count FROM flood_window
    UNION ALL
    SELECT agg_key, flood_count FROM flood_fallback
    WHERE NOT EXISTS (SELECT 1 FROM flood_window)
),

eq_window AS (
    SELECT
        b.agg_key,
        COUNT(DISTINCT m.history_id) AS eq_count
    FROM earthquake_sa1_mapping m
    JOIN earthquake_history h ON h.id = m.history_id
    JOIN base_sa1 b ON b.sa1_code21 = m.sa1_code21
    WHERE h.time::date BETWEEN
        (SELECT hist_date - 3 FROM target_hist_date)
        AND
        (SELECT hist_date + 3 FROM target_hist_date)
    GROUP BY b.agg_key
),

eq_fallback_date AS (
    SELECT MAX(h.time::date) AS fallback_date
    FROM earthquake_history h
    WHERE h.time::date <= (SELECT hist_date FROM target_hist_date)
      AND NOT EXISTS (SELECT 1 FROM eq_window)
),

eq_fallback AS (
    SELECT
        b.agg_key,
        COUNT(DISTINCT m.history_id) AS eq_count
    FROM earthquake_sa1_mapping m
    JOIN earthquake_history h ON h.id = m.history_id
    JOIN base_sa1 b ON b.sa1_code21 = m.sa1_code21
    JOIN eq_fallback_date fd
        ON h.time::date = fd.fallback_date
    GROUP BY b.agg_key
),

eq_final AS (
    SELECT agg_key, eq_count FROM eq_window
    UNION ALL
    SELECT agg_key, eq_count FROM eq_fallback
    WHERE NOT EXISTS (SELECT 1 FROM eq_window)
),

disease_agg AS (
    SELECT
        b.agg_key,
        dis.type,
        dis.category,
        SUM(dis.estimated_count) AS estimated_count,
        SUM(dis.total_sa1) AS total_population,
        CASE
            WHEN SUM(dis.total_sa1) = 0 THEN 0
            ELSE SUM(dis.estimated_count) / SUM(dis.total_sa1)
        END AS proportion
    FROM sa1_disease dis
    JOIN base_sa1 b USING (sa1_code21)
    WHERE dis.type NOT IN (
        'Other Physical',
        'Other Sensory/Speech',
        'Other Neurological',
        'Other'
    )
    GROUP BY b.agg_key, dis.type, dis.category
),

disease_with_needs AS (
    SELECT
        d.agg_key,
        d.type,
        d.category,
        d.estimated_count,
        d.total_population,
        d.proportion,
        n.needs,
        n.power_dependent
    FROM disease_agg d
    LEFT JOIN needs_mapping n
        ON LOWER(d.type) = LOWER(n.type)
),

disease_needs_agg AS (
    SELECT
        agg_key,
        type,
        category,
        estimated_count,
        total_population,
        proportion,
        JSON_AGG(
            JSON_BUILD_OBJECT(
                'need', needs,
                'power_dependent', power_dependent
            )
        ) AS needs_list
    FROM disease_with_needs
    GROUP BY
        agg_key,
        type,
        category,
        estimated_count,
        total_population,
        proportion
),

disease_json AS (
    SELECT
        agg_key,
        JSON_AGG(
            JSON_BUILD_OBJECT(
                'type', type,
                'category', category,
                'estimated_count', ROUND(CAST(estimated_count AS numeric), 2),
                'total_population', ROUND(CAST(total_population AS numeric), 2),
                'proportion', ROUND(CAST(proportion AS numeric), 6),
                'needs', COALESCE(needs_list, '[]'::json)
            )
            ORDER BY proportion DESC
        ) AS disease_breakdown
    FROM disease_needs_agg
    GROUP BY agg_key
)

SELECT
    g.agg_key AS area_code,
    g.area_name,

    CAST(ST_AsGeoJSON(g.geometry) AS json) AS geometry,

    COALESCE(d.tot_disability, 0) AS tot_disability,

    COALESCE(fo.official_total, 0) + COALESCE(os.osm_total, 0) AS total_facilities,
    COALESCE(fo.policing, 0) AS policing,
    COALESCE(fo.ambulance, 0) AS ambulance,
    COALESCE(fo.fire_service, 0) AS fire_service,
    COALESCE(fo.ses, 0) AS ses,
    COALESCE(fo.other_official, 0) AS other_official,

    COALESCE(os.hospital, 0) AS hospital,
    COALESCE(os.fire_hydrant, 0) AS fire_hydrant,
    COALESCE(os.shelter, 0) AS shelter,

    COALESCE(r.avg_bushfire_risk, 0) AS avg_bushfire_risk,
    COALESCE(r.avg_flood_risk, 0) AS avg_flood_risk,
    COALESCE(r.avg_earthquake_risk, 0) AS avg_earthquake_risk,
    COALESCE(r.avg_overall_risk, 0) AS avg_overall_risk,

    COALESCE(f.fire_count, 0) AS historical_fire_count,
    COALESCE(fl.flood_count, 0) AS historical_flood_count,
    COALESCE(eq.eq_count, 0) AS historical_eq_count,

    COALESCE(dj.disease_breakdown, '[]'::json) AS disease_breakdown

FROM geo_agg g
LEFT JOIN disability_agg d ON d.agg_key = g.agg_key
LEFT JOIN facility_official fo ON fo.agg_key = g.agg_key
LEFT JOIN facility_osm os ON os.agg_key = g.agg_key
LEFT JOIN risk_agg r ON r.agg_key = g.agg_key
LEFT JOIN fire_final f ON f.agg_key = g.agg_key
LEFT JOIN flood_final fl ON fl.agg_key = g.agg_key
LEFT JOIN eq_final eq ON eq.agg_key = g.agg_key
LEFT JOIN disease_json dj ON dj.agg_key = g.agg_key

ORDER BY g.agg_key;