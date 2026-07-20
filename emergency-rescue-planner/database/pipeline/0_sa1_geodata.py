# pip install openpyxl
# pip install geoalchemy2 psycopg2


import os
from dotenv import load_dotenv

load_dotenv()

import pandas as pd
import geopandas as gpd
from sqlalchemy import create_engine

'''
----------------------------------------------------------------------------------
Part 1: SA1 code data and related area
'''

sa1_code = pd.read_excel(
    "Source data/0 - SA1_2021_AUST.xlsx",
    dtype={"SA1_CODE_2021": str}
)

sa1_code = sa1_code[
    [
        "SA1_CODE_2021",
        "SA2_CODE_2021",
        "SA2_NAME_2021",
        "SA3_CODE_2021",
        "SA3_NAME_2021",
        "SA4_CODE_2021",
        "SA4_NAME_2021",
        "GCCSA_CODE_2021",
        "GCCSA_NAME_2021",
        "STATE_CODE_2021",
        "STATE_NAME_2021",
        "AUS_CODE_2021",
        "AUS_NAME_2021",
        "AREA_ALBERS_SQKM",
    ]
]

# rename to lowercase snake_case
sa1_code = sa1_code.rename(
    columns={
        "SA1_CODE_2021": "sa1_code21",
        "SA2_CODE_2021": "sa2_code21",
        "SA2_NAME_2021": "sa2_name",
        "SA3_CODE_2021": "sa3_code21",
        "SA3_NAME_2021": "sa3_name",
        "SA4_CODE_2021": "sa4_code21",
        "SA4_NAME_2021": "sa4_name",
        "GCCSA_CODE_2021": "gccsa_code21",
        "GCCSA_NAME_2021": "gccsa_name",
        "STATE_CODE_2021": "state_code21",
        "STATE_NAME_2021": "state_name",
        "AUS_CODE_2021": "aus_code21",
        "AUS_NAME_2021": "aus_name",
        "AREA_ALBERS_SQKM": "area_sqkm",
    }
)

sa1_code["area_sqkm"] = sa1_code["area_sqkm"].astype(float)

'''
----------------------------------------------------------------------------------
Part 2: Geometry of SA1
'''

gdf_sa1 = gpd.read_file(
    "Source data/0 - SA1_2021_AUST_SHP_GDA2020/SA1_2021_AUST_GDA2020.shp"
).to_crs("EPSG:4326")

gdf_sa1 = gdf_sa1[["SA1_CODE21", "geometry"]]
gdf_sa1 = gdf_sa1.rename(columns={"SA1_CODE21": "sa1_code21"})

'''
----------------------------------------------------------------------------------
Part 3: Merge attributes + geometry
'''

gdf_sa1_full = gdf_sa1.merge(sa1_code, on="sa1_code21", how="left")

'''
----------------------------------------------------------------------------------
Part 4: Compute center lat/lon
'''

gdf_sa1_full["center_lat"] = gdf_sa1_full.geometry.centroid.y
gdf_sa1_full["center_lon"] = gdf_sa1_full.geometry.centroid.x

# remove NA rows
gdf_sa1_full = gdf_sa1_full.dropna()

print(gdf_sa1_full.head())
print(gdf_sa1_full.isna().sum())

## Up load to data base

engine = create_engine(
    os.environ["DATABASE_URL"]
)
gdf_sa1_full.to_postgis(
    name="sa1_geo",
    con=engine,
    if_exists="append",
    index=False
)