# AGENTS.md - statusline & statusline-server

## 1. 项目目标与定位
`statusline` 是专为 AI 命令行助手（Google Antigravity `agy` 与 Anthropic Claude Code）打造的高性能跨平台终端状态栏插件与集中遥测监控系统。
- **`statusline`（客户端插件）**：标准输入读取 CLI 推送的实时会话 JSON，<2ms 内向终端渲染 ANSI 状态行至 `stdout`，并异步非阻塞上报标准信封至服务端。
- **`statusline-server`（服务端与 Web 仪表盘）**：纯 Go 零 CGO 独立常驻服务，基于嵌入式 SQLite3（WAL 高并发模式）实现 4 表事务化持久存储，并内嵌深色实时 Web 监控仪表盘。

---

## 2. 终端显示与美学色彩规范

```text
[~] (main*) <kirito_wu@163.com> [Gemini 3.8 Flash (High)] \\\\\\\\ 0% [1M] | ↑0 ↓0 | Gemini: 93%(2h10m) 89%(5d18h) | Claude: 100%(4h59m) 99%(5d18h)
```

- **工作区目录 `[~]`**：天蓝色（Sky Blue `\033[38;5;39m`），家目录自动折叠为 `~`。
- **Git 仓库分支 `(main*)`**：浅薰衣草紫（Lavender `\033[38;5;141m`），脏工作区带黄色标记（Yellow `*`）。
- **账号标识 `<user@email.com>`**：暖琥珀金（Amber `\033[38;5;214m`），暗灰尖括号（`\033[90m< >\033[0m`），无账号时自动跳过。
- **模型标识 `[Gemini 3.8 Flash (High)]`**：粗体亮绿（Bold Bright Green `\033[1;32m`）。
- **上下文进度 `\\\\\\\\ 22% [1M]`**：
  - 8 段斜杠进度条：`<70%` 绿、`70-89%` 黄、`≥90%` 红；未用进度为暗灰（`\033[90m`）。
  - 百分比：亮白（`\033[1;37m`）；容量：暗灰括号加白字 `[1M]`。
- **会话 Token 总和 `↑input ↓output`**：
  - `↑` 输入 Token（明青色 `\033[36m`），`↓` 输出 Token（品红色 `\033[35m`），紧凑单位展示（如 `1.5k`, `231k`, `1.5M`）。
- **配额余量与倒计时**：
  - Gemini: 明青色标签 `Gemini:`，绿/黄/红余量百分比，暗灰倒计时（`2h10m` / `5d18h`）。
  - Claude: 暖橙色标签 `Claude:`，支持多阶窗口倒计时。
- **窄屏自适应**：
  - 依据 `TerminalWidth` 自动执行 Level 1~3 降级缩减，防止终端换行。

---

## 3. 架构设计与目录结构

```text
.
├── client.yaml                     # 客户端配置文件模板 (上报、节点指纹、模块化显示)
├── server.yaml                     # 服务端配置文件模板 (端口、SQLite WAL 存储路径)
├── config.yaml                     # 统一配置文件模板 (兼容旧版合并配置)
├── go.mod                          # Go 模块定义 (Go 1.22+, modernc.org/sqlite)
├── cmd/
│   ├── statusline/                 # 客户端入口 (stdin -> render -> async report)
│   │   ├── main.go
│   │   └── main_test.go
│   └── statusline-server/          # 服务端入口 (HTTP 监听、自动 Migrate)
│       ├── main.go
│       ├── main_test.go
│       └── web/                    # 嵌入式深色 Web 仪表盘 (//go:embed)
│           ├── index.html
│           ├── app.js
│           └── style.css
├── internal/
│   ├── config/                     # 配置加载 (YAML + 环境变量覆盖)
│   ├── node/                       # 跨平台硬件指纹 (Linux/Windows Machine-ID -> SHA256)
│   ├── model/                      # 遥测模型、归一化与 UTF-8 乱码清洗器
│   ├── render/                     # 终端 ANSI 状态栏渲染核心与自适应排版
│   │   ├── colors.go               # ANSI 色彩与代码剥除
│   │   ├── layout.go               # 模块化驱动排版与 Level 1~3 降级
│   │   ├── segment_account.go      # 账号尖括号渲染与前缀截断
│   │   ├── segment_context.go      # 8 段斜杠进度条与百分比
│   │   ├── segment_dir.go          # 家目录折叠与工作区指示
│   │   ├── segment_model.go        # 模型名称与 Effort 深度
│   │   ├── segment_quota.go        # 多阶配额与重置倒计时
│   │   ├── segment_tokens.go       # 紧凑单位 Token 吞吐
│   │   └── segment_vcs.go          # Git 分支与脏标记
│   ├── vcs/                        # Git 超时探测工具 (20ms 熔断)
│   ├── reporter/                   # 客户端非阻塞异步上报器 (100ms 超时，静默丢弃异常)
│   ├── storage/                    # SQLite3 WAL 存储引擎 (nodes, sessions, events, quotas)
│   │   ├── schema.sql              # 数据库建表 DDL
│   │   ├── db.go                   # 数据库连接池与自动 Migrate
│   │   └── store.go                # 事务化 UPSERT 与查询
│   └── server/                     # 服务端 HTTP 路由与控制器
│       ├── handlers_report.go      # POST /api/v1/report
│       ├── handlers_api.go         # GET /api/v1/sessions, overview, quotas
│       └── refresher.go            # 终端空闲重绘调度
├── test/
│   └── e2e/                        # 端到端全链路集成测试
└── docs/
    └── superpowers/                # 架构设计与实施计划归档
```

---

## 4. 数据库设计 (SQLite3 WAL)

- **`nodes`**：管理机器指纹（如 `node_8f9d2a1b4c3e5f6a`）、操作系统、架构与最后上报时间。
- **`sessions`**：会话维度快照表，存储产品类型（`antigravity` / `claude_code`）、工作目录、Git 仓库元数据、最新累计 Token、费用与活跃时间。通过 `ON CONFLICT DO UPDATE` 幂等刷新。
- **`telemetry_events`**：瞬时流水明细表，记录每次问答触发的 Token 消耗、Context Window 使用率、缓存命中率及原始 JSON 备份。
- **`account_quotas`**：多账号配额实时状态表，聚合记录各账号的 Gemini / Claude 配额余量与冷却重置时间。

---

## 5. 开发与部署命令

### 编译本地二进制 (支持 Windows / Linux 跨平台零 CGO 静态编译)
```bash
# Linux amd64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/statusline ./cmd/statusline
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/statusline-server ./cmd/statusline-server

# Windows amd64
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/statusline.exe ./cmd/statusline
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/statusline-server.exe ./cmd/statusline-server
```

### 构建全平台发布包 (Release Assets)
```bash
./scripts/build-release.sh v0.1.0
```
推送版本 Tag（如 `git tag v0.1.0 && git push origin v0.1.0`）将自动触发 GitHub Actions 构建并发布 Release。

### 运行测试
```bash
go test ./... -v
```

### 启动服务端
```bash
./bin/statusline-server -port 8765 -db ~/.local/share/statusline/statusline.db
```
浏览器访问 `http://127.0.0.1:8765` 即可打开实时深色 Web 监控仪表盘。

### 客户端接入配置
- **一键配置 Antigravity CLI 与 Claude Code**:
  ```bash
  statusline init
  ```
  自动解析当前运行二进制的绝对路径（原地生效，无需移动），并自动配置：
  - Antigravity: `~/.gemini/antigravity-cli/settings.json`（或 `~/.gemini/settings.json`）
  - Claude Code: `~/.claude/settings.json`（及工作区 `.claude/settings.json`）
  支持 `-target [all|agy|claude]` 过滤目标工具。
- **自定义环境变量**:
  - `export STATUSLINE_SERVER_URL="http://127.0.0.1:8765/api/v1/report"`（修改上报地址）
  - `export STATUSLINE_SERVER_URL="off"`（停用遥测上报，纯本地离线渲染）
