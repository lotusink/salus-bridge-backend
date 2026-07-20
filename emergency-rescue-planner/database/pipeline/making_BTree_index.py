import os
from dotenv import load_dotenv

load_dotenv()

from sqlalchemy import create_engine, text

engine = create_engine(
    os.environ["DATABASE_URL"]
)

# 建索引（begin 自动 commit）
with engine.begin() as conn:
    conn.execute(text("CREATE INDEX IF NOT EXISTS idx_sa1_geo_state_code21 ON sa1_geo (state_code21);"))
    conn.execute(text("CREATE INDEX IF NOT EXISTS idx_sa1_geo_sa4_code21   ON sa1_geo (sa4_code21);"))
    conn.execute(text("CREATE INDEX IF NOT EXISTS idx_sa1_geo_sa3_code21   ON sa1_geo (sa3_code21);"))
    conn.execute(text("CREATE INDEX IF NOT EXISTS idx_sa1_geo_sa2_code21   ON sa1_geo (sa2_code21);"))
    print("索引创建完成")

# 验证
with engine.connect() as conn:
    result = conn.execute(text("""
        SELECT indexname, indexdef FROM pg_indexes
        WHERE tablename = 'sa1_geo' ORDER BY indexname;
    """))
    for row in result:
        print(row)