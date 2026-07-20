import os
from dotenv import load_dotenv

load_dotenv()

from sqlalchemy import create_engine, text

engine = create_engine(
    os.environ["DATABASE_URL"]
)

with engine.connect() as conn:
    result = conn.execute(text("SELECT version();"))
    for row in result:
        print(row)