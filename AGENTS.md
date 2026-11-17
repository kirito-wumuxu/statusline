# AGENTS.md - statusline & statusline-server

## 1. 项目目标与定位
`statusline` 是专为 AI 命令行助手（Google Antigravity `agy` 与 Anthropic Claude Code）打造的高性能跨平台终端状态栏插件、集中遥测监控系统与独立 TUI 会话管理器。
- **`statusline`（客户端插件）**：标准输入读取 CLI 推送的实时会话 JSON，<2ms 内向终端渲染 ANSI 状态行至 `stdout`，并异步非阻塞上报标准信封至服务端。
- **`statusline-server`（服务端与 Web 仪表盘）**：纯 Go 零 CGO 独立常驻服务，基于嵌入式 SQLite3（WAL 高并发模式）实现 4 表事务化持久存储，并内嵌深色实时 Web 监控仪表盘。
- **`statusline-session`（独立 TUI 会话管理器，别名 `sls`）**：独立现代化双栏 TUI 终端会话管理器（基于 Bubble Tea + Lip Gloss），严格按本机设备 ID 过滤会话，提供模糊搜索、转录历史对话预览与一键切目录接管恢复（支持 Tmux 独立窗口拉起与剪贴板复制）。

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
- **窄屏自适应与模块优先级渐进降级**：
  - 基于模块优先级（`display.priorities`）驱动 3 阶段贪心状态机流水线（Phase 1 渐进短名 -> Phase 2 渐进结构紧凑 -> Phase 3 渐进隐藏剪裁），严格保证状态行 $\le$ `TerminalWidth`，永不自动折行，且最大限度保护高优先级核心指标。

---

## 3. 架构设计与目录结构

```text
.
├── client.yaml                     # 客户端配置文件模板 (上报、节点指纹、模块化显示、日志设置)
├── server.yaml                     # 服务端配置文件模板 (端口、SQLite WAL 存储路径、日志设置)
├── go.mod                          # Go 模块定义 (Go 1.22+, modernc.org/sqlite)
├── cmd/
│   ├── statusline/                 # 客户端入口 (stdin -> render -> async report)
│   │   ├── main.go
│   │   └── main_test.go
│   ├── statusline-server/          # 服务端入口 (HTTP 监听、自动 Migrate)
│   │   ├── main.go
│   │   ├── main_test.go
│   │   └── web/                    # 嵌入式深色 Web 仪表盘 (//go:embed)
│   │       ├── index.html
│   │       ├── app.js
│   │       └── style.css
│   └── statusline-session/         # 独立 TUI 会话管理器入口 (别名 sls)
│       ├── main.go
│       ├── main_test.go
│       ├── app.go                  # Bubble Tea 响应式状态机
│       ├── app_test.go
│       └── ui/                     # Lip Gloss 深色排版与双栏组件
│           ├── styles.go
│           ├── list_pane.go
│           ├── detail_pane.go
│           └── preview_pane.go
├── internal/
│   ├── session/                    # 会话服务、本设备 Node ID 过滤引擎与 Launcher 恢复执行器
│   │   ├── service.go
│   │   ├── preview.go
│   │   └── launcher.go
│   ├── config/                     # 配置加载 (YAML + 环境变量覆盖)
│   ├── logging/                    # 结构化日志 (log/slog，固定写入可执行文件同目录，nopHandler零开销)
│   ├── autoupdate/                 # 客户端每日首次调用自动后台静默同步与升级 (防抖状态戳、Detached 子进程)
│   ├── node/                       # 跨平台硬件指纹 (Linux/Windows Machine-ID -> SHA256)
│   ├── model/                      # 遥测模型、归一化与 UTF-8 乱码清洗器
│   ├── render/                     # 终端 ANSI 状态栏渲染核心与自适应排版
│   │   ├── aliases.go              # 实体别名映射引擎与最长前缀匹配 (配额/模型/账号)
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
│   ├── updater/                    # 跨平台原地自我升级引擎 (SemVer比对、解压、校验、原子替换、双源探测)
│   │   ├── version.go              # SemVer 语义化版本解析与比较
│   │   ├── extract.go              # 纯 Go 标准库 (.tar.gz / .zip) 解压与 SHA256 校验
│   │   ├── replace.go              # 跨平台原地原子替换调度
│   │   ├── replace_unix.go         # Unix/Linux Inode 原子重命名与跨分区拷贝替换
│   │   ├── replace_windows.go      # Windows .old 重命名与启动清理规避文件锁
│   │   ├── source.go               # 双源接口与 ReleaseInfo 抽象
│   │   ├── source_server.go        # 自建 statusline-server 分发源与下载器
│   │   ├── source_github.go        # GitHub Releases 官方分发源
│   │   └── updater.go              # 升级 CLI 流程编排与终端交互
│   ├── storage/                    # SQLite3 WAL 存储引擎 (nodes, sessions, events, quotas)
│   │   ├── schema.sql              # 数据库建表 DDL
│   │   ├── db.go                   # 数据库连接池与自动 Migrate
│   │   └── store.go                # 事务化 UPSERT 与查询
│   └── server/                     # 服务端 HTTP 路由与控制器
│       ├── handlers_report.go      # POST /api/v1/report
│       ├── handlers_api.go         # GET /api/v1/sessions, overview, quotas, tokens/daily, debug/raw
│       ├── handlers_upgrade.go     # GET /api/v1/version, GET /api/v1/upgrade/download
│       ├── handlers_release_mgr.go # POST /api/v1/upgrade/upload, POST /api/v1/upgrade/sync-github (API Key 保护)
│       ├── release_scanner.go      # 本地发布包 SemVer 目录扫描与校验和解析
│       ├── middleware.go           # HTTP 请求结构化访问日志中间件
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
- **`display_aliases`**：显示别名与模块优先级映射表（`category: quota | model | account | priority`, `alias_key`, `display_name`, `short_name`, `sort_order`），纯 Web 界面管理 + SQLite 持久化（服务端 YAML 零配置），客户端 `statusline init` 时自动拉取同步。

---

## 5. 开发与部署命令

### 编译本地二进制 (支持 Windows / Linux 跨平台零 CGO 静态编译)
```bash
# Linux amd64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X 'main.Version=v0.2.6'" -o bin/statusline ./cmd/statusline
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X 'main.Version=v0.2.6'" -o bin/statusline-server ./cmd/statusline-server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X 'main.Version=v0.2.6'" -o bin/statusline-session ./cmd/statusline-session

# Windows amd64
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X 'main.Version=v0.2.6'" -o bin/statusline.exe ./cmd/statusline
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X 'main.Version=v0.2.6'" -o bin/statusline-server.exe ./cmd/statusline-server
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X 'main.Version=v0.2.6'" -o bin/statusline-session.exe ./cmd/statusline-session
```

### 构建全平台发布包与自动上传 (Release Assets)
```bash
# 构建发布包
./scripts/build-release.sh v0.2.0

# 构建并自动上传至本地或局域网 statusline-server 分发中心:
./scripts/build-release.sh v0.2.0 --upload http://127.0.0.1:8628 --api-key <secret>
```
推送版本 Tag（如 `git tag v0.2.0 && git push origin v0.2.0`）将自动触发 GitHub Actions 构建并发布 Release。

### 运行测试
```bash
go test ./... -v
```

### 启动服务端
```bash
./bin/statusline-server -port 8628 -db ~/.local/share/statusline/statusline.db
```
浏览器访问 `http://127.0.0.1:8628` 即可打开实时深色 Web 监控仪表盘（默认配置文件自动在二进制所在目录下检索 `server.yaml`，顶部支持「🏷️ 别名映射」与「📦 版本管理」）。

### 客户端接入与维护命令
- **一键配置 Antigravity CLI 与 Claude Code**:
  ```bash
  statusline init
  ```
  自动解析当前运行二进制的绝对路径（原地生效，无需移动），并自动配置：
  - Antigravity: `~/.gemini/antigravity-cli/settings.json`（或 `~/.gemini/settings.json`）
  - Claude Code: `~/.claude/settings.json`（及工作区 `.claude/settings.json`）
  支持 `-target [all|agy|claude]` 过滤目标工具。客户端默认在二进制所在目录下检索 `client.yaml`。
- **客户端与服务端一键原地自我升级 (`upgrade` 或 `update`)**:
  ```bash
  statusline upgrade               # 检查并一键安全升级客户端到最新版
  statusline upgrade -check        # 仅检查是否有新版本，不执行下载与安装
  statusline upgrade -all          # 升级 statusline 同时自动升级同目录下的 statusline-server
  statusline upgrade -version v0.2.5 # 升级或回退至指定版本
  statusline upgrade -source server # 指定更新源 (auto: 优先服务端回退GitHub, server: 仅服务端, github: 仅GitHub)
  statusline upgrade -y            # 静默升级，跳过终端交互确认

  # 服务端同样原生支持 upgrade 命令：
  statusline-server upgrade        # 原地自我升级 statusline-server 服务端
  statusline-server upgrade -all   # 同时升级 statusline-server 及同目录下的 statusline
  statusline-server upgrade -check # 检查服务端可用更新
  ```
- **客户端每日首次调用自动后台静默同步与升级 (`auto-update`)**:
  ```bash
  statusline auto-update           # 手动触发每日配置同步与自动升级检查
  statusline auto-update -check    # 仅检查新版本与配置，不修改本地文件
  statusline auto-update -force    # 强制执行（即使今日已执行过）
  ```
  AI CLI (`agy` / `claude`) 每天首次唤起 `statusline` 渲染状态行后，自动在后台静默拉起独立 Detached 进程：从服务端同步别名与优先级并写入 `client.yaml`，若检测到官方发布新版本则自动执行原地原子升级（零终端延迟、零 stdout 污染、并发状态戳防抖）。可通过 `client.yaml` 中的 `auto_sync:` 块灵活开启或关闭。
- **独立 TUI 会话管理器与快速恢复 (`statusline-session` / `sls`)**:
  ```bash
  statusline-session               # 启动交互式 TUI 会话管理器（默认严格展示当前设备活跃会话）
  statusline-session -a            # 查看跨设备全量会话（包含异地机器记录）
  statusline-session -db <path>    # 指定自定义 SQLite 数据库路径
  ```
  在双栏 TUI 界面中：
  - `↑/↓` 或 `k/j`：浏览会话列表，右侧即时联动显示元数据卡片与最后 2 轮问答（Prompt 与 Model 回复摘要）语法高亮预览；
  - `/`：激活即时模糊搜索框，支持按项目名、分支、工具名或会话 ID 快速筛选；
  - `Enter`：退出 TUI，在当前终端自动 `cd <CWD>` 并唤起对应 AI CLI（`claude --resume` 或 `agy`）原生接管；
  - `t`：在当前 Tmux 独立新窗口中打开并恢复该会话；
  - `c`：将完整的 `cd <CWD> && <command>` 恢复指令复制到系统剪贴板；
  - `a`：在「仅本机设备会话」与「全部跨设备会话」之间快速切换。
- **自定义环境变量**:
  - `export STATUSLINE_SERVER_URL="http://127.0.0.1:8628/api/v1/report"`（修改上报地址）
  - `export STATUSLINE_SERVER_URL="off"`（停用遥测上报，纯本地离线渲染）
  - `export STATUSLINE_LOG_ENABLED="true"`（强制启用日志）
  - `export STATUSLINE_LOG_LEVEL="debug"`（日志级别：debug / info / warn / error）
  - `export STATUSLINE_LOG_FORMAT="json"`（日志格式：text / json）

---

## 6. 日志子系统规范 (Logging)

- **零外部依赖**：基于 Go 标准库 `log/slog` 实现结构化日志。
- **只写文件**：
  - 日志**固定写入可执行文件所在目录**，绝不写入 `stdout`（严防污染客户端终端状态行管道）。
  - 客户端日志文件名为 `statusline.log`（默认关闭，`enabled: false`，通过 `nopHandler` 保证零分配零开销）。
  - 服务端日志文件名为 `statusline-server.log`（默认开启，`enabled: true`，级别 `info`）。
- **配置与覆盖**：
  - 可在 `client.yaml` / `server.yaml` 中的 `logging:` 块配置 `enabled`, `level`, `format`。
  - 支持通过环境变量 `STATUSLINE_LOG_ENABLED`, `STATUSLINE_LOG_LEVEL`, `STATUSLINE_LOG_FORMAT` 临时动态覆盖。
