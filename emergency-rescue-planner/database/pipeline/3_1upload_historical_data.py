


import os
from dotenv import load_dotenv

load_dotenv()

import pandas as pd
import geopandas as gpd
from sqlalchemy import create_engine, text

# Set engine
engine = create_engine(
    os.environ["DATABASE_URL"]
)


# In[3]:


# =========================
# Get SA1 geometry
# =========================
gdf_sa1 = gpd.read_postgis(
    "SELECT sa1_code21, geometry FROM sa1_geo",
    engine,
    geom_col="geometry"
).to_crs("EPSG:4326")


# In[7]:


# =========================
# Part 1: Fire disaster historical data
# =========================

gdf_fire = gpd.read_file("Source data/4-1 Historical_Bushfire_Boundaries_Version_2.0.geojson")

gdf_fire = gdf_fire[gdf_fire["geometry"].notna()]
gdf_fire = gdf_fire[gdf_fire["ignition_date"].notna()]
gdf_fire = gdf_fire[gdf_fire["area_ha"] > 0]

# Get year
gdf_fire["year"] = gdf_fire["ignition_date"].dt.year

gdf_fire = gdf_fire.to_crs("EPSG:4326")


# In[10]:


# =========================
# Part 2: Flood disaster historical data
# =========================

gdf_flood = gpd.read_file("Source data/5-1 flood_study_summary_3ce61_-8922895210507100045.geojson")

gdf_flood = gdf_flood[gdf_flood["geometry"].notna()]
gdf_flood = gdf_flood.to_crs("EPSG:4326")


# In[19]:


print(gdf_flood.dtypes)
print(flood_joined.dtypes)


# In[16]:


# =========================
# Part 3: Calculate earthquake disaster risk
# =========================

gdf_earthquake = gpd.read_file("Source data/6-1 australia_earthquakes_last11years.csv")

# 

gdf_eq = gdf_earthquake.copy()

gdf_eq["time"] = pd.to_numeric(gdf_eq["time"], errors="coerce")
gdf_eq["time"] = pd.to_datetime(gdf_eq["time"], unit="ms", errors="coerce")
gdf_eq["mag"] = pd.to_numeric(gdf_eq["mag"], errors="coerce")
gdf_eq["lon"] = pd.to_numeric(gdf_eq["lon"], errors="coerce")
gdf_eq["lat"] = pd.to_numeric(gdf_eq["lat"], errors="coerce")
gdf_eq["depth"] = pd.to_numeric(gdf_eq["depth"], errors="coerce")
gdf_eq["tsunami"] = pd.to_numeric(gdf_eq["tsunami"], errors="coerce")
gdf_eq["year"] = gdf_eq["time"].dt.year

# Remove na
gdf_eq = gdf_eq.dropna(subset=["lon", "lat", "time"])

gdf_eq = gpd.GeoDataFrame(
    gdf_eq,
    geometry=gpd.points_from_xy(gdf_eq["lon"], gdf_eq["lat"]),
    crs="EPSG:4326"
)

gdf_eq = gdf_eq.reset_index(drop=True)
gdf_eq["eq_id"] = (gdf_eq.index + 1).astype(str)


# In[17]:


print(gdf_eq.dtypes)
print(eq_joined.dtypes)


# In[26]:


# =========================
# Upload history data
# =========================

# Fire
fire_upload = gdf_fire[[
    "fire_id", "fire_name", "ignition_date", "capture_date", "extinguish_date",
    "fire_type", "ignition_cause", "capt_method", "area_ha", "perim_km",
    "state", "agency", "year", "geometry"
]].copy()
fire_upload.columns = [
    "fire_id", "fire_name", "ignition_date", "capture_date", "extinguish_date",
    "fire_type", "ignition_cause", "capt_method", "area_ha", "perim_km",
    "state", "agency", "year", "geometry"
]
# NaT → None
fire_upload = fire_upload.where(pd.notnull(fire_upload), None)
fire_upload.to_postgis("bushfire_history", engine, if_exists="append", index=False)
print(f"Fire record: {len(fire_upload)} ")

# Flood
flood_upload = gdf_flood[[
    "id", "name", "year", "commission", "lead_consu", "rivers", "state", "abstract", "geometry"
]].copy()
flood_upload.columns = [
    "flood_id", "name", "year", "commission", "lead_consu", "rivers", "state", "abstract", "geometry"
]
flood_upload = flood_upload.where(pd.notnull(flood_upload), None)
flood_upload.to_postgis("flood_history", engine, if_exists="append", index=False)
print(f"Flood record: {len(flood_upload)} ")

#  Earthquake
eq_upload = gdf_eq[[
    "eq_id", "time", "mag", "place", "lon", "lat", "depth", "tsunami", "year", "geometry"
]].copy()
eq_upload = eq_upload.where(pd.notnull(eq_upload), None)
eq_upload.to_postgis("earthquake_history", engine, if_exists="append", index=False)
print(f"Earthquake record: {len(eq_upload)}")


# In[29]:


# Get data from the database
fire_data = gpd.read_postgis(
    "SELECT id, geometry FROM bushfire_history",
    engine,
    geom_col="geometry"
).to_crs("EPSG:4326")


flood_data = gpd.read_postgis(
    "SELECT id, geometry FROM flood_history",
    engine,
    geom_col="geometry"
).to_crs("EPSG:4326")

earthquake_data = gpd.read_postgis(
    "SELECT id, geometry FROM earthquake_history",
    engine,
    geom_col="geometry"
).to_crs("EPSG:4326")


# In[31]:


# Find SA1 of each fire
fire_joined = gpd.sjoin(
    fire_data[["id", "geometry"]],
    gdf_sa1[["sa1_code21", "geometry"]],
    how="inner",
    predicate="intersects"
)

# Find SA1 of each flood
flood_joined = gpd.sjoin(
    flood_data[["id", "geometry"]],
    gdf_sa1[["sa1_code21", "geometry"]],
    how="inner",
    predicate="intersects"
)


# Find SA1 of each earthquake
eq_joined = gpd.sjoin(
    earthquake_data[["id", "geometry"]],
    gdf_sa1[["sa1_code21", "geometry"]],
    how="inner",
    predicate="within"
)


# In[36]:


print(fire_joined.head())
print(f"Record: {len(fire_joined)}")

print(flood_joined.head())
print(f"Record: {len(flood_joined)}")

print(eq_joined.head())
print(f"Record: {len(eq_joined)}")


# In[37]:


# Clean the data

fire_mapping = fire_joined[["id", "sa1_code21"]].rename(columns={"id": "history_id"})

flood_mapping = flood_joined[["id", "sa1_code21"]].rename(columns={"id": "history_id"})

eq_mapping = eq_joined[["id", "sa1_code21"]].rename(columns={"id": "history_id"})


# In[39]:


# Upload

fire_mapping.to_sql("fire_sa1_mapping", engine, if_exists="append", index=False)

flood_mapping.to_sql("flood_sa1_mapping", engine, if_exists="append", index=False)

eq_mapping.to_sql("earthquake_sa1_mapping", engine, if_exists="append", index=False)


# In[ ]:
