# 基于优先级的细粒度渐进式优雅降级排版与云端配置同步系统实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现基于模块优先级的 3 阶段细粒度渐进式优雅降级排版引擎，支持服务端 SQLite 持久化、Web 控制台可视化排序配置，以及客户端 `statusline init` 自动同步。

**Architecture:** 云端复用 SQLite `display_aliases`（`category = 'priority'`）存储 7 个核心模块优先级权重（1~7）；Web 控制台「别名映射」弹窗新增「模块优先级」Tab 支持上下移动排序与一键恢复默认；客户端 `statusline init` 将服务端优先级同步写入 `client.yaml`；终端排版引擎在屏幕空间不足时，以优先级由低到高的顺序执行「Phase 1 渐进短名 -> Phase 2 渐进结构紧凑 -> Phase 3 渐进隐藏剪裁」的贪心状态机流水线。

**Tech Stack:** Go 1.22+, modernc.org/sqlite, HTML5/CSS/JavaScript (Vanilla embedded), YAML, log/slog.

**Spec:** `docs/superpowers/specs/2026-11-15-priority-degradation-design.md`

## Global Constraints

- **Strict Latency Budget:** 终端状态行渲染管线从 stdin 到 stdout 耗时严格保持在 < 2ms 内，渲染期间绝不发起任何网络 I/O。
- **Fail-Silent & Offline-First:** 无论云端连通与否、配置文件缺失与否，客户端均无缝回退至内置默认优先级（`model > context > quota > vcs > tokens > account > dir`）。
- **Zero CGO:** 客户端与服务端纯静态编译（`CGO_ENABLED=0`）。
- **Backward Compatibility:** 兼容缺少 `display.priorities` 的既有 `client.yaml`。

## Review Focus

1. **终端自适应管道符孤立:** 当低优先级模块被隐藏（如 `tokens` 或 `dir`）时，不能出现首尾悬挂管道符 `|` 或连续管道符 `| |`。
2. **极端超窄屏保底:** 当终端宽度极小（如 30~50 字符）导致所有低优先级模块均被隐藏时，系统必须保底渲染最高优先级模块（如 `model`），绝不空屏或 panic。
3. **优先级反转验证:** 当用户在云端将 `dir` 设为优先级 1、`model` 设为优先级 7 时，窄屏下必须优先裁剪 `model` 并保护 `dir`。
4. **云端无配置自愈:** 全新部署的数据库首次调用 `/api/v1/config/aliases` 时，服务端能自动播种 7 项默认优先级并持久化，不返回空切片。
5. **Windows 兼容换行与编码:** Web 端与客户端 YAML 写入保持纯 UTF-8 与平台兼容换行。

---

### Task 1: 数据模型与 SQLite 持久化 (Data Models & Storage)

**Files:**
- Modify: `internal/model/envelope.go:400-415`
- Modify: `internal/storage/store.go:756-830`
- Test: `internal/storage/store_test.go`

**Interfaces:**
- Consumes: `display_aliases` SQLite table schema.
- Produces:
  - `model.DisplayAliasesConfig.Priorities []DisplayAliasItem`
  - `model.DefaultPriorityAliases() []DisplayAliasItem`
  - `store.GetDisplayAliases() (*model.DisplayAliasesConfig, error)` 填充 `Priorities`
  - `store.SaveDisplayAliases(items []model.DisplayAliasItem) error` 支持 `category = 'priority'`

- [ ] **Step 1: Write failing test in `store_test.go` for priority storage and querying**
- [ ] **Step 2: Run test to verify it fails**
- [ ] **Step 3: Update `internal/model/envelope.go` to add `Priorities` field and `DefaultPriorityAliases()`**
- [ ] **Step 4: Update `internal/storage/store.go` to scan and save `category = 'priority'` rows**
- [ ] **Step 5: Run tests to verify they pass**
- [ ] **Step 6: Commit**

---

### Task 2: 服务端 API 端点与默认优先级自愈 (Server API & Auto-Seed)

**Files:**
- Modify: `internal/server/handlers_api.go:280-340`
- Test: `internal/server/handlers_test.go`

**Interfaces:**
- Consumes: `store.GetDisplayAliases()` and `store.SaveDisplayAliases()`.
- Produces:
  - `GET /api/v1/config/aliases` 包含 `priorities: [...]`；若为空自动播种并持久化。
  - `POST /api/v1/config/aliases` 接收并保存批量优先级。

- [ ] **Step 1: Write failing test in `handlers_test.go` for `GET /api/v1/config/aliases` returning seeded priorities**
- [ ] **Step 2: Run test to verify it fails**
- [ ] **Step 3: Implement auto-seeding in `handleGetAliases` in `internal/server/handlers_api.go`**
- [ ] **Step 4: Run tests to verify they pass**
- [ ] **Step 5: Commit**

---

### Task 3: 地端配置扩展与自动同步 (`client.yaml` & `statusline init`)

**Files:**
- Modify: `internal/config/config.go:50-140`
- Modify: `client.yaml:20-40`
- Modify: `internal/installer/init.go:210-280`
- Test: `internal/config/config_test.go`
- Test: `internal/installer/init_test.go`

**Interfaces:**
- Consumes: Server `/api/v1/config/aliases` response.
- Produces:
  - `config.DisplayConfig.Priorities []string`
  - `config.DefaultPriorities() []string`
  - `installer.SyncAliasesFromRemote` 自动提取 `priorities` 写入 `client.yaml`。

- [ ] **Step 1: Write failing tests in `config_test.go` and `init_test.go`**
- [ ] **Step 2: Run tests to verify they fail**
- [ ] **Step 3: Update `internal/config/config.go` and `client.yaml` to include `display.priorities`**
- [ ] **Step 4: Update `internal/installer/init.go` to parse remote priorities and write to `client.yaml`**
- [ ] **Step 5: Run tests to verify they pass**
- [ ] **Step 6: Commit**

---

### Task 4: Web 仪表盘模块优先级管理 (Web Dashboard UI)

**Files:**
- Modify: `cmd/statusline-server/web/index.html:240-265`
- Modify: `cmd/statusline-server/web/app.js:1075-1350`

**Interfaces:**
- Consumes: `GET /api/v1/config/aliases` priorities array, `POST /api/v1/config/aliases`.
- Produces:
  - Tab `⚡ 模块优先级 (Priorities)`
  - 优先级列表、`#1`~`#7` 徽标、中文说明、`▲`/`▼` 快速排序按钮与「↺ 恢复默认排序」按钮。

- [ ] **Step 1: Update `index.html` to add `data-tab="priority"` button and table container**
- [ ] **Step 2: Update `app.js` to render priority rows with badges, descriptions and move buttons**
- [ ] **Step 3: Add click handlers for up/down swapping, default reset, and automatic saving via `POST /api/v1/config/aliases`**
- [ ] **Step 4: Verify web UI files syntax and commit**

---

### Task 5: 终端渲染引擎与 3 阶段贪心降级流水线 (Rendering Engine)

**Files:**
- Modify: `internal/render/layout.go:1-135`
- Modify: `cmd/statusline/main.go:30-60`
- Test: `internal/render/render_test.go`

**Interfaces:**
- Consumes: `model.StatusLineData`, `homeDir`, `segments []string`, `priorities []string`.
- Produces:
  - `render.AssembleLine(data, homeDir, segments, priorities...)`
  - 3 阶段贪心状态机：
    - Phase 1: 渐进短名 (`FormatDirCompact`, `formatAccountCompact`, `formatModelCompact`, `formatQuotaCompact(..., true, false)`)
    - Phase 2: 渐进结构紧凑 (`formatContextCompact`, `formatQuotaCompact(..., true, true)`)
    - Phase 3: 渐进隐藏剪裁 (从低优先级到高优先级逐个剔除，保护最高优先级核心模块)

- [ ] **Step 1: Write failing tests in `render_test.go` covering 140, 100, 75, 50 chars and priority inversion**
- [ ] **Step 2: Run tests to verify they fail**
- [ ] **Step 3: Implement 3-phase greedy degradation pipeline in `internal/render/layout.go`**
- [ ] **Step 4: Update `cmd/statusline/main.go` to pass `cfg.Display.Priorities` into `AssembleLine`**
- [ ] **Step 5: Run all render tests to verify they pass**
- [ ] **Step 6: Commit**

---

### Task 6: 全链路回归验证、二进制构建与本地运行时替换

**Files:**
- Build: `bin/statusline`, `bin/statusline-server`
- Runtime: `/opt/software/statusline/statusline`

- [ ] **Step 1: Run full test suite `go test -count=1 ./...`**
- [ ] **Step 2: Compile static binaries for Linux amd64**
- [ ] **Step 3: Atomically update active runtime `/opt/software/statusline/statusline`**
- [ ] **Step 4: Verify rendering output with sample JSON input at different terminal widths**
- [ ] **Step 5: Update documentation and agent memory**
