# 基于 ccusage 真实 API Token 吞吐采集与转录同步体系实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建对齐 `ccusage` 业界标准的原子级 API Token 记录与转录同步引擎，支持客户端微秒级转录尾部探测与服务端全量/增量转录同步，提供毫秒级原子聚合求和与 7 列深色 Web 监控看板。

**Architecture:** 采用双擎架构：引擎 A（客户端轻量游标探测）在渲染完成后基于本地 `.offset` 缓存 (<0.05ms) 读取新增 Step 的真实 `input_tokens`、`output_tokens` 与 `cache_read_tokens`；引擎 B（服务端转录同步器）通过 `POST /api/v1/sync/transcripts` 及启动检查扫描 Antigravity/Claude Code 日志，并幂等回填历史轮次；数据库废弃窗口函数推演，直接采用原子字段极速 `SUM(...)`。

**Tech Stack:** Go 1.22+, modernc.org/sqlite (SQLite3 WAL, Zero CGO), 标准库 `io`, `os`, `encoding/json`, 原生深色 Web 仪表盘 (HTML5 / Tailwind CDN / Vanilla JS).

**Spec:** `docs/superpowers/specs/2026-11-16-ccusage-token-architecture-design.md`

## Global Constraints

- **零外部运行时依赖**：纯 Go 标准库与既有嵌入式驱动，严禁引入 CGO 或额外重型三方库。
- **客户端终端渲染热路径严格 < 2ms**：增量探测读取必须在终端 ANSI 输出完成后触发，探测超时严格熔断（5ms）。
- **服务端聚合查询耗时 < 2ms**：彻底废弃全表分区窗口函数 `LAG() OVER (...)`，基于原子事件列执行单步索引求和。
- **幂等性与数学守恒**：转录日志同步基于 `(session_id, prompt_id)` 唯一索引去重，多次同步数据不膨胀、不重复插入。
- **向后兼容**：保持现有 `/api/v1/tokens/daily` 响应结构向后兼容，平滑新增 `cache_read_tokens` 与 `total_cache_read_tokens` 字段。

## Review Focus

1. **转录文件被截断或轮转时游标处理**：若当前文件长度小于上次记录的 byte offset，探测器自动重置 offset 为 0，避免越界 panic 或漏记。
2. **瞬时超大日志安全熔断**：单次增量扫描设定最大读取字节保护（如 2MB），超长行安全截断或分批处理，严防内存暴涨。
3. **Claude Code 与 Antigravity 转录格式异构**：Antigravity 为顶层 `PLANNER_RESPONSE` + `input_tokens`/`output_tokens`/`cache_read_tokens`；Claude 为嵌套 `message.usage`，解析器须自适应识别两种格式。
4. **历史未记录 ModelID 的转录回填归因**：历史转录 Step 未内嵌 ModelID 时，从 `sessions.latest_model_id` 或已有 events 关联推导，避免归为 `unknown`。
5. **并发同步写竞争规避**：服务端转录同步采用写互斥锁 `s.mu.Lock()` 配合批量事务提交，避免并发写冲突锁表。

---

### Task 1: 客户端转录日志微秒级增量游标探测器 (`internal/transcript`)

**Files:**
- Create: `internal/transcript/tailer.go`
- Create: `internal/transcript/tailer_test.go`
- Modify: `cmd/statusline/main.go:96-108`

**Interfaces:**
- Produces:
  ```go
  type TurnUsage struct {
      InputTokens         int64
      OutputTokens        int64
      CacheReadTokens     int64
      CacheCreationTokens int64
  }
  func ProbeRecentUsage(transcriptPath string, sessionID string, timeout time.Duration) TurnUsage
  ```
- Consumes: `model.TelemetryEnvelope` from `internal/model`

- [ ] **Step 1: 编写客户端增量游标探测器失败测试**

在 `internal/transcript/tailer_test.go` 中编写针对增量追加读取、游标持久化及格式解析的单元测试：
```go
package transcript

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProbeRecentUsage(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "transcript.jsonl")
	sessionID := "test-session-123"

	// 1. 写入初始行 (普通用户请求，无 usage)
	line1 := `{"step_index": 0, "type": "USER_INPUT", "created_at": "2026-11-16T10:00:00Z"}` + "\n"
	if err := os.WriteFile(logPath, []byte(line1), 0644); err != nil {
		t.Fatal(err)
	}

	u1 := ProbeRecentUsage(logPath, sessionID, 50*time.Millisecond)
	if u1.InputTokens != 0 || u1.OutputTokens != 0 {
		t.Fatalf("expected 0 tokens, got %+v", u1)
	}

	// 2. 追加 Antigravity 格式的 PLANNER_RESPONSE
	line2 := `{"step_index": 1, "type": "PLANNER_RESPONSE", "input_tokens": 2570, "output_tokens": 89, "cache_read_tokens": 249741}` + "\n"
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(line2)
	f.Close()

	u2 := ProbeRecentUsage(logPath, sessionID, 50*time.Millisecond)
	if u2.InputTokens != 2570 || u2.OutputTokens != 89 || u2.CacheReadTokens != 249741 {
		t.Fatalf("unexpected tokens for step 1: %+v", u2)
	}

	// 3. 再次探测，无新内容应返回 0
	u3 := ProbeRecentUsage(logPath, sessionID, 50*time.Millisecond)
	if u3.InputTokens != 0 || u3.OutputTokens != 0 || u3.CacheReadTokens != 0 {
		t.Fatalf("expected 0 tokens on second read, got %+v", u3)
	}

	// 4. 追加 Claude Code 格式日志
	line3 := `{"sessionId": "test-session-123", "message": {"model": "claude-3-7-sonnet", "usage": {"input_tokens": 1200, "output_tokens": 350, "cache_read_input_tokens": 8500, "cache_creation_input_tokens": 500}}}` + "\n"
	f, err = os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(line3)
	f.Close()

	u4 := ProbeRecentUsage(logPath, sessionID, 50*time.Millisecond)
	if u4.InputTokens != 1200 || u4.OutputTokens != 350 || u4.CacheReadTokens != 8500 || u4.CacheCreationTokens != 500 {
		t.Fatalf("unexpected tokens for claude step: %+v", u4)
	}
}
```

- [ ] **Step 2: 运行测试以验证失败**

运行：`go test -v ./internal/transcript`
预期：FAIL（`package statusline/internal/transcript: cannot find package`）

- [ ] **Step 3: 实现 `ProbeRecentUsage` 增量探测器**

在 `internal/transcript/tailer.go` 中实现：
1. `offset` 状态缓存文件路径：`filepath.Join(os.TempDir(), fmt.Sprintf(".statusline_tail_%s.offset", sanitizeSessionID(sessionID)))`；
2. 比较文件真实大小与 `last_offset`：若文件被缩减则重置 `offset = 0`；
3. `f.Seek(last_offset, io.SeekStart)` 顺序读取新追加的行；
4. 解析 JSON 并同时兼容 Antigravity（顶层 `input_tokens` / `output_tokens` / `cache_read_tokens`）与 Claude Code（`message.usage`）；
5. 记录并保存新的 `offset`；
6. 使用通道/计时器实现 5ms 严格熔断。

- [ ] **Step 4: 将 `ProbeRecentUsage` 接入 `cmd/statusline/main.go`**

在 `cmd/statusline/main.go` 中，在 `render.AssembleLine` 渲染并输出至 `stdout` 后、调用 `reporter.Report` 之前：
```go
	// Probe real API tokens from transcript tail if path is available
	if env != nil && env.Normalized.TranscriptPath != "" {
		usage := transcript.ProbeRecentUsage(env.Normalized.TranscriptPath, env.Normalized.SessionID, 5*time.Millisecond)
		if usage.InputTokens > 0 || usage.OutputTokens > 0 || usage.CacheReadTokens > 0 || usage.CacheCreationTokens > 0 {
			env.Normalized.CurrentInputTokens = usage.InputTokens
			env.Normalized.CurrentOutputTokens = usage.OutputTokens
			env.Normalized.CacheReadTokens = usage.CacheReadTokens
			env.Normalized.CacheCreationTokens = usage.CacheCreationTokens
		}
	}
```

- [ ] **Step 5: 运行测试确认通过**

运行：`go test -v ./internal/transcript ./cmd/statusline`
预期：PASS

- [ ] **Step 6: 提交代码**

```bash
git add internal/transcript cmd/statusline/main.go
git commit -m "feat(client): implement microsecond transcript tail parser for atomic turn tokens"
```

---

### Task 2: 服务端转录同步器与 HTTP 同步端点 (`internal/server`)

**Files:**
- Create: `internal/server/transcript_scanner.go`
- Create: `internal/server/transcript_scanner_test.go`
- Modify: `internal/server/server.go:40-46`
- Create: `internal/server/handlers_sync.go`

**Interfaces:**
- Produces:
  ```go
  type SyncResult struct {
      ScannedFiles int   `json:"scanned_files"`
      NewEvents    int   `json:"new_events"`
      ElapsedMS    int64 `json:"elapsed_ms"`
  }
  type TranscriptScanner struct { store *storage.Store }
  func NewTranscriptScanner(store *storage.Store) *TranscriptScanner
  func (s *TranscriptScanner) SyncAll(ctx context.Context) (*SyncResult, error)
  ```
- Consumes: `storage.Store` from `internal/storage`

- [ ] **Step 1: 编写转录同步器单元测试**

在 `internal/server/transcript_scanner_test.go` 中测试：
1. 扫描模拟的 Antigravity `transcript.jsonl` 文件，提取所有 step 并去重入库；
2. 重复调用 `SyncAll`，验证已入库 Step 不重复插入（`NewEvents == 0`）。

```go
package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"statusline/internal/model"
	"statusline/internal/node"
	"statusline/internal/storage"
)

func TestTranscriptScannerSync(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// 预置一个 session
	logPath := filepath.Join(tmpDir, "transcript.jsonl")
	sessID := "sync-test-session"
	rawJSON := `{"session_id":"` + sessID + `","cwd":"/tmp","model":{"id":"gemini-3.8-flash-high"},"transcript_path":"` + logPath + `"}`
	env, _, _ := model.ParseAndNormalize([]byte(rawJSON), node.NodeInfo{NodeID: "node_1"})
	if err := store.Ingest(env); err != nil {
		t.Fatal(err)
	}

	// 写入 2 个 step
	lines := `{"step_index": 1, "type": "PLANNER_RESPONSE", "created_at": "2026-11-16T10:00:00Z", "input_tokens": 1000, "output_tokens": 50, "cache_read_tokens": 20000}` + "\n" +
		`{"step_index": 2, "type": "PLANNER_RESPONSE", "created_at": "2026-11-16T10:01:00Z", "input_tokens": 1200, "output_tokens": 60, "cache_read_tokens": 25000}` + "\n"
	if err := os.WriteFile(logPath, []byte(lines), 0644); err != nil {
		t.Fatal(err)
	}

	scanner := NewTranscriptScanner(store)
	res, err := scanner.SyncAll(context.Background())
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if res.NewEvents != 2 {
		t.Fatalf("expected 2 new events, got %d", res.NewEvents)
	}

	// 再次同步，幂等测试
	res2, err := scanner.SyncAll(context.Background())
	if err != nil {
		t.Fatalf("sync 2 failed: %v", err)
	}
	if res2.NewEvents != 0 {
		t.Fatalf("expected 0 new events on repeat sync, got %d", res2.NewEvents)
	}
}
```

- [ ] **Step 2: 运行测试以验证失败**

运行：`go test -v ./internal/server -run TestTranscriptScannerSync`
预期：FAIL（`undefined: NewTranscriptScanner`）

- [ ] **Step 3: 实现 `TranscriptScanner`**

在 `internal/server/transcript_scanner.go` 中实现：
1. 收集目标转录路径：
   - 优先查询 `SELECT DISTINCT session_id, node_id, product_type, latest_model_id, transcript_path FROM sessions WHERE transcript_path IS NOT NULL AND transcript_path != ''`；
   - 自动扫描家目录下可能遗漏的历史路径：`~/.gemini/antigravity*/brain/*/.system_generated/logs/transcript.jsonl` 与 `~/.claude/projects/*/*.jsonl`；
2. 针对每个文件读取所有有效 Step：
   - 过滤条件：`type == "PLANNER_RESPONSE"` 且 `(input_tokens > 0 || output_tokens > 0 || cache_read_tokens > 0)`；
   - 构造标准唯一标识：`prompt_id = fmt.Sprintf("step_%d", step_index)`；
3. 查询并排除数据库中已存在的 `prompt_id`（通过 `SELECT prompt_id FROM telemetry_events WHERE session_id = ? AND prompt_id IS NOT NULL`）；
4. 开启单事务批量写入未入库的 `telemetry_events`，精确填入 `current_input_tokens`, `current_output_tokens`, `cache_read_tokens`, `reported_at`, `model_id`；
5. 返回统计数据 `SyncResult`。

- [ ] **Step 4: 实现 `POST /api/v1/sync/transcripts` 端点**

创建 `internal/server/handlers_sync.go`：
```go
package server

import (
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) handleSyncTranscripts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	scanner := NewTranscriptScanner(s.store)
	start := time.Now()
	res, err := scanner.SyncAll(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	res.ElapsedMS = time.Since(start).Milliseconds()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(res)
}
```
并在 `internal/server/server.go` 中挂载路由：
`s.router.HandleFunc("POST /api/v1/sync/transcripts", s.handleSyncTranscripts)`
`s.router.HandleFunc("POST /api/sync/transcripts", s.handleSyncTranscripts)`

- [ ] **Step 5: 运行测试确认通过**

运行：`go test -v ./internal/server`
预期：PASS

- [ ] **Step 6: 提交代码**

```bash
git add internal/server/transcript_scanner.go internal/server/transcript_scanner_test.go internal/server/handlers_sync.go internal/server/server.go
git commit -m "feat(server): implement transcript backfill scanner and sync API"
```

---

### Task 3: 存储层极速聚合与 Cache Read 字段重构 (`internal/storage`)

**Files:**
- Modify: `internal/storage/store.go:100-110, 450-550, 325-345`
- Modify: `internal/storage/store_test.go`

**Interfaces:**
- Produces:
  ```go
  type DailyModelToken struct {
      ModelID             string  `json:"model_id"`
      DisplayName         string  `json:"display_name"`
      InputTokens         int64   `json:"input_tokens"`
      OutputTokens        int64   `json:"output_tokens"`
      CacheReadTokens     int64   `json:"cache_read_tokens"`
      CacheCreationTokens int64   `json:"cache_creation_tokens"`
      TotalTokens         int64   `json:"total_tokens"`
      Percentage          float64 `json:"percentage"`
      CostUSD             float64 `json:"cost_usd,omitempty"`
  }
  type DailyTokensSummary struct {
      Date                 string            `json:"date"`
      TotalInputTokens     int64             `json:"total_input_tokens"`
      TotalOutputTokens    int64             `json:"total_output_tokens"`
      TotalCacheReadTokens int64             `json:"total_cache_read_tokens"`
      TotalTokens          int64             `json:"total_tokens"`
      TotalCostUSD         float64           `json:"total_cost_usd,omitempty"`
      Models               []DailyModelToken `json:"models"`
  }
  ```

- [ ] **Step 1: 编写聚合查询单元测试**

在 `internal/storage/store_test.go` 中编写针对原子事件聚合（包含 `CacheReadTokens`）的单元测试：
```go
func TestDailyModelTokensAtomicAggregation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_atomic.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// 插入 2 个模型的单步事件
	// 事件 1: gemini-3.8-flash-high, in: 4000, out: 100, cache_read: 60000
	// 事件 2: claude-opus-4-6, in: 2000, out: 50, cache_read: 15000
	sessID := "atomic-sess-1"
	e1 := &model.TelemetryEnvelope{
		ReportedAt: "2026-11-16T10:00:00Z",
		Payload:    map[string]interface{}{},
		Normalized: model.NormalizedData{
			ProductType:         "antigravity",
			SessionID:           sessID,
			ModelID:             "gemini-3.8-flash-high",
			ModelDisplayName:    "Gemini 3.8 Flash (High)",
			CurrentInputTokens:  4000,
			CurrentOutputTokens: 100,
			CacheReadTokens:     60000,
		},
	}
	e2 := &model.TelemetryEnvelope{
		ReportedAt: "2026-11-16T10:05:00Z",
		Payload:    map[string]interface{}{},
		Normalized: model.NormalizedData{
			ProductType:         "antigravity",
			SessionID:           sessID,
			ModelID:             "claude-opus-4-6",
			ModelDisplayName:    "Claude Opus 4.6",
			CurrentInputTokens:  2000,
			CurrentOutputTokens: 50,
			CacheReadTokens:     15000,
		},
	}
	if err := s.Ingest(e1); err != nil {
		t.Fatal(err)
	}
	if err := s.Ingest(e2); err != nil {
		t.Fatal(err)
	}

	summaries, err := s.GetDailyModelTokens(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	sum := summaries[0]
	// 4000 + 100 + 60000 + 2000 + 50 + 15000 = 81150
	expectedTotal := int64(81150)
	if sum.TotalTokens != expectedTotal {
		t.Fatalf("expected total tokens %d, got %d", expectedTotal, sum.TotalTokens)
	}
	if sum.TotalCacheReadTokens != 75000 {
		t.Fatalf("expected cache read tokens 75000, got %d", sum.TotalCacheReadTokens)
	}
}
```

- [ ] **Step 2: 运行测试以验证失败**

运行：`go test -v ./internal/storage -run TestDailyModelTokensAtomicAggregation`
预期：FAIL（`sum.TotalCacheReadTokens undefined`）

- [ ] **Step 3: 重构 `GetDailyModelTokens` 与 `GetOverview`**

在 `internal/storage/store.go` 中：
1. 更新结构体定义，增加 `CacheReadTokens`, `CacheCreationTokens`, `TotalCacheReadTokens`；
2. 重构 `GetDailyModelTokens(days int)` SQL，使用高性能原子汇总：
```sql
	SELECT 
		strftime('%Y-%m-%d', reported_at) AS day,
		model_id,
		COALESCE(MAX(model_display_name), model_id) AS display_name,
		COALESCE(SUM(current_input_tokens), 0) AS in_tokens,
		COALESCE(SUM(current_output_tokens), 0) AS out_tokens,
		COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
		COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
		COALESCE(SUM(current_input_tokens + current_output_tokens + cache_read_tokens + cache_creation_tokens), 0) AS total_tokens
	FROM telemetry_events
	WHERE reported_at >= datetime('now', '-' || ? || ' days')
	GROUP BY day, model_id
	ORDER BY day DESC, total_tokens DESC;
```
3. 更新 `GetOverview()`：
从 `telemetry_events` 汇总真实的 `TotalInputTokens`, `TotalOutputTokens`, `TotalCacheReadTokens` 与 `TotalTokens`，保证大盘数据与日明细完全守恒。

- [ ] **Step 4: 运行测试确认通过**

运行：`go test -v ./internal/storage`
预期：PASS

- [ ] **Step 5: 提交代码**

```bash
git add internal/storage/store.go internal/storage/store_test.go
git commit -m "refactor(storage): simplify daily token aggregation to atomic sum with cache read tokens"
```

---

### Task 4: Web 仪表盘 UI 重塑与转录同步按钮 (`cmd/statusline-server/web`)

**Files:**
- Modify: `cmd/statusline-server/web/index.html:160-195, 80-110`
- Modify: `cmd/statusline-server/web/app.js:880-970, 1770-1810`

**Interfaces:**
- Frontend calls: `POST /api/v1/sync/transcripts`
- Frontend renders: 7-column table with `缓存读取 (⚡ Cache)` and symmetric token breakdown in metrics cards.

- [ ] **Step 1: 更新 `index.html` 布局结构**

1. 在 Tab 3 工具栏（`cmd/statusline-server/web/index.html`）新增同步操作按钮：
```html
<button id="btn-sync-transcripts" onclick="triggerTranscriptSync()" class="px-3.5 py-1.5 text-xs font-semibold rounded-lg bg-sky-600 hover:bg-sky-500 text-white flex items-center gap-1.5 transition shadow-sm">
  <span id="sync-icon">🔄</span> 同步本地转录
</button>
```
2. 在 `#daily-tokens-table` 表头插入第 5 列：
```html
<th class="py-3 px-4" style="width: 140px;">缓存命中 (⚡ Cache)</th>
```
3. 在指标卡 2 和指标卡 3 的微标详情中预留三维布局 `↑in ↓out ⚡cache`。

- [ ] **Step 2: 更新 `app.js` 渲染逻辑与交互事件**

1. 更新 `renderDailyTokensTable`:
   - 提取各模型与各日期的 `cache_read_tokens` / `total_cache_read_tokens`；
   - 汇总行与明细行渲染 7 列，新增 `<td class="py-2.5 px-4 text-amber-300">${formatTokens(m.cache_read_tokens || 0)}</td>`；
2. 更新顶部卡片渲染（`updateOverview` / `refreshData`）：
   - 卡片 2 微标详情：`↑${formatTokens(todayInput)} ↓${formatTokens(todayOutput)} ⚡${formatTokens(todayCache)}`；
   - 卡片 3 微标详情：`↑${formatTokens(totalInput)} ↓${formatTokens(totalOutput)} ⚡${formatTokens(totalCache)}`；
3. 新增 `triggerTranscriptSync()` 函数：
   - 按钮显示加载旋转态；
   - `fetch('/api/v1/sync/transcripts', { method: 'POST' })`；
   - 解析返回结果 `res.new_events`，弹出通知提示并触发 `refreshData(true)` 强制刷新表格。

- [ ] **Step 3: 运行服务器端全套端到端与 HTTP 测试**

运行：`go test -v ./internal/server`
预期：PASS

- [ ] **Step 4: 提交代码**

```bash
git add cmd/statusline-server/web/index.html cmd/statusline-server/web/app.js
git commit -m "feat(web): add cache read column, 3-metric cards, and transcript sync button"
```

---

### Task 5: 全链路端到端集成验证、历史转录全量同步与产物编译

**Files:**
- Modify: `.agent-memory/PROGRESS.md`
- Compile: `bin/statusline`, `bin/statusline-server`

- [ ] **Step 1: 全库单元测试验证**

运行全库测试，确认 100% 通过：
```bash
go test -count=1 ./...
```
预期：All 14 packages PASS.

- [ ] **Step 2: 静态编译本地全量二进制**

执行生产构建：
```bash
CGO_ENABLED=0 go build -ldflags="-s -w -X 'main.Version=v0.2.3'" -o bin/statusline ./cmd/statusline
CGO_ENABLED=0 go build -ldflags="-s -w -X 'main.Version=v0.2.3'" -o bin/statusline-server ./cmd/statusline-server
```

- [ ] **Step 3: 启动服务端执行转录同步并对齐 `ccusage`**

1. 启动临时服务实例执行同步探测：
```bash
curl -X POST http://127.0.0.1:8628/api/v1/sync/transcripts
```
2. 查询 `/api/v1/tokens/daily` 并与 `npx ccusage daily --json` 对比 2026-11-12 数据：
   - 验证总 Token 约为 `73.3M`；
   - 验证 CacheRead 约为 `68.7M`；
   - 验证 `gpt-oss-120b-medium` 输出为真实的 `1,187` 而非虚假的 `353k`；
   - 误差在 < 0.1% 范围内。

- [ ] **Step 4: 更新进度档案与记忆**

更新 `.agent-memory/PROGRESS.md`，记录 Task 1 ~ Task 5 完成情况及验证数据。

- [ ] **Step 5: 提交最终工程改动**

```bash
git add .agent-memory/PROGRESS.md
git commit -m "chore(release): complete ccusage token architecture overhaul v0.2.3"
```
