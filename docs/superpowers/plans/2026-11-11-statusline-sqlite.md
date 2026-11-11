# statusline & statusline-server (SQLite3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a high-performance, cross-platform, zero-dependency Statusline CLI telemetry system (`statusline`) and centralized monitoring server (`statusline-server`) backed by pure-Go SQLite3 (WAL mode) with an embedded Web dashboard.

**Architecture:** 
- The client reads AI CLI (`agy` / `claude`) session JSON via stdin, renders ANSI statusline text to stdout in <2ms, and fires an asynchronous, fail-silent HTTP report to the server with node hardware fingerprint.
- The server normalizes Antigravity and Claude Code payloads, executes single-transaction `UPSERT` operations across 4 SQLite3 tables (`nodes`, `sessions`, `telemetry_events`, `account_quotas`), and serves an embedded dark-mode Web dashboard.
- Zero CGO (`modernc.org/sqlite`), static single-binary outputs for Windows and Linux.

**Tech Stack:** Go 1.22+, `modernc.org/sqlite`, `gopkg.in/yaml.v3`, HTML5/CSS3/Vanilla JS (`embed.FS`).

**Spec:** [`docs/superpowers/specs/2026-11-11-statusline-sqlite-design.md`](file:///home/wmx/workspace/repository/statusline/docs/superpowers/specs/2026-11-11-statusline-sqlite-design.md)

---

## Global Constraints

- **Language & Runtime**: Go 1.22+; zero CGO (`CGO_ENABLED=0` compatible across Linux and Windows).
- **SQLite Driver**: `modernc.org/sqlite` (pure Go implementation; no gcc/clang needed).
- **Client Latency & Safety**: Render output to stdout within 2ms; network timeout <= 100ms; errors silently discarded; never write error text to stdout/stderr.
- **Database Schema**: 4 core tables (`nodes`, `sessions`, `telemetry_events`, `account_quotas`) with WAL mode (`PRAGMA journal_mode = WAL`) and 5000ms busy timeout.
- **Character Encoding**: Clean invalid UTF-8 bytes via `strings.ToValidUTF8` before persistence.

---

## Review Focus

1. **Empty or malformed JSON on stdin**: Client must output a fallback statusline and exit 0 without panic or printing error messages.
2. **Server unreachable / offline**: Client must complete stdout rendering, fail silently in background report, and exit 0 in <5ms.
3. **GBK/corrupted UTF-8 strings in session names**: Cleaner must normalize characters to valid UTF-8 and prevent database insert failures.
4. **Burst concurrent writes to SQLite**: SQLite store with WAL mode and `busy_timeout = 5000` must handle concurrent write transactions without "database is locked" errors.
5. **Ultra-narrow terminal width (< 60 cols)**: Layout engine must adaptively compact through Level 1~3 without breaking ANSI escape codes.

---

## Tasks

### Task 1: Project Scaffolding & Configuration Module

**Files:**
- Create: `go.mod`
- Create: `config.yaml`
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces:
  ```go
  type Config struct {
      Server  ServerConfig  `yaml:"server"`
      Node    NodeConfig    `yaml:"node"`
      Display DisplayConfig `yaml:"display"`
  }
  type ServerConfig struct {
      Endpoint  string `yaml:"endpoint"`
      APIKey    string `yaml:"api_key"`
      TimeoutMS int    `yaml:"timeout_ms"`
      Port      int    `yaml:"port"`
      DBPath    string `yaml:"db_path"`
  }
  type NodeConfig struct {
      ID         string            `yaml:"id"`
      CustomTags map[string]string `yaml:"custom_tags"`
  }
  type DisplayConfig struct {
      EnableStdout bool   `yaml:"enable_stdout"`
      Format       string `yaml:"format"`
  }
  func LoadConfig(path string) (*Config, error)
  func DefaultConfig() *Config
  ```

- [ ] **Step 1: Write the failing test for configuration loading and environment variable overrides**

```go
// internal/config/config_test.go
package config

import (
	"os"
	"testing"
)

func testDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Server.Port != 8765 {
		t.Fatalf("expected default port 8765, got %d", cfg.Server.Port)
	}
	if cfg.Server.TimeoutMS != 100 {
		t.Fatalf("expected default timeout 100ms, got %d", cfg.Server.TimeoutMS)
	}
}

func testEnvOverrides(t *testing.T) {
	os.Setenv("STATUSLINE_SERVER_URL", "http://custom:9999/api/v1/report")
	defer os.Unsetenv("STATUSLINE_SERVER_URL")
	cfg := DefaultConfig()
	if cfg.Server.Endpoint != "http://custom:9999/api/v1/report" {
		t.Fatalf("expected env override, got %s", cfg.Server.Endpoint)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/... -v`
Expected: FAIL (package or functions not defined)

- [ ] **Step 3: Implement `go.mod`, `config.yaml`, and `internal/config/config.go`**

Initialize `go.mod` with `module statusline`, add `gopkg.in/yaml.v3` and `modernc.org/sqlite`. Implement `LoadConfig` with YAML decoding and environment variable overrides (`STATUSLINE_CONFIG`, `STATUSLINE_SERVER_URL`, `STATUSLINE_NODE_ID`, `STATUSLINE_DB_PATH`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum config.yaml internal/config/
git commit -m "feat(config): add project scaffolding and configuration module"
```

---

### Task 2: Cross-Platform Node Hardware Fingerprint Module

**Files:**
- Create: `internal/node/machine_id.go`
- Create: `internal/node/machine_id_linux.go`
- Create: `internal/node/machine_id_windows.go`
- Test: `internal/node/machine_id_test.go`

**Interfaces:**
- Consumes: `internal/config.NodeConfig`
- Produces:
  ```go
  type NodeInfo struct {
      NodeID        string `json:"node_id"`
      Hostname      string `json:"hostname"`
      OSType        string `json:"os_type"`
      OSArch        string `json:"os_arch"`
      ClientVersion string `json:"client_version"`
  }
  func GetNodeInfo(explicitID, version string) NodeInfo
  func HashMachineID(raw string) string
  ```

- [ ] **Step 1: Write the failing test for node ID generation and hashing**

```go
// internal/node/machine_id_test.go
package node

import (
	"strings"
	"testing"
)

func testHashMachineID(t *testing.T) {
	raw := "c7d23a1f-4b82-4f33-b91b-8b548bce91c5"
	hashed := HashMachineID(raw)
	if !strings.HasPrefix(hashed, "node_") {
		t.Fatalf("expected prefix node_, got %s", hashed)
	}
	if len(hashed) != 5+16 { // "node_" + 16 chars
		t.Fatalf("expected length 21, got %d (%s)", len(hashed), hashed)
	}
}

func testGetNodeInfoExplicit(t *testing.T) {
	info := GetNodeInfo("my-node-01", "1.0.0")
	if info.NodeID != "my-node-01" {
		t.Fatalf("expected my-node-01, got %s", info.NodeID)
	}
	if info.OSType == "" || info.OSArch == "" || info.Hostname == "" {
		t.Fatalf("expected non-empty OS details, got %+v", info)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/node/... -v`
Expected: FAIL

- [ ] **Step 3: Implement cross-platform machine ID reading and hashing**

Read `/etc/machine-id` or `/var/lib/dbus/machine-id` on Linux. Read `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid` on Windows. Fall back to hostname. Compute SHA256, format `node_%s` (first 16 hex chars).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/node/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/node/
git commit -m "feat(node): add cross-platform hardware fingerprinting"
```

---

### Task 3: Data Contracts, Normalization & UTF-8 Cleaner

**Files:**
- Create: `internal/model/types_agy.go`
- Create: `internal/model/types_claude.go`
- Create: `internal/model/envelope.go`
- Create: `internal/model/utf8_cleaner.go`
- Test: `internal/model/envelope_test.go`
- Test: `internal/model/utf8_cleaner_test.go`

**Interfaces:**
- Produces:
  ```go
  type TelemetryEnvelope struct {
      NodeID        string                 `json:"node_id"`
      Hostname      string                 `json:"hostname"`
      OSType        string                 `json:"os_type"`
      OSArch        string                 `json:"os_arch"`
      ClientVersion string                 `json:"client_version"`
      ReportedAt    string                 `json:"reported_at"`
      Payload       map[string]interface{} `json:"payload"`
      Normalized    NormalizedData         `json:"normalized"`
  }
  type NormalizedData struct {
      ProductType       string
      SessionID         string
      PromptID          string
      AccountEmail      string
      PlanTier          string
      ToolVersion       string
      CWD               string
      ProjectDir        string
      TranscriptPath    string
      GitRepoHost       string
      GitRepoOwner      string
      GitRepoName       string
      GitBranch         string
      GitWorktree       string
      ModelID           string
      ModelDisplayName  string
      ModelEffort       string
      AgentState        string
      TotalInputTokens  int64
      TotalOutputTokens int64
      ContextWindowSize int64
      UsedPercentage    float64
      RemainingPct      float64
      Exceeds200k       bool
      TotalCostUSD      float64
      TotalDurationMS   int64
      TotalAPIDuration  int64
      CacheHitRatio     float64
      Quotas            map[string]QuotaItem
  }
  func CleanUTF8(s string) string
  func ParseAndNormalize(raw []byte, node node.NodeInfo) (*TelemetryEnvelope, *StatusLineData, error)
  ```

- [ ] **Step 1: Write failing tests for UTF-8 cleaning and normalization**

Include test cases for:
- AGY payload with nested `model.effort`
- Claude Code payload with top-level `effort.level` and `cost`
- Windows GBK-corrupted string `agentops 涓婃姤` (Review Focus #3)
- Empty or truncated JSON (Review Focus #1)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/... -v`
Expected: FAIL

- [ ] **Step 3: Implement data structures, CleanUTF8, and ParseAndNormalize**

Implement model parsing, detect `product_type`, extract and normalize fields, sanitize strings, and populate `NormalizedData` and `StatusLineData`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/model/
git commit -m "feat(model): add data contracts, normalization and utf8 cleaner"
```

---

### Task 4: Terminal ANSI Rendering & Adaptive Layout

**Files:**
- Create: `internal/render/colors.go`
- Create: `internal/render/layout.go`
- Create: `internal/render/segment_dir.go`
- Create: `internal/render/segment_model.go`
- Create: `internal/render/segment_context.go`
- Create: `internal/render/segment_tokens.go`
- Create: `internal/render/segment_quota.go`
- Create: `internal/render/segment_vcs.go`
- Create: `internal/vcs/git.go`
- Test: `internal/render/render_test.go`
- Test: `internal/vcs/git_test.go`

**Interfaces:**
- Consumes: `internal/model.StatusLineData`
- Produces:
  ```go
  func AssembleLine(data *model.StatusLineData, homeDir string) string
  func StripANSI(str string) string
  func GetGitStatus(dir string, timeout time.Duration) (branch string, dirty bool, ahead int, behind int)
  ```

- [ ] **Step 1: Write failing tests for ANSI assembling and adaptive layout**

Test standard line output formatting and test ultra-narrow width (<60 cols) triggering Level 1, 2, 3 compactions (Review Focus #5).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render/... ./internal/vcs/... -v`
Expected: FAIL

- [ ] **Step 3: Implement ANSI colors, segment formatters, git probe, and adaptive layout**

Port and adapt from `temp`: dir with `~` collapse, Git branch with lavender and dirty `*`, model with bold bright green, 8-segment context bar `\\\\\\\\` with threshold coloring, compact tokens (`1.5k`), compact quota (`2h10m`, `5d18h`), and adaptive compaction fallback.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/render/... ./internal/vcs/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/render/ internal/vcs/
git commit -m "feat(render): add ANSI rendering engine and adaptive compaction layout"
```

---

### Task 5: SQLite3 Storage Engine & Migrations

**Files:**
- Create: `internal/storage/schema.sql`
- Create: `internal/storage/db.go`
- Create: `internal/storage/store.go`
- Test: `internal/storage/store_test.go`

**Interfaces:**
- Consumes: `internal/model.TelemetryEnvelope`
- Produces:
  ```go
  type Store struct { db *sql.DB }
  func NewStore(dbPath string) (*Store, error)
  func (s *Store) Close() error
  func (s *Store) Ingest(env *model.TelemetryEnvelope) error
  func (s *Store) GetNodes() ([]NodeRecord, error)
  func (s *Store) GetSessions(nodeID string, limit int) ([]SessionRecord, error)
  func (s *Store) GetOverview() (*OverviewData, error)
  func (s *Store) GetHistory(sessionID string, limit int) ([]EventRecord, error)
  func (s *Store) GetAccountQuotas() ([]QuotaRecord, error)
  ```

- [ ] **Step 1: Write failing tests for SQLite storage with concurrent transactions**

Use `:memory:` or temp file. Test:
- Table creation and foreign key / WAL pragmas.
- Ingestion of AGY envelope and Claude Code envelope.
- `ON CONFLICT DO UPDATE` idempotency and cumulative updates.
- Burst concurrent writes (100 parallel goroutines) to verify WAL mode prevents "database is locked" (Review Focus #4).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/storage/... -v`
Expected: FAIL

- [ ] **Step 3: Implement SQLite connection pool, DDL migration, and atomic transaction UPSERTs**

Configure `modernc.org/sqlite` with `PRAGMA journal_mode = WAL`, `PRAGMA busy_timeout = 5000`, `PRAGMA synchronous = NORMAL`. In `Ingest(env)`, execute the 4-step transaction:
1. `UPSERT nodes`
2. `UPSERT sessions`
3. `INSERT telemetry_events`
4. `UPSERT account_quotas` (if email and quota present)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/storage/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/storage/
git commit -m "feat(storage): implement SQLite3 WAL storage engine and transaction upserts"
```

---

### Task 6: Client CLI Implementation & Non-Blocking Reporter

**Files:**
- Create: `internal/reporter/client.go`
- Create: `internal/reporter/client_test.go`
- Create: `cmd/statusline/main.go`
- Test: `cmd/statusline/main_test.go`

**Interfaces:**
- Consumes: `internal/config`, `internal/node`, `internal/model`, `internal/render`, `internal/reporter`
- Produces: `bin/statusline` executable

- [ ] **Step 1: Write failing tests for reporter and client CLI execution**

Test:
- Server offline: reporter exits immediately without error (Review Focus #2).
- Empty stdin: client outputs default statusline and exits with code 0 (Review Focus #1).
- Stdin with valid JSON: client writes ANSI string to stdout and triggers background report.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/reporter/... ./cmd/statusline/... -v`
Expected: FAIL

- [ ] **Step 3: Implement non-blocking reporter and client main**

In `cmd/statusline/main.go`:
1. Load configuration and node info.
2. Read stdin into buffer.
3. Call `model.ParseAndNormalize`.
4. Render ANSI line to `stdout` immediately.
5. If server endpoint is configured (and not "off"/"none"), invoke `reporter.ReportAsync`.
6. Exit 0 in <5ms.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/reporter/... ./cmd/statusline/... -v`
Expected: PASS

- [ ] **Step 5: Build client binary and test execution**

```bash
mkdir -p bin
go build -ldflags="-s -w" -o bin/statusline ./cmd/statusline
echo '{"product":"antigravity","model":{"id":"gemini-3.8-flash"}}' | ./bin/statusline
```

- [ ] **Step 6: Commit**

```bash
git add internal/reporter/ cmd/statusline/
git commit -m "feat(client): implement zero-dependency statusline client"
```

---

### Task 7: Server REST API, Web Dashboard & Refresh Scheduler

**Files:**
- Create: `cmd/statusline-server/web/index.html`
- Create: `cmd/statusline-server/web/style.css`
- Create: `cmd/statusline-server/web/app.js`
- Create: `internal/server/handlers_report.go`
- Create: `internal/server/handlers_api.go`
- Create: `internal/server/refresher.go`
- Create: `internal/server/handlers_test.go`
- Create: `cmd/statusline-server/main.go`
- Test: `cmd/statusline-server/main_test.go`

**Interfaces:**
- Consumes: `internal/storage.Store`, `internal/config.Config`
- Produces: `bin/statusline-server` executable with embedded dashboard

- [ ] **Step 1: Write failing tests for HTTP API endpoints**

Test:
- `POST /api/v1/report` validates payload and inserts into SQLite.
- `GET /api/v1/sessions` returns active sessions.
- `GET /api/v1/nodes` returns node lists.
- `GET /api/v1/overview` returns cumulative token metrics.
- `GET /` serves embedded HTML dashboard.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/... ./cmd/statusline-server/... -v`
Expected: FAIL

- [ ] **Step 3: Implement web assets, API handlers, refresher, and server entrypoint**

Port and adapt Web assets from `temp/cmd/statusline-server/web/`. Implement HTTP handlers connected to SQLite `Store`. Support command-line flags `-port`, `-db`, `-config`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/... ./cmd/statusline-server/... -v`
Expected: PASS

- [ ] **Step 5: Build server binary and test startup**

```bash
go build -ldflags="-s -w" -o bin/statusline-server ./cmd/statusline-server
./bin/statusline-server -port 18765 -db /tmp/test_statusline.db &
SERVER_PID=$!
sleep 1
curl -s http://127.0.0.1:18765/api/v1/overview
kill $SERVER_PID
```

- [ ] **Step 6: Commit**

```bash
git add cmd/statusline-server/ internal/server/
git commit -m "feat(server): implement statusline server, REST APIs and embedded dashboard"
```

---

### Task 8: End-to-End Integration, AGENTS.md & Cross-Compilation Verification

**Files:**
- Create: `AGENTS.md`
- Create: `test/e2e/e2e_test.go`

**Interfaces:**
- Verifies: Full flow from stdin -> client -> HTTP -> SQLite -> Web API query.
- Verifies: `CGO_ENABLED=0` build for Linux and Windows (`GOOS=windows go build`).

- [ ] **Step 1: Write E2E test sending both Antigravity and Claude Code sample inputs**

Verify both inputs are accepted by server and stored correctly in `telemetry_events` and `sessions` in SQLite.

- [ ] **Step 2: Run all tests in the repository**

Run: `go test ./... -v`
Expected: All packages PASS.

- [ ] **Step 3: Verify cross-platform zero-CGO compilation**

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/statusline-linux-amd64 ./cmd/statusline
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/statusline.exe ./cmd/statusline
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/statusline-server-linux-amd64 ./cmd/statusline-server
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/statusline-server.exe ./cmd/statusline-server
```
Expected: All binaries build cleanly with zero errors.

- [ ] **Step 4: Create AGENTS.md describing architecture, conventions, and operational commands**

Follow project standards per `<RULE[user_global]>`.

- [ ] **Step 5: Commit**

```bash
git add AGENTS.md test/
git commit -m "docs: add AGENTS.md and end-to-end integration tests"
```
