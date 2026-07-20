
# =========================
# Part 1: official released emergency facility
# =========================
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


official_emergency = pd.read_csv("Source data/2 - Emergency Management Facilities.csv")

official_emergency = official_emergency[
    ['CLASS',
     'FACILITY_NAME',
     'FACILITY_STATE',
     'GNAF_FORMATTED_ADDRESS',
     'GNAF_POSTCODE',
     'GNAF_SUBURB',
     'GNAF_LAT',
     'GNAF_LONG']
]

# Rename
official_emergency = official_emergency.rename(columns={
    "CLASS": "category",
    "FACILITY_NAME": "facility_name",
    "FACILITY_STATE": "state",
    "GNAF_FORMATTED_ADDRESS": "address",
    "GNAF_POSTCODE": "postcode",
    "GNAF_SUBURB": "suburb",
    "GNAF_LAT": "lat",
    "GNAF_LONG": "lon"
})


# Add SA1 code

# 1 get SA1 shp
gdf_sa1 = gpd.read_file("Source data/0 - SA1_2021_AUST_SHP_GDA2020/SA1_2021_AUST_GDA2020.shp").to_crs("EPSG:4326")


# 2 official emergency → GeoDataFrame
gdf_oe = gpd.GeoDataFrame(
    official_emergency,
    geometry=gpd.points_from_xy(official_emergency.lon, official_emergency.lat),
    crs="EPSG:4326"
)

# 3 Spacial join
gdf_oe_joined = gpd.sjoin(
    gdf_oe,
    gdf_sa1[["SA1_CODE21", "geometry"]],
    how="left",
    predicate="within"
)

# 4. Add SA1_CODE_2021
gdf_oe_joined["SA1_CODE_2021"] = gdf_oe_joined["SA1_CODE21"].astype(str)

# 5 Remove na
gdf_oe_joined = gdf_oe_joined[gdf_oe_joined["SA1_CODE_2021"].notna()]

# 6 Rename and change data type
gdf_oe_joined = gdf_oe_joined.rename(columns={"SA1_CODE21": "sa1_code21"})

gdf_oe_joined["postcode"] = (
    gdf_oe_joined["postcode"]
    .astype("Int64")
    .astype(str)
    .str.replace(".0", "", regex=False)
    .str.zfill(4)
)

# Change category
gdf_oe_joined["category"] = gdf_oe_joined["category"].replace({
    "POLICING FACILITY": "POLICING",
    "SES FACILITY": "SES",
    "RURAL/COUNTRY FIRE SERVICE FACILITY": "FIRE SERVICE",
    "AMBULANCE STATION": "AMBULANCE",
    "METRO FIRE FACILITY": "FIRE SERVICE",
    "OTHER EMERGENCY MANAGEMENT FACILITY": "OTHER"
})


# 7 Remove
official_emergency = gdf_oe_joined.drop(columns=["geometry", "index_right","SA1_CODE_2021"])

print(official_emergency.dtypes)
print(official_emergency.head())


# Upload
official_emergency.to_sql(
    name="emergency_facilities",
    con=engine,
    if_exists="append",
    index=False
)

# # =========================
# # Part 2: OSM emergency facility
# # =========================
#
#
# # pip install requests
# import requests
#
# url = "https://overpass-api.de/api/interpreter"
#
# query = """
# [out:json][timeout:180];
# area["name"="Australia"]->.a;
# (
#   // hospital
#   nwr["amenity"="hospital"](area.a);
#
#   // helipad
#   nwr["aeroway"="helipad"](area.a);
#
#   // fire hydrant
#   node["emergency"="fire_hydrant"](area.a);
#
#   // parks
#   nwr["leisure"="park"](area.a);
#
#   // schools
#   nwr["amenity"="school"](area.a);
#
#   // sports / open space
#   nwr["leisure"="pitch"](area.a);
#   nwr["leisure"="stadium"](area.a);
# );
# out center 10;
# """
#
#
# response = requests.get(url, params={"data": query})
# data = response.json()
#
#
# # Clean the data
# import pandas as pd
#
# elements = data["elements"]
#
# rows = []
#
# for el in elements:
#     tags = el.get("tags", {})
#
#     # get category
#     if tags.get("amenity") == "hospital":
#         category = "hospital"
#     elif tags.get("aeroway") == "helipad":
#         category = "helipad"
#     elif tags.get("emergency") == "fire_hydrant":
#         category = "fire_hydrant"
#     elif tags.get("leisure") in ["park", "stadium", "sports_centre", "pitch"]:
#         category = "open_space"
#     elif tags.get("amenity") == "school":
#         category = "school"
#     else:
#         category = "other"
#
#     # if it's node or way
#     lat = el.get("lat") or el.get("center", {}).get("lat")
#     lon = el.get("lon") or el.get("center", {}).get("lon")
#
#
#     row = {
#         "osm_id": el.get("id"),
#         "type": el.get("type"),
#         "category": category,
#         "facility_name": tags.get("name"),
#         "phone": tags.get("phone"),
#         "fax": tags.get("fax"),
#         "lat": lat,
#         "lon": lon,
#         "source": "osm"
#     }
#
#     rows.append(row)
#
# osm_emergency_df = pd.DataFrame(rows)
#
# # Add SA1 code
# import geopandas as gpd
#
# # 1 df → GeoDataFrame
# gdf_osm = gpd.GeoDataFrame(
#     osm_emergency_df,
#     geometry=gpd.points_from_xy(osm_emergency_df.lon, osm_emergency_df.lat),
#     crs="EPSG:4326"
# )
#
# # 2. Spacial join
# gdf_joined = gpd.sjoin(
#     gdf_osm,
#     gdf_sa1[["SA1_CODE21", "geometry"]],
#     how="left",
#     predicate="within"
# )
#
# # 3. Add SA1_CODE_2021
# gdf_joined["SA1_CODE_2021"] = gdf_joined["SA1_CODE21"].astype(str)
#
# # 4 Remove nan
# gdf_joined = gdf_joined[gdf_joined["SA1_CODE_2021"].notna()]
#
# # 5 Remove
# osm_emergency_df = gdf_joined.drop(columns=["geometry", "index_right", "SA1_CODE21"])
#
# # osm_emergency_df.to_csv("3 - facilities_clean.csv", index=False)
# print(osm_emergency_df.info())



# =========================
# Part 3: Upload OSM emergency facility
# =========================


osm_emergency = pd.read_csv("Source data/3 - osm_emergency.csv")

# Add SA1 code
import geopandas as gpd

# 1 df → GeoDataFrame
gdf_osm = gpd.GeoDataFrame(
    osm_emergency,
    geometry=gpd.points_from_xy(osm_emergency.lon, osm_emergency.lat),
    crs="EPSG:4326"
)

# 2. Spacial join
gdf_osm_joined = gpd.sjoin(
    gdf_osm,
    gdf_sa1[["SA1_CODE21", "geometry"]],
    how="left",
    predicate="within"
)

# 3. Add SA1_CODE_2021
gdf_osm_joined["SA1_CODE_2021"] = gdf_osm_joined["SA1_CODE21"].astype(str)

# 4 Remove nan
gdf_osm_joined = gdf_osm_joined[gdf_osm_joined["SA1_CODE_2021"].notna()]

# 6 Rename and change data type
gdf_osm_joined = gdf_osm_joined.rename(columns={"SA1_CODE21": "sa1_code21"})

# 7 Remove
osm_emergency_df = gdf_osm_joined.drop(columns=["geometry", "index_right", "SA1_CODE_2021"])

# Add web column
osm_emergency_df["web"] = None

# osm_emergency_df.to_csv("3 - facilities_clean.csv", index=False)
print(osm_emergency_df.info())

# Upload
osm_emergency_df.to_sql(
    name="osm_support_facilities",
    con=engine,
    if_exists="append",
    index=False
)