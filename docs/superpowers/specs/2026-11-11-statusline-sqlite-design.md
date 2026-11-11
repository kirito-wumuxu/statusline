# statusline & statusline-server Architecture & Design Specification (SQLite3)

- **Date**: 2026-11-11
- **Target Projects**: `statusline` (Cross-Platform CLI Plugin) and `statusline-server` (Telemetry & Monitoring Service)
- **Primary Database**: SQLite3 (Embedded, WAL Mode, Zero-CGO via `modernc.org/sqlite`)
- **Language**: Go 1.22+ (Static compilation, Zero runtime dependencies)
- **Status**: Draft Spec for User Review

---

## 1. 目标与设计原则 (Objectives & Principles)

本系统针对 Google Antigravity (`agy`) 与 Anthropic Claude Code 命令行助手的 Statusline 遥测采集与状态栏呈现场景设计，兼顾单机极速渲染与多节点集中监控需求。

### 核心原则
1. **渲染绝对零阻塞**：客户端执行必须在 2ms 内向 `stdout` 输出格式化 ANSI 文本，网络上报完全处于异步 Goroutine 中且具备 100ms 严格超时与异常静默丢弃（Fail-Silent），绝不阻塞终端交互。
2. **纯 Go 零外部依赖 (Zero CGO)**：客户端与服务端均支持 `CGO_ENABLED=0` 静态编译为单个独立二进制，无需宿主机安装 C 编译器或运行库，无缝跨平台（Windows / Linux amd64/arm64 / macOS）。
3. **便携式单文件数据库 (SQLite3 + WAL)**：服务端内置 SQLite3，所有节点、会话、流水与配额集中存储在单个 `.db` 文件中。开启 WAL 模式与并发优化，无需部署 MySQL/PostgreSQL 或 Docker 容器。
4. **统一归一化与字符健壮性**：自动识别 `antigravity` 与 `claude_code` 数据格式；针对 Windows GBK/UTF-8 字符错位问题进行严格清洗（`strings.ToValidUTF8`），防止入库乱码。
5. **吸收并升华原型经验**：继承 `~/workspace/repository/temp` 的 ANSI 极简美学色彩系统、窄屏自适应缩放机制（Level 1~3）、Git 状态快速探针（20ms 熔断）与嵌入式深色实时 Web 仪表盘。

---

## 2. 系统整体架构与目录结构

```text
statusline/
├── config.yaml                     # 统一配置文件模板 (服务端/客户端共享)
├── go.mod                          # Go 模块定义 (依赖 modernc.org/sqlite)
├── AGENTS.md                       # 项目开发与架构指南
├── cmd/
│   ├── statusline/                 # 跨平台客户端 (单二进制 ~2MB)
│   │   ├── main.go                 # 客户端入口 (stdin -> render -> async report)
│   │   └── main_test.go
│   └── statusline-server/          # 集中服务程序 (单二进制 ~15MB)
│       ├── main.go                 # 服务端入口 (HTTP 监听、自动 Migrate)
│       ├── main_test.go
│       └── web/                    # 嵌入式深色 Web 仪表盘 (//go:embed)
│           ├── index.html
│           ├── app.js
│           └── style.css
├── internal/
│   ├── config/                     # 配置加载与合并 (YAML + 环境变量覆盖)
│   │   ├── config.go
│   │   └── config_test.go
│   ├── node/                       # 跨平台节点硬件指纹 (node_id 提取与脱敏)
│   │   ├── machine_id.go
│   │   ├── machine_id_linux.go
│   │   ├── machine_id_windows.go
│   │   └── machine_id_test.go
│   ├── model/                      # 遥测模型契约与数据归一化
│   │   ├── types_agy.go            # AGY 原始 JSON 契约
│   │   ├── types_claude.go         # Claude Code 原始 JSON 契约
│   │   ├── envelope.go             # 统一遥测信封与归一化逻辑
│   │   ├── utf8_cleaner.go         # 字符编码清洗与防乱码
│   │   └── envelope_test.go
│   ├── render/                     # 终端 ANSI 状态栏渲染核心
│   │   ├── colors.go               # 终端颜色与 ANSI 转义常量
│   │   ├── layout.go               # 终端自适应收缩与降级排版 (Level 1~3)
│   │   ├── segment_dir.go          # 目录分段 (~ 折叠)
│   │   ├── segment_model.go        # 模型与推理等级分段
│   │   ├── segment_context.go      # 8 段斜杠进度条与容量分段
│   │   ├── segment_tokens.go       # Token 计数分段 (↑0 ↓0)
│   │   ├── segment_quota.go        # 配额余量与倒计时分段
│   │   ├── segment_vcs.go          # Git 分支与脏状态分段
│   │   └── render_test.go
│   ├── vcs/                        # Git 超时探测工具
│   │   ├── git.go                  # 20ms 熔断探测 ahead/behind
│   │   └── git_test.go
│   ├── reporter/                   # 客户端非阻塞异步上报器
│   │   ├── client.go
│   │   └── client_test.go
│   ├── storage/                    # SQLite3 存储引擎与事务层
│   │   ├── schema.sql              # DDL 初始定义
│   │   ├── db.go                   # 连接初始化、WAL 模式配置与自动迁移
│   │   ├── store.go                # 4 表事务化写入与查询逻辑
│   │   └── store_test.go
│   └── server/                     # 服务端 HTTP API 控制器
│       ├── handlers_report.go      # POST /api/v1/report
│       ├── handlers_api.go         # GET /api/v1/sessions, overview, quotas
│       ├── refresher.go            # 终端空闲自动 SIGWINCH 调度器
│       └── handlers_test.go
└── docs/
    └── superpowers/
        └── specs/
            └── 2026-11-11-statusline-sqlite-design.md
```

---

## 3. SQLite3 数据库设计与事务规范

### 3.1 核心表结构 (DDL)

```sql
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = NORMAL;
PRAGMA foreign_keys = ON;

-- 1. 节点表 (nodes)
CREATE TABLE IF NOT EXISTS nodes (
    node_id         TEXT PRIMARY KEY,              -- 'node_8f9d2a1b4c3e5f6a'
    hostname        TEXT,                          -- 主机名
    os_type         TEXT NOT NULL,                 -- 'windows' | 'linux' | 'darwin'
    os_arch         TEXT,                          -- 'amd64' | 'arm64'
    ip_address      TEXT,                          -- 客户端出口/内网 IP
    client_version  TEXT,                          -- 采集器版本
    first_seen_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_seen_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_nodes_last_seen ON nodes(last_seen_at DESC);

-- 2. 会话维度表 (sessions)
CREATE TABLE IF NOT EXISTS sessions (
    session_id              TEXT PRIMARY KEY,              -- UUID 字符串
    node_id                 TEXT NOT NULL REFERENCES nodes(node_id),
    product_type            TEXT NOT NULL,                 -- 'antigravity' | 'claude_code'
    conversation_id         TEXT,                          -- AGY 专属
    session_name            TEXT,                          -- Claude Code 专属 (UTF-8 清洗后文本)
    
    account_email           TEXT,                          -- AGY: email
    plan_tier               TEXT,                          -- AGY: plan_tier
    tool_version            TEXT,                          -- 对应 version ("1.5.5" / "2.1.312")
    
    cwd                     TEXT NOT NULL,
    project_dir             TEXT,
    transcript_path         TEXT,
    git_repo_host           TEXT,                          -- workspace.repo.host 或解析自 remote
    git_repo_owner          TEXT,                          -- workspace.repo.owner
    git_repo_name           TEXT,                          -- workspace.repo.name
    git_branch              TEXT,                          -- worktree.branch 或 vcs.branch
    git_worktree            TEXT,                          -- worktree.name
    
    latest_model_id         TEXT,
    total_input_tokens      INTEGER DEFAULT 0,
    total_output_tokens     INTEGER DEFAULT 0,
    context_used_pct        REAL DEFAULT 0.0,
    total_cost_usd          REAL DEFAULT 0.0,
    total_duration_ms       INTEGER DEFAULT 0,
    total_lines_added       INTEGER DEFAULT 0,
    total_lines_removed     INTEGER DEFAULT 0,
    
    started_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_active_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_sessions_node_active ON sessions(node_id, last_active_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_product ON sessions(product_type, last_active_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_email ON sessions(account_email) WHERE account_email IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sessions_repo ON sessions(git_repo_owner, git_repo_name) WHERE git_repo_name IS NOT NULL;

-- 3. 对话状态上报明细表 (telemetry_events)
CREATE TABLE IF NOT EXISTS telemetry_events (
    id                          INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id                     TEXT NOT NULL,
    session_id                  TEXT NOT NULL,
    prompt_id                   TEXT,                          -- Claude Code 专属
    product_type                TEXT NOT NULL,                 -- 'antigravity' | 'claude_code'
    reported_at                 TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    
    model_id                    TEXT NOT NULL,
    model_display_name          TEXT,
    model_effort                TEXT,                          -- 归一化: 'high' 等
    agent_state                 TEXT,                          -- AGY: 'idle' 等
    
    context_window_size         INTEGER,
    total_input_tokens          INTEGER,
    total_output_tokens         INTEGER,
    current_input_tokens        INTEGER,
    current_output_tokens       INTEGER,
    cache_creation_tokens       INTEGER DEFAULT 0,
    cache_read_tokens           INTEGER DEFAULT 0,
    used_percentage             REAL,
    remaining_percentage        REAL,
    exceeds_200k_tokens         INTEGER DEFAULT 0,             -- 0: false, 1: true
    
    total_cost_usd              REAL,
    total_duration_ms           INTEGER,
    total_api_duration_ms       INTEGER,
    cache_hit_ratio             REAL,
    
    quota_snapshot              TEXT,                          -- JSON 字符串
    prompt_cache_detail         TEXT,                          -- JSON 字符串
    raw_payload                 TEXT NOT NULL                  -- 原始 JSON 字符串
);
CREATE INDEX IF NOT EXISTS idx_events_session_time ON telemetry_events(session_id, reported_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_node_time ON telemetry_events(node_id, reported_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_reported_at ON telemetry_events(reported_at DESC);

-- 4. 账号配额实时状态表 (account_quotas)
CREATE TABLE IF NOT EXISTS account_quotas (
    account_email       TEXT NOT NULL,
    quota_key           TEXT NOT NULL,                 -- '3p-5h', 'gemini-weekly', 'gemini-5h'
    remaining_fraction  REAL NOT NULL,                 -- 0.8738665
    reset_time          TEXT NOT NULL,                 -- ISO8601
    reset_in_seconds    INTEGER NOT NULL,
    last_node_id        TEXT,
    updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (account_email, quota_key)
);
```

### 3.2 归一化与单事务写入逻辑

每次收到客户端上报的请求后，服务端开启单个 SQLite 事务：
1. **识别产品类型 (`product_type`)**：
   - 若 `payload.product == "antigravity"`，确定为 `antigravity`。
   - 若不存在 `product` 且存在 `cost` / `prompt_id` / 或 `transcript_path` 包含 `.claude`，确定为 `claude_code`。
2. **字段对齐 (Normalization)**：
   - 推理等级 `model_effort`：优先提取 `payload.model.effort`，若无则提取 `payload.effort.level`。
   - 字符清洗：对 `session_name` 与 `cwd` 等字符串应用 `strings.ToValidUTF8(s, "")`。
   - 时间戳统一：客户端带 `reported_at` 则转为 ISO8601 UTC；若无则服务端取 `time.Now().UTC().Format(time.RFC3339Nano)`。
3. **事务内依次执行**：
   - **Step 1: 节点更新**
     ```sql
     INSERT INTO nodes (node_id, hostname, os_type, os_arch, ip_address, client_version, last_seen_at)
     VALUES (?, ?, ?, ?, ?, ?, ?)
     ON CONFLICT(node_id) DO UPDATE SET
       hostname = excluded.hostname,
       ip_address = excluded.ip_address,
       client_version = excluded.client_version,
       last_seen_at = excluded.last_seen_at;
     ```
   - **Step 2: 会话快照更新**
     ```sql
     INSERT INTO sessions (
       session_id, node_id, product_type, conversation_id, session_name,
       account_email, plan_tier, tool_version, cwd, project_dir, transcript_path,
       git_repo_host, git_repo_owner, git_repo_name, git_branch, git_worktree,
       latest_model_id, total_input_tokens, total_output_tokens, context_used_pct,
       total_cost_usd, total_duration_ms, total_lines_added, total_lines_removed,
       last_active_at
     ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
     ON CONFLICT(session_id) DO UPDATE SET
       node_id = excluded.node_id,
       session_name = COALESCE(excluded.session_name, sessions.session_name),
       account_email = COALESCE(excluded.account_email, sessions.account_email),
       plan_tier = COALESCE(excluded.plan_tier, sessions.plan_tier),
       latest_model_id = excluded.latest_model_id,
       total_input_tokens = excluded.total_input_tokens,
       total_output_tokens = excluded.total_output_tokens,
       context_used_pct = excluded.context_used_pct,
       total_cost_usd = excluded.total_cost_usd,
       total_duration_ms = excluded.total_duration_ms,
       total_lines_added = excluded.total_lines_added,
       total_lines_removed = excluded.total_lines_removed,
       last_active_at = excluded.last_active_at;
     ```
   - **Step 3: 流水明细插入**
     执行 `INSERT INTO telemetry_events (...) VALUES (...)`。
   - **Step 4: 配额状态快照更新 (仅当存在 quota 且有 email 时)**
     遍历 quota buckets 执行：
     ```sql
     INSERT INTO account_quotas (account_email, quota_key, remaining_fraction, reset_time, reset_in_seconds, last_node_id, updated_at)
     VALUES (?, ?, ?, ?, ?, ?, ?)
     ON CONFLICT(account_email, quota_key) DO UPDATE SET
       remaining_fraction = excluded.remaining_fraction,
       reset_time = excluded.reset_time,
       reset_in_seconds = excluded.reset_in_seconds,
       last_node_id = excluded.last_node_id,
       updated_at = excluded.updated_at;
     ```

---

## 4. 客户端架构与 ANSI 渲染设计 (`statusline`)

### 4.1 终端渲染美学规范 (继承自 temp)
```text
[~] (main*) [Gemini 3.8 Flash (High)] \\\\\\\\ 0% [1M] | ↑0 ↓0 | Gemini: 93%(2h10m) 89%(5d18h) | Claude: 100%(4h59m) 99%(5d18h)
```
- **工作区目录 `[~]`**: 天蓝色（Sky Blue `\033[38;5;39m`），家目录自动折叠为 `~`。
- **Git 仓库分支 `(main*)`**: 浅薰衣草紫（`\033[38;5;141m`），有未提交改动时追加黄色星号 `*`。
- **模型标识 `[Gemini 3.8 Flash (High)]`**: 粗体亮绿（`\033[1;32m`）。
- **8 段斜杠进度条 `\\\\\\\\ 0% [1M]`**:
  - 已用进度：绿色（`<70%`）、黄色（`70~89%`）、红色（`≥90%`）。
  - 未用进度：暗灰（`\033[90m`）。
  - 百分比：亮白（`\033[1;37m`）。
- **Token 总和 `↑input ↓output`**: 明青色 `↑` 与品红色 `↓`，数值纯白，紧凑数字缩写（`1.5k`, `231k`, `1.5M`）。
- **配额指示器**:
  - Gemini: 明青色标签 `Gemini:`，绿/黄/红余量百分比，暗灰倒计时（`2h10m` / `5d18h`）。
  - Claude: 暖橙色标签 `Claude:`，支持多阶窗口倒计时。

### 4.2 窄屏自适应降级排版 (TerminalWidth 驱动)
- **Level 0 (宽度充足)**：完整输出全部字段。
- **Level 1 (宽度不足)**：模型名称精简为短词（如 `[Flash]`, `[Sonnet]`），配额标签精简（`G:`, `C:`）。
- **Level 2 (中度紧缩)**：进度条由 8 段压缩为 4 段，配额仅保留最近的单桶倒计时。
- **Level 3 (极窄屏幕)**：隐藏工作区目录，仅保留模型、紧凑进度条与 Token 箭头。

### 4.3 节点指纹识别与脱敏 (`internal/node`)
- **优先级 1**：读取 `config.yaml` 或环境变量 `STATUSLINE_NODE_ID` 中配置的手动标识。
- **优先级 2**：读取操作系统硬件标识：
  - **Linux**: 读取 `/etc/machine-id` 或 `/var/lib/dbus/machine-id`。
  - **Windows**: 读取注册表 `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Cryptography\MachineGuid`。
  - **Darwin**: 读取 `ioreg -rd1 -c IOPlatformExpertDevice` 中的 `IOPlatformUUID`。
- **脱敏哈希**：对系统原生 ID 执行 SHA256，取前 16 位字符并附加前缀（如 `node_8f9d2a1b4c3e5f6a`），确保唯一且不可逆还原主机敏感信息。

---

## 5. 服务端架构与 Web 仪表盘 (`statusline-server`)

### 5.1 服务端 RESTful API 契约
- `POST /api/v1/report`：遥测数据上报核心接口。接收包装信封，单事务入库。
- `GET /api/v1/sessions`：获取活跃及历史会话列表（支持按 `node_id`、`product_type` 过滤）。
- `GET /api/v1/nodes`：获取所有节点状态列表及最后在线时间。
- `GET /api/v1/overview`：返回聚合大盘数据（活跃节点数、总消耗 Token、今日调用次数、最新账号配额状态）。
- `GET /api/v1/history?session_id=...&limit=100`：获取指定会话的 Token/上下文时序流水数据。

### 5.2 嵌入式深色 Web 仪表盘 (`web/`)
- 使用 Go 1.16+ `//go:embed web/*` 打包入单二进制。
- 纯原生 HTML5/CSS3/Vanilla JS，无前端编译构建包，加载秒级呈现。
- 支持实时刷新轮询、多机器节点卡片切换、Gemini / Claude 配额实时健康刻度盘与会话 Token 消耗走势图。

---

## 6. 验证与测试策略

1. **单元测试 (`_test.go`)**：
   - `internal/model/envelope_test.go`：测试 AGY 与 Claude Code 原始 JSON 解析及归一化正确性，覆盖 Windows GBK 错位清洗。
   - `internal/render/render_test.go`：测试 ANSI 色彩代码及 Level 1~3 窄屏自适应截断。
   - `internal/storage/store_test.go`：使用内存 SQLite 数据库 (`:memory:`) 测试 DDL 自动建表、UPSERT 幂等性、并发 WAL 写入与查询聚合。
   - `internal/node/machine_id_test.go`：测试机器唯一码脱敏哈希算法一致性。
2. **集成与端到端测试**：
   - `cmd/statusline/main_test.go`：模拟管道传递 JSON，验证 stdout 输出 ANSI 单行且进程在 5ms 内以状态码 0 退出。
   - `cmd/statusline-server/main_test.go`：拉起测试 HTTP 服务，连续上报 100 条流水，验证 SQLite 数据库中 4 张表的数据一致性与事务隔离。

---

## 7. 部署与使用命令

- **编译单二进制**：
  ```bash
  # 跨平台静态编译 (纯 Go，无需 CGO)
  CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/statusline ./cmd/statusline
  CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/statusline-server ./cmd/statusline-server
  ```
- **服务端运行**：
  ```bash
  # 启动服务，自动初始化 SQLite 数据库
  ./bin/statusline-server -port 8765 -db ~/.local/share/statusline/statusline.db
  ```
- **客户端接入**：
  - AGY CLI：配置 `settings.json` 的 statusline 命令为 `~/.local/bin/statusline`。
  - Claude Code：配置 statusline 钩子调用 `statusline` 二进制。
