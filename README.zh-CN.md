<div align="center">

# Salus Bridge — 后端

[English](README.md) · **简体中文**

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-Azure%20%2B%20PostGIS-4169E1?logo=postgresql&logoColor=white)
[![CI](https://img.shields.io/github/actions/workflow/status/lotusink/salus-bridge-backend/go_ci.yml?label=CI&branch=main)](https://github.com/lotusink/salus-bridge-backend/actions/workflows/go_ci.yml)
![PRs welcome](https://img.shields.io/badge/PRs-welcome-brightgreen)
![Maintained](https://img.shields.io/badge/Maintained-yes-green)

**Salus Bridge** 的后端 —— 一个应急救援规划 PWA（Monash FIT5120 2026 S1 毕业项目）。
一个 Go BFF 作为 React 前端的唯一入口并独占全部业务逻辑；一个 PostgreSQL/PostGIS
数据库存放地理、人口普查、设施、灾害与风险数据，由一套 Python 数据管线构建。

它面向灾害场景（山火 · 洪水 · 地震）中的救援志愿者，提供基于地图的态势总览、
道路网络路径规划、危险区实时预警，以及就近定位脆弱人群等能力。

</div>

---

> **关于本仓库。** 本仓库是一个 6 人硕士毕业项目的后端部分。其中 **Go BFF 服务**
> 由本人独立设计与实现（end-to-end），**`database/pipeline`** 数据管线由本人
> 与项目的数据负责人共同开发。前端与产品设计由团队其他成员完成。

---

## 架构

```text
React PWA  (:3000)
      │
      │  HTTP / WebSocket
      ▼
Go BFF  (:8080)  ──  main.go  +  internal/<engine>/
      │
      ├──►  Azure PostgreSQL / PostGIS   (pgx + sqlx · TLS required)
      │
      └──►  External APIs      (OpenAI · Anthropic · OpenRouteService · Nominatim)
```

> BFF 是前端的唯一入口，独占全部 CORS、认证/会话头与请求校验，随后直接
> 读写 PostgreSQL，或调用上方的外部 API。
>
> 数据库由 `database/pipeline/` 下的 Python 管线提前填充（它是构建期工具，
> 而非常驻运行的服务）。

---

## 仓库结构

```
emergency-rescue-planner/
├── database/
│   ├── README.md                     # 管线文档与执行顺序
│   └── pipeline/                     # Python 构建期数据管线
│       ├── schema.sql                # 汇总的 DDL（参考用）
│       ├── 0_*..3_* / Risk Score.py  # 建表 + 加载数据（按数字顺序执行）
│       └── making_BTree_index.py     # 索引
└── service/
    └── go/
        └── bff/                      # Go BFF —— 本服务
            ├── main.go               # 路由注册与启动
            ├── docs/                 # Swagger（swaggo/swag 生成）
            └── internal/<engine>/    # 每个功能一个包（info、route、conditions……）
```

---

## 技术栈

### Go BFF

| 组成 | 选型 |
|---|---|
| 语言 | Go 1.26 |
| HTTP / 路由 | `net/http` |
| WebSocket | `gorilla/websocket` |
| PostgreSQL 驱动 | `jackc/pgx/v5`、`jmoiron/sqlx` |
| 配置 | `joho/godotenv` |
| API 文档 | `swaggo/swag` |

### 数据管线

| 组成 | 选型 |
|---|---|
| 语言 | Python 3.12 |
| 数据库 | PostgreSQL + PostGIS |
| 访问 / 地理 I/O | SQLAlchemy · psycopg2 · geopandas · shapely |

安装与执行顺序见 [`database/README.md`](emergency-rescue-planner/database/README.md)。

---

## 本地开发

### Go BFF

在 BFF 目录下创建 `.env` 文件，填入下方**环境变量**一节列出的变量，然后：

```bash
cd emergency-rescue-planner/service/go/bff
go run main.go                # 在 $GO_SERVICE_PORT 上服务（默认 8080）
go vet ./...                  # 推送前静态检查
go test ./...                 # 单元测试
swag init                     # 重新生成 Swagger 文档到 ./docs
```

### 数据库

BFF 预期连接一个已填充数据的 PostgreSQL/PostGIS 数据库。构建方式见
[`database/README.md`](emergency-rescue-planner/database/README.md)
（`schema.sql` 为 DDL；按数字编号的脚本负责加载数据）。

### 环境变量

BFF 必需：

| 变量 | 用途 |
|---|---|
| `GO_SERVICE_PORT` | 监听端口（默认 8080） |
| `ENV` | 取值 `deployment` 时禁用 Swagger UI，并将 CORS 切换为 `DEPLOYMENT_FRONTEND_URL`；其他取值使用 `LOCAL_FRONTEND_URL` |
| `DATABASE_URL` | PostgreSQL DSN —— **必须包含 `sslmode=require`** |
| `DEPLOYMENT_FRONTEND_URL` / `LOCAL_FRONTEND_URL` | CORS 白名单（精确 origin，不支持通配符） |

可选（按特性门控 —— 缺失时对应端点不可达）：

| 变量 | 缺失时禁用 |
|---|---|
| `OPENAI_API_KEY` | `/api/ai/*`、`/api/knowledge/{transcribe,tts,voice-search}` → 404 |
| `ANTHROPIC_API_KEY` | `/api/ai/*`、`/api/knowledge/{translate,voice-search}` → 404 |
| `ORS_API_KEY` | `/api/route/calculate` → 503 |

---

## API 一览

路由在 `service/go/bff/main.go` 中注册，按功能分组：

| 分组 | 前缀 | 主要端点 |
|---|---|---|
| Health | `/health_check` | `GET /health_check` |
| Overview | `/api/overview/*` | `geogroup`（10 分钟缓存）、`facilities` |
| Routing | `/api/route/*` | ORS 道路网络路径计算 |
| Field reports | `/api/field-reports/*` | 提交 · 确认 · 列表（需 `X-Volunteer-Session`） |
| Conditions | `/api/conditions/risk-zones`、`/ws/hazards` | 风险区 + WS 危险推送通道（子协议 `volunteerlink.hazards.v1`） |
| Vulnerable persons | `/api/vulnerable-persons` | Haversine 距离过滤 |
| Active routes | `/api/routes/active/*` | 注册 · 删除 · 接受改道（会话作用域） |
| Knowledge | `/api/knowledge/*` | 搜索 · 文章 · STT · TTS · 翻译 · 语音搜索 |
| Checklist | `/api/checklist` | 灾害 × 残障 查询 |
| Geocoding | `/api/geocode/*` | Nominatim 代理（正向 + 逆向） |
| AI | `/api/ai/*` | chat · stream（SSE） |
| Demo | `/api/demo/*` | heartbeat · 危险演示 启动/停止 |
| WebSocket | `/ws`、`/ws/hazards` | echo · 按会话的危险推送 |

**完整 schema：** `GET /swagger/`（仅当 `ENV != "deployment"` 时可用）。
