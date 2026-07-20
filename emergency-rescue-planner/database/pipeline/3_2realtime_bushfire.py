import os
from dotenv import load_dotenv

load_dotenv()

import requests
import geopandas as gpd
import pandas as pd
from sqlalchemy import create_engine, text

# =========================
# Connect to data base
# =========================


engine = create_engine(
    os.environ["DATABASE_URL"]
)

# =========================
# Get realtime fire data
# =========================

url = "https://services-ap1.arcgis.com/ypkPEy1AmwPKGNNv/arcgis/rest/services/Near_Real_Time_Bushfire_Boundaries_view/FeatureServer/3/query"

params = {
    "where": "1=1",
    "outFields": "*",
    "returnGeometry": "true",
    "f": "geojson"
}

response = requests.get(url, params=params)
geojson = response.json()

gdf = gpd.GeoDataFrame.from_features(
    geojson["features"],
    crs="EPSG:4326"
)

# =========================
# Data cleaning
# =========================

gdf = gdf[gdf["fire_id"].notna()]

gdf["fire_id"] = gdf["fire_id"].astype(str)

# Change data time
gdf["capt_date"] = pd.to_datetime(gdf["capt_date"], unit="ms", errors="coerce")
gdf["ignition_date"] = pd.to_datetime(gdf["ignition_date"], unit="ms", errors="coerce")

# Add snapshot time
gdf["snapshot_time"] = pd.Timestamp.now()

# =========================
# Load SA1 data
# =========================



gdf_sa1 = gpd.read_file(
    "Source data/0 - SA1_2021_AUST_SHP_GDA2020/SA1_2021_AUST_GDA2020.shp"
).to_crs("EPSG:4326")

# Spacial join to find sa1 of the fire

mapping = gpd.sjoin(
    gdf[["fire_id", "geometry"]],
    gdf_sa1[["SA1_CODE21", "geometry"]],
    how="inner",
    predicate="intersects"
)

# keep mapping table
mapping = mapping[["fire_id", "SA1_CODE21"]].drop_duplicates()

with engine.begin() as conn:
    conn.execute(text("""
        ALTER TABLE bushfire_events 
        ADD COLUMN IF NOT EXISTS extinguish_date TIMESTAMP
    """))
# =========================
# Check the fire state
# =========================

try:
    old_df = pd.read_sql("SELECT fire_id FROM bushfire_events", engine)
    old_ids = set(old_df["fire_id"].astype(str))
except:
    old_ids = set()

gdf["fire_id"] = gdf["fire_id"].astype(str)
new_ids = set(gdf["fire_id"])


new_fire_ids = new_ids - old_ids
existing_fire_ids = new_ids & old_ids
extinguished_ids = old_ids - new_ids

# =========================
# Update to data base
# =========================

# 1 Insert: Current Fire

gdf_new = gdf[gdf["fire_id"].isin(new_fire_ids)]

gdf_new.to_postgis(
    "bushfire_events",
    engine,
    if_exists="append",
    index=False
)

# 2 Update: Current Fire state


with engine.begin() as conn:
    for _, row in gdf[gdf["fire_id"].isin(existing_fire_ids)].iterrows():
        conn.execute(text("""
            UPDATE bushfire_events
            SET
                fire_name = :fire_name,
                fire_type = :fire_type,
                area_ha = :area_ha,
                perim_km = :perim_km,
                state = :state,
                agency = :agency,
                capt_date = :capt_date,
                snapshot_time = :snapshot_time
            WHERE fire_id = :fire_id
        """), row.where(pd.notnull(row), None).to_dict())

# 3 Update: extinguished fire
with engine.begin() as conn:
    conn.execute(text("""
        UPDATE bushfire_events
        SET extinguish_date = NOW()
        WHERE fire_id = ANY(CAST(:ids AS text[]))
    """), {"ids": list(extinguished_ids)})


# Update: SA1 mapping of fire
mapping.to_sql(
    "bushfire_sa1_mapping",
    engine,
    if_exists="replace",
    index=False
)

snapshot_cols = [
    "fire_id",
    "snapshot_time",
    "area_ha",
    "state",
    "geometry"
]
# Append: history snapshots
gdf[snapshot_cols].to_postgis(
    "bushfire_snapshots",
    engine,
    if_exists="append",
    index=False
)
