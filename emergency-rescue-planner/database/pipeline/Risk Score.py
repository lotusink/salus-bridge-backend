# =========================
# Part 1: Import Package
# =========================
import os
from dotenv import load_dotenv

load_dotenv()

import pandas as pd
import geopandas as gpd
from sqlalchemy import create_engine, text
import numpy as np

# Set engine
engine = create_engine(
    os.environ["DATABASE_URL"]
)


# In[ ]:


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
# Part 2: Calculate fire disaster risk
# =========================

gdf_fire = gpd.read_file("Source data/4-1 Historical_Bushfire_Boundaries_Version_2.0.geojson")

# 
print(gdf_fire.columns.tolist())
print(gdf_fire.dtypes)
print(gdf_fire.head(3))


# In[9]:


# Clean data
gdf_fire = gdf_fire[gdf_fire["geometry"].notna()]
gdf_fire = gdf_fire[gdf_fire["ignition_date"].notna()]
gdf_fire = gdf_fire[gdf_fire["area_ha"] > 0]

# Get year
gdf_fire["year"] = gdf_fire["ignition_date"].dt.year

# Filter the latest 10 years of data
gdf_fire = gdf_fire[gdf_fire["year"] > 2013]

gdf_fire = gdf_fire.to_crs("EPSG:4326")

print(f"Fire records: {len(gdf_fire)}")
print(f"Date range: {gdf_fire['year'].min()} - {gdf_fire['year'].max()}")


# In[23]:


# Find SA1 of each fire
fire_joined = gpd.sjoin(
    gdf_fire[["fire_id", "fire_type", "year", "geometry"]],
    gdf_sa1[["sa1_code21", "geometry"]],
    how="inner",
    predicate="intersects"
)


# In[14]:


print(fire_joined.head(3))
print(f"Records: {len(fire_joined)}")


# In[25]:


# Calculate fire times of sa1

# ARI = N / n        （N = total year，n = number of events）
# AEP = 1 - exp(-1 / ARI)

fire_N = fire_joined["year"].max() - fire_joined["year"].min() + 1

fire_stats = (
    fire_joined
    .groupby(["sa1_code21"])
    .agg(
        fire_n =("geometry", "count")
    )
    .reset_index()
)

fire_stats["fire_N"] = fire_N

# ARI = N / n
fire_stats["fire_ari"] = fire_stats["fire_N"] / fire_stats["fire_n"]

# AEP = 1 - exp(-1 / ARI)
fire_stats["fire_aep"] = 1 - np.exp(-1 / fire_stats["fire_ari"])


print(fire_stats.head(10))
print(f"Records: {len(fire_stats)}")


# In[ ]:


# =========================
# Part 3: Calculate flood disaster risk
# =========================


# In[31]:


gdf_flood = gpd.read_file("Source data/5-1 flood_study_summary_3ce61_-8922895210507100045.geojson")

# 
print(gdf_flood.columns.tolist())
print(gdf_flood.dtypes)
print(gdf_flood.head(3))


# In[36]:


gdf_flood = gdf_flood[gdf_flood["geometry"].notna()]
gdf_flood = gdf_flood[gdf_flood["year"] > 2008]
gdf_flood = gdf_flood.to_crs("EPSG:4326")

print(f"Flood Record: {len(gdf_flood)}")
print(f"Date Range: {gdf_flood['year'].min()} - {gdf_flood['year'].max()}")


# In[38]:


flood_joined = gpd.sjoin(
    gdf_flood[["id", "year", "geometry"]],
    gdf_sa1[["sa1_code21", "geometry"]],
    how="inner",
    predicate="within"
)

print(f"Record: {len(flood_joined)}")
print(flood_joined.head(3))


# In[47]:


flood_N = gdf_flood["year"].max() - gdf_flood["year"].min() + 1

flood_stats = (
    flood_joined
    .groupby("sa1_code21")
    .agg(flood_n=("id", "count"))
    .reset_index()
)

flood_stats["flood_N"] = flood_N
flood_stats["flood_ari"] = flood_stats["flood_N"] / flood_stats["flood_n"]
flood_stats["flood_aep"] = 1 - np.exp(-1 / flood_stats["flood_ari"])

print(flood_stats.head())
print(flood_stats.sort_values("flood_aep", ascending  = False).head())


# In[ ]:


# =========================
# Part 4: Calculate earthquake disaster risk
# =========================


# In[50]:


gdf_earthquake = gpd.read_file("Source data/6-1 australia_earthquakes_last11years.csv")

# 
print(gdf_earthquake.columns.tolist())
print(gdf_earthquake.dtypes)
print(gdf_earthquake.head(3))


# In[56]:


gdf_eq = gdf_earthquake.copy()

gdf_eq["time"] = pd.to_numeric(gdf_eq["time"], errors="coerce")
gdf_eq["time"] = pd.to_datetime(gdf_eq["time"], unit="ms", errors="coerce")
gdf_eq["mag"] = pd.to_numeric(gdf_eq["mag"], errors="coerce")
gdf_eq["lon"] = pd.to_numeric(gdf_eq["lon"], errors="coerce")
gdf_eq["lat"] = pd.to_numeric(gdf_eq["lat"], errors="coerce")
gdf_eq["depth"] = pd.to_numeric(gdf_eq["depth"], errors="coerce")
gdf_eq["tsunami"] = pd.to_numeric(gdf_eq["tsunami"], errors="coerce")
gdf_eq["year"] = gdf_eq["time"].dt.year
gdf_eq = gdf_eq[gdf_eq["year"] > 2016]

# Remove na
gdf_eq = gdf_eq.dropna(subset=["lon", "lat", "time"])

print(f"Earthquake Record: {len(gdf_eq)}")
print(f"Data Range: {gdf_eq['year'].min()} - {gdf_eq['year'].max()}")


# In[59]:


# =========================

gdf_eq = gpd.GeoDataFrame(
    gdf_eq,
    geometry=gpd.points_from_xy(gdf_eq["lon"], gdf_eq["lat"]),
    crs="EPSG:4326"
)

eq_joined = gpd.sjoin(
    gdf_eq[["year", "geometry"]],
    gdf_sa1[["sa1_code21", "geometry"]],
    how="inner",
    predicate="within"
)
print(f"Record: {len(eq_joined)}")
print(eq_joined.head())


# In[62]:


eq_N = gdf_eq["year"].max() - gdf_eq["year"].min() + 1

eq_stats = (
    eq_joined
    .groupby("sa1_code21")
    .agg(eq_n=("geometry", "count"))
    .reset_index()
)

eq_stats["eq_N"] = eq_N
eq_stats["eq_ari"] = eq_stats["eq_N"] / eq_stats["eq_n"]
eq_stats["eq_aep"] = 1 - np.exp(-1 / eq_stats["eq_ari"])

print(eq_stats.head())
print(eq_stats.sort_values("eq_aep", ascending  = False).head())


# In[ ]:


# =========================
# Part 5: Calculate vulnerability index
# =========================


# In[64]:


# Read data
g01 = pd.read_sql("SELECT * FROM population_g01", engine)
g09 = pd.read_sql("SELECT * FROM population_language_g09", engine)
g18 = pd.read_sql("SELECT * FROM disability_g18", engine)
g34 = pd.read_sql("SELECT * FROM family_vehicles_g34", engine)


# In[65]:


sa1_state = pd.read_sql("SELECT sa1_code21, state_name FROM sa1_geo", engine)


# In[75]:


print(g01.head(3))
print(g01["age_group"].unique())


# In[89]:


print(g09.head(3))
print(g09["country"].unique())


# In[78]:


print(g18.head(3))
print(g18["age_group"].unique())


# In[101]:


print(g34.head(3))
print(g34["num_vehicles"].unique())


# In[81]:


# Vulnerability factor

# Total population
total_pop = (
    g01.groupby("sa1_code21")["population_count"]
    .sum()
    .reset_index(name="total_pop")
)

# 1. Aged 65 or older
age_65_plus = g01[
    g01["age_group"].isin(["65_74", "75_84", "85ov"])
].groupby("sa1_code21")["population_count"].sum().reset_index(name="age_65_plus")


# 2. Aged 14 or younger
age_14_minus = g01[
    g01["age_group"].isin(["0_4", "5_14"])
].groupby("sa1_code21")["population_count"].sum().reset_index(name="age_14_minus")

# Combine
g01_features = total_pop.merge(age_65_plus, on="sa1_code21", how="left") \
                       .merge(age_14_minus, on="sa1_code21", how="left")


# In[113]:


g01_features[g01_features["total_pop"] == 0][["sa1_code21", "total_pop"]]


# In[116]:


# 3. Speak English "less than well"
g09_total = (
    g09[g09["country"] == "Tot"]
    .groupby("sa1_code21")["population_count"]
    .sum()
    .reset_index(name="total_pop")
)


g09_english = (
    g09[g09["primary_language"].str.contains("English", case=False, na=False)]
    .groupby("sa1_code21")["population_count"]
    .sum()
    .reset_index(name="english_pop")
)

non_english = g09_total.merge(g09_english, on="sa1_code21", how="left")


non_english["non_english_ratio"] = 1 - (non_english["english_pop"] / non_english["total_pop"])


non_english["non_english_ratio"] = non_english["non_english_ratio"].fillna(1.0)

non_english = non_english.drop(columns=["total_pop", "english_pop"])


# In[117]:


# 4. Disability
g18_disability = (
    g18[g18["age_group"] == "Tot"]
    .groupby("sa1_code21")["population_count"]
    .sum()
    .reset_index(name="disability_pop")
)


# In[118]:


# 4. No vehicle

no_vehicle = g34[g34["num_vehicles"] == "0"].groupby("sa1_code21")["count"].sum()

total_household = g34.groupby("sa1_code21")["count"].sum()

no_vehicle_ratio = (
    (no_vehicle / total_household)
    .reset_index(name="no_vehicle_ratio")
)


# In[127]:


# Merge data 
merge_df = g01_features \
    .merge(non_english, on="sa1_code21", how="left") \
    .merge(g18_disability, on="sa1_code21", how="left") \
    .merge(no_vehicle_ratio, on="sa1_code21", how="left") \
    .merge(sa1_state, on="sa1_code21", how="left")

merge_df["pct_65_plus"] = merge_df["age_65_plus"] / merge_df["total_pop"]
merge_df["pct_14_minus"] = merge_df["age_14_minus"] / merge_df["total_pop"]
merge_df["pct_disability"] = merge_df["disability_pop"] / merge_df["total_pop"]

# Fill NA value
num_cols = merge_df.select_dtypes(include=["number"]).columns

merge_df.loc[merge_df["total_pop"] == 0, num_cols] = 0


# In[130]:


print(merge_df["state_name"].unique())
print(merge_df.dtypes)
print(merge_df.head(3))


# In[132]:


cols = [
    "pct_disability",
    "pct_65_plus",
    "pct_14_minus",
    "non_english_ratio",
    "no_vehicle_ratio"
]

# Calculate for each state
for col in cols:
    merge_df[col + "_rank"] = merge_df.groupby("state_name")[col].rank(pct=True)

# Add score together
rank_cols = [col + "_rank" for col in cols]

merge_df["svi_raw"] = merge_df[rank_cols].sum(axis=1)

# Normalization
merge_df["SVI"] = merge_df.groupby("state_name")["svi_raw"].transform(
    lambda x: (x - x.min()) / (x.max() - x.min())
)

print(merge_df[["sa1_code21", "state_name", "SVI"]].head())


# In[133]:


# =========================
# Part 5: Calculate resilience
# =========================


# In[142]:


# Get emergency facilities data from the database

official_data = pd.read_sql(
    "SELECT facility_id, category, sa1_code21 FROM emergency_facilities",
    engine
)

osm_data = pd.read_sql(
    """
    SELECT id, category, sa1_code21
    FROM osm_support_facilities
    WHERE category IN ('fire_hydrant', 'hospital')
    """,
    engine
)


# In[156]:


print(official_data.head(3))
print(official_data["category"].unique())

print(osm_data.head(3))
print(osm_data["category"].unique())


# In[181]:


# Combine emergency facilities data

all_facilities = pd.concat([
    official_data[["category", "sa1_code21"]],
    osm_data[["category", "sa1_code21"]]
])

facility_count = (
    all_facilities
    .groupby(["sa1_code21", "category"])
    .size()
    .unstack(fill_value=0)
    .reset_index()
)


# In[182]:


print(facility_count)


# In[183]:


# Combine with population data

df = merge_df.merge(facility_count, on="sa1_code21", how="left").fillna(0)

# Density of emergency facilities

facility_cols = [col for col in facility_count.columns if col != "sa1_code21"]

for col in facility_cols:
    df[col + "_density"] = df[col] / df["total_pop"] * 1000

density_cols = [col + "_density" for col in facility_cols]


# In[184]:


print(density_cols)


# In[185]:


print(df.columns)


# In[187]:


# Normalization by state

def min_max_norm(group):
    for col in density_cols:
        min_v = group[col].min()
        max_v = group[col].max()

        if max_v - min_v == 0:
            group[col + "_norm"] = 0
        else:
            group[col + "_norm"] = (group[col] - min_v) / (max_v - min_v)

    return group

df = df.groupby("state_name", group_keys=False).apply(min_max_norm)

norm_cols = [col + "_norm" for col in density_cols]

# resilience index

df["resilience_index"] = df[norm_cols].mean(axis=1)

# Lack of resilience

df["lack_resilience"] = 1 - df["resilience_index"]


# In[190]:


print(df.columns)


# In[192]:


# =========================
# Final part: Risk table
# =========================

risk_stats_df = df \
    .merge(fire_stats, on="sa1_code21", how="left") \
    .merge(flood_stats, on="sa1_code21", how="left") \
    .merge(eq_stats, on="sa1_code21", how="left")

print(risk_stats_df.columns)


# In[201]:


risk_score = (risk_stats_df[
    ['sa1_code21','total_pop','fire_aep','flood_aep','eq_aep','SVI','lack_resilience']
    ].copy()
    .fillna(0)
             )


# In[199]:


print(risk_score.head(20))


# In[206]:


# Risk score of fire
risk_score["bushfire_risk"] = (
    risk_score["fire_aep"] *
    risk_score["SVI"] *
    risk_score["lack_resilience"]
) ** (1/3)

# Risk score of flood
risk_score["flood_risk"] = (
    risk_score["flood_aep"] *
    risk_score["SVI"] *
    risk_score["lack_resilience"]
) ** (1/3)

# Risk score of earthquake
risk_score["earthquake_risk"] = (
    risk_score["eq_aep"] *
    risk_score["SVI"] *
    risk_score["lack_resilience"]
) ** (1/3)

# Normalization
for col in ["bushfire_risk", "flood_risk", "earthquake_risk"]:
    min_v = risk_score[col].min()
    max_v = risk_score[col].max()
    if max_v - min_v == 0:
        risk_score[col + "_norm"] = 0
    else:
        risk_score[col + "_norm"] = (risk_score[col] - min_v) / (max_v - min_v)

mask = risk_score["total_pop"] == 0
num_cols = risk_score.select_dtypes(include="number").columns

risk_score.loc[mask, num_cols] = 0


# Risk level

for col in ["bushfire_risk_norm", "flood_risk_norm", "earthquake_risk_norm"]:
    risk_score[col.replace("_norm", "_level")] = pd.cut(
        risk_score[col],
        bins=[0, 0.20, 0.35, 0.50, 0.65, 1.0],
        labels=["very low","low", "medium", "high", "very high"],
        include_lowest=True
    )

# overall_risk
risk_score["overall_risk_norm"] = risk_score[[
    "bushfire_risk_norm", "flood_risk_norm", "earthquake_risk_norm"
]].max(axis=1)

risk_score["overall_risk_level"] = pd.cut(
    risk_score["overall_risk_norm"],
    bins=[0, 0.20, 0.35, 0.50, 0.65, 1.0],
    labels=["very low","low", "medium", "high", "very high"],
    include_lowest=True
)


# In[207]:


print(risk_score)


# In[217]:


print(risk_score.head())
print(risk_score.dtypes)


# In[214]:


with engine.begin() as conn:
    conn.execute(text("""
    DROP TABLE IF EXISTS sa1_risk;

    CREATE TABLE sa1_risk (
        id SERIAL PRIMARY KEY,

        sa1_code21 TEXT,

        fire_aep FLOAT,
        flood_aep FLOAT,
        eq_aep FLOAT,

        svi FLOAT,
        lack_resilience FLOAT,

        bushfire_risk FLOAT,
        flood_risk FLOAT,
        earthquake_risk FLOAT,

        bushfire_risk_norm FLOAT,
        flood_risk_norm FLOAT,
        earthquake_risk_norm FLOAT,

        bushfire_risk_level TEXT,
        flood_risk_level TEXT,
        earthquake_risk_level TEXT,

        overall_risk_norm FLOAT,
        overall_risk_level TEXT,

        CONSTRAINT fk_sa1
        FOREIGN KEY (sa1_code21)
        REFERENCES sa1_geo(sa1_code21)
        ON DELETE CASCADE
    );
    """))


# In[219]:


# upload data

df_upload = risk_score.copy()

# drop total pop
df_upload = df_upload.drop(columns=["total_pop"])

df_upload = df_upload.rename(columns={"SVI": "svi"})

# category → string
cat_cols = [
    "bushfire_risk_level",
    "flood_risk_level",
    "earthquake_risk_level",
    "overall_risk_level"
]

for col in cat_cols:
    df_upload[col] = df_upload[col].astype(str)

# NaN → None
df_upload = df_upload.replace({np.nan: None})

# upload
df_upload.to_sql(
    "sa1_risk",
    engine,
    if_exists="append",
    index=False
)





