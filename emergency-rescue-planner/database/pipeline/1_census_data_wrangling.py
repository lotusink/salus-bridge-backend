# pip install pandas
import os
from dotenv import load_dotenv

load_dotenv()

import requests
import geopandas as gpd
import pandas as pd
from sqlalchemy import create_engine, text
# =========================
# Create engine upload data
# =========================

engine = create_engine(
    os.environ["DATABASE_URL"]
)
sa1_df = pd.read_sql("SELECT sa1_code21 FROM sa1_geo", engine)
valid_sa1 = set(sa1_df["sa1_code21"])
# =========================
# Part 1:
# Change G01 population data format from wide to long format
# sa1_code, age_group, gender
# =========================

# read census
df_g01 = pd.read_csv("Source data/1 - Census dataAU/2021Census_G01_AUST_SA1.csv"
                     , dtype={"SA1_CODE_2021": str})

# melt the table
df_g01_long = df_g01.melt(
    id_vars=["SA1_CODE_2021"],
    var_name="variable",
    value_name="population_count"
)

# Extract（age_group + gender）
df_g01_long[["age_group", "gender"]] = df_g01_long["variable"].str.extract(
    r'Age_(\d+_\d+|85ov)(?:_yr)?_([MF])'
)


# Change gender in to male and female
df_g01_long["gender"] = df_g01_long["gender"].map({
    "M": "male",
    "F": "female"
})

# Rename sa1 code
df_g01_long = df_g01_long.rename(columns={"SA1_CODE_2021": "sa1_code21"})

# Drop variable
df_g01_long = df_g01_long.drop(columns="variable")

# Add data_date
df_g01_long["data_date"] = pd.to_datetime("2021-01-01")

# Clean NA
df_g01_long_final = df_g01_long.dropna()

# Drop invalid sa1
df_g01_long_final = df_g01_long_final[
    df_g01_long_final["sa1_code21"].isin(valid_sa1)
]

print("df_g01_long")
print(df_g01_long_final.head(10))

# print(df_g01_long_final[df_g01_long_final["sa1_code21"] == "10102100701"])


## Save File
# df_g01_long_final.to_csv("g01_data.csv", index=False)

df_g01_long_final.to_sql(
    name="population_g01",
    con=engine,
    if_exists="append",
    index=False
)

# =========================
# Part 2:
# Change G09 population of country data format from wide to long format
# sa1_code, country, population
# =========================

# Load data


folder_path = "Source data/1 - Census dataAU"

g09_data = {}

for letter in list("ABCDEFGH"):
    file_name = f"2021Census_G09{letter}_AUST_SA1.csv"
    file_path = os.path.join(folder_path, file_name)

    df = pd.read_csv(file_path, dtype={"SA1_CODE_2021": str})

    # Save to data
    g09_data[letter] = df

for key, df in g09_data.items():
    print(f"{key}: {df.shape}")

# 2. Extract total population of each country at each sa1

all_dfs = []

for key, df in g09_data.items():

    # Find all 'Tot' columns
    tot_cols = [col for col in df.columns if col.endswith("_Tot")]

    # Save SA1 + 'Tot' columns
    df_filtered = df[["SA1_CODE_2021"] + tot_cols].copy()

    all_dfs.append(df_filtered)

# Combine all table
df_combined = pd.concat(all_dfs, ignore_index=True)

print(df_combined.shape)

# Merge data by sa1 code
g09_country_tot= all_dfs[0]

for df in all_dfs[1:]:
    g09_country_tot = g09_country_tot.merge(
        df,
        on="SA1_CODE_2021",
        how="outer"
    )

print(g09_country_tot.shape)


# 3. Change to long formate

df_g09_long = g09_country_tot.melt(
    id_vars=["SA1_CODE_2021"],
    var_name="variable",
    value_name="population_count"
)

df_g09_long[["country"]] = df_g09_long["variable"].str.extract(
    r'P_(.*)_Tot'
)

# Rename sa1 code
df_g09_long = df_g09_long.rename(columns={"SA1_CODE_2021": "sa1_code21"})

# Drop variable
df_g09_long = df_g09_long.drop(columns="variable")

# Add data_date
df_g09_long["data_date"] = pd.to_datetime("2021-01-01")

# Clean NA
df_g09_long_final = df_g09_long.dropna()

# 4. Match language

# Find unique country

countries = df_g09_long_final["country"].unique()
# print(countries)

# Create language map
language_map = {
    "Afghanistan": "Dari",
    "Australia": "English",
    "Bangladesh": "Bengali",
    "Bosnia_Herzegov": "Bosnian",
    "Brazil": "Portuguese",
    "Cambodia": "Khmer",
    "Canada": "English",
    "Chile": "Spanish",
    "China": "Mandarin",
    "Croatia": "Croatian",
    "Egypt": "Arabic",
    "England": "English",
    "Fiji": "English",
    "France": "French",
    "Germany": "German",
    "Greece": "Greek",
    "Hong_Kong_SAR_Ch": "Cantonese",
    "India": "Hindi",
    "Indonesia": "Indonesian",
    "Iran": "Persian",
    "Iraq": "Arabic",
    "Ireland": "English",
    "Italy": "Italian",
    "Japan": "Japanese",
    "Korea_South": "Korean",
    "Lebanon": "Arabic",
    "Malaysia": "Malay",
    "Malta": "English",
    "Mauritius": "English",
    "Myanmar": "Burmese",
    "Nepal": "Nepali",
    "Netherlands": "Dutch",
    "New_Zealand": "English",
    "North_Macedonia": "Macedonian",
    "Pakistan": "Urdu",
    "PNG": "English",
    "Philippines": "Filipino",
    "Poland": "Polish",
    "Samoa": "Samoan",
    "Scotland": "English",
    "Singapore": "English",
    "South_Africa": "English",
    "Sri_Lanka": "Sinhala",
    "Taiwan": "Mandarin",
    "Thailand": "Thai",
    "Turkey": "Turkish",
    "USA": "English",
    "Vietnam": "Vietnamese",
    "Wales": "English",
    "Zimbabwe": "English",

    # Special categories
    "Elsewhere": "missing",
    "COB_NS": "missing",
    "Tot": "total"
}


df_g09_long_final["primary_language"] = df_g09_long_final["country"].map(language_map)

# Drop invalid sa1
df_g09_long_final = df_g09_long_final[
    df_g09_long_final["sa1_code21"].isin(valid_sa1)
]

print("df_g09_long_final")
print(df_g09_long_final[df_g09_long_final["sa1_code21"] == "10102100701"])

## Save File
# df_g09_long_final.to_csv("g09_data.csv", index=False)


df_g09_long_final.to_sql(
    name="population_language_g09",
    con=engine,
    if_exists="append",
    index=False
)
# =========================
# Part 3:
# Change G18 population of people with disability data format from wide to long format
# sa1_code, age_group, population_count, gender
# =========================


# read census
df_g18 = pd.read_csv("Source data/1 - Census dataAU/2021Census_G18_AUST_SA1.csv"
                     , dtype={"SA1_CODE_2021": str})

cols = [col for col in df_g18.columns if "Need_for_assistance" in col and not col.endswith("_ns")]
df_g18_filtered = df_g18[["SA1_CODE_2021"] + cols].copy()


# melt the table
df_g18_long = df_g18_filtered.melt(
    id_vars=["SA1_CODE_2021"],
    var_name="variable",
    value_name="population_count"
)

# Extract（age_group + gender）
df_g18_long[["gender","age_group"]] = df_g18_long["variable"].str.extract(
    r'([MF])_(.*?)_(?:yrs_)?Need_for_assistance'
)


# Change gender in to male and female
df_g18_long["gender"] = df_g18_long["gender"].map({
    "M": "male",
    "F": "female"
})


# Rename sa1 code
df_g18_long = df_g18_long.rename(columns={"SA1_CODE_2021": "sa1_code21"})

# Drop variable
df_g18_long = df_g18_long.drop(columns="variable")

# Add data_date
df_g18_long["data_date"] = pd.to_datetime("2021-01-01")


# Clean NA
df_g18_long_final = df_g18_long.dropna()

# Drop invalid sa1
df_g18_long_final = df_g18_long_final[
    df_g18_long_final["sa1_code21"].isin(valid_sa1)
]

print("df_g18_long_final")
print(df_g18_long_final[df_g18_long_final["sa1_code21"] == "10102100701"])

## Save File
# df_g18_long_final.to_csv("g18_data.csv", index=False)

df_g18_long_final.to_sql(
    name="disability_g18",
    con=engine,
    if_exists="append",
    index=False
)

# =========================
# Part 4:
# Change G25 population of people can assist data format from wide to long format
# sa1_code, age_group, population_count, gender
# =========================


# read census
df_g25 = pd.read_csv("Source data/1 - Census dataAU/2021Census_G25_AUST_SA1.csv"
                     , dtype={"SA1_CODE_2021": str})

# melt the table
df_g25_long = df_g25.melt(
    id_vars=["SA1_CODE_2021"],
    var_name="variable",
    value_name="population_count"
)

# Extract（age_group + gender）
df_g25_long[["gender"]] = df_g25_long["variable"].str.extract(
    r'([MF])_Tot_Tot'
)


# Change gender in to male and female
df_g25_long["gender"] = df_g25_long["gender"].map({
    "M": "male",
    "F": "female"
})


# Rename sa1 code
df_g25_long = df_g25_long.rename(columns={"SA1_CODE_2021": "sa1_code21"})

# Drop variable
df_g25_long = df_g25_long.drop(columns="variable")

# Add data_date
df_g25_long["data_date"] = pd.to_datetime("2021-01-01")

# Clean NA
df_g25_long_final = df_g25_long.dropna()

# Drop invalid sa1
df_g25_long_final = df_g25_long_final[
    df_g25_long_final["sa1_code21"].isin(valid_sa1)
]

print("df_g25_long_final")
print(df_g25_long_final[df_g25_long_final["sa1_code21"] == "10102100701"])

## Save File
# df_g25_long_final.to_csv("g25_data.csv", index=False)

df_g25_long_final.to_sql(
    name="assistance_g25",
    con=engine,
    if_exists="append",
    index=False
)

# =========================
# Part 5:
# Change G34 number of family with motor vehicles data format from wide to long format
# sa1_code, num_vehicles, count
# =========================


# read census
df_g34 = pd.read_csv("Source data/1 - Census dataAU/2021Census_G34_AUST_SA1.csv"
                     , dtype={"SA1_CODE_2021": str})

# melt the table
df_g34_long = df_g34.melt(
    id_vars=["SA1_CODE_2021"],
    var_name="variable",
    value_name="count"
)

# Extract（num_vehicles）
df_g34_long[["num_vehicles"]] = df_g34_long["variable"].str.extract(
    r'Num_MVs_per_dweling_([0-9]+|4mo)'
)


# Rename sa1 code
df_g34_long = df_g34_long.rename(columns={"SA1_CODE_2021": "sa1_code21"})

# Drop variable
df_g34_long = df_g34_long.drop(columns="variable")

# Add data_date
df_g34_long["data_date"] = pd.to_datetime("2021-01-01")

# Clean NA
df_g34_long_final = df_g34_long.dropna()

# Drop invalid sa1
df_g34_long_final = df_g34_long_final[
    df_g34_long_final["sa1_code21"].isin(valid_sa1)
]


print("df_g34_long_final")
print(df_g34_long_final[df_g34_long_final["sa1_code21"] == "10102100701"])

## Save File
# df_g34_long_final.to_csv("g34_data.csv", index=False)


df_g34_long_final.to_sql(
    name="family_vehicles_g34",
    con=engine,
    if_exists="append",
    index=False
)