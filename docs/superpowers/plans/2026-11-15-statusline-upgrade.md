# statusline 客户端跨平台自我升级体系实施计划 (statusline upgrade)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `statusline` 客户端构建跨平台（Linux、macOS、Windows）原地安全自升级命令行工具，支持双源自适应分发（自建服务端优先 + GitHub 回退）、解压与 SHA256 完整性校验、Windows 锁突破原子替换、以及服务端 Web 发布包管理与一键同步。

**Architecture:** 客户端通过新增 `statusline upgrade` 独立子命令执行自升级流程，终端状态栏渲染保持零阻塞；`internal/updater` 模块负责双源探测、纯 Go 流式解压（`.tar.gz`/`.zip`）与跨平台替换（Unix `os.Rename` 与 Windows `.old` 机制）；服务端通过 `internal/server` 提供版本查询、包下载、Web 拖拽上传与 GitHub Releases 代理缓存。

**Tech Stack:** Go 1.22+ (标准库 `archive/tar`, `compress/gzip`, `archive/zip`, `crypto/sha256`, `net/http`, 零 CGO), SQLite3 (WAL), 原生 JavaScript/HTML/CSS Web 仪表盘。

**Spec:** `docs/superpowers/specs/2026-11-15-statusline-upgrade-design.md`

## Global Constraints

- **零外部依赖**：解压和散列校验必须基于 Go 标准库，严禁使用外部命令 `curl`, `tar`, `unzip`。
- **渲染零侵入**：升级逻辑完全独立，严禁在 `execute()` 终端渲染主干中执行阻塞式升级检测或下载。
- **跨平台支持**：必须兼容 Linux (amd64/arm64)、macOS (amd64/arm64) 与 Windows (amd64)。
- **纯 Go 零 CGO**：所有代码在 `CGO_ENABLED=0` 静态编译下完全可用。
- **API Key 保护**：服务端的上传与同步接口必须经过 `s.checkAPIKey(r)` 鉴权。

## Review Focus

1. **Windows 运行中覆盖失败**：Windows 施加可执行文件独占读锁；必须测试重命名为 `.old` 后写入新文件并清理旧文件流程。
2. **跨文件系统重命名 (EXDEV)**：临时文件若创建在系统 `/tmp`，而二进制位于 `/opt` 或挂载卷上，`os.Rename` 会失败；临时文件必须创建在目标二进制的同级目录。
3. **压缩包损坏或散列篡改**：网络中断或恶意劫持导致下载不完整；必须在替换二进制前强制完成 SHA256 比对，不一致则立即销毁临时文件。
4. **服务不可达回退故障**：`client.yaml` 配置的服务端挂掉或不支持分发时；必须测试超时 3 秒后无缝回退至 GitHub Releases。
5. **版本号语义比较边界**：处理带 `v` 前缀（`v0.1.5` vs `0.1.6`）、开发版后缀（`v0.1.6-dev`）及大小判断，严防降级报错或无限自我升级。

---

### Task 1: 语义化版本解析与比较引擎 (`internal/updater/version.go`)

**Files:**
- Create: `internal/updater/version.go`
- Test: `internal/updater/version_test.go`
- Modify: `cmd/statusline/main.go:20` (将 `var Version = "1.0.0"` 改为 `var Version = "dev"`)
- Modify: `cmd/statusline-server/main.go:25` (增加 `var Version = "dev"`)

**Interfaces:**
- Produces:
  - `updater.ParseSemVer(v string) (SemVer, error)`
  - `updater.CompareSemVer(v1, v2 string) int` (-1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2)
  - `updater.IsNewer(latest, current string) bool`

- [ ] **Step 1: 编写版本比较的失败测试**

```go
// internal/updater/version_test.go
package updater

import "testing"

func TestCompareSemVer(t *testing.T) {
	cases := []struct {
		v1, v2 string
		want   int
	}{
		{"v0.1.5", "v0.1.6", -1},
		{"v0.1.6", "v0.1.6", 0},
		{"0.1.6", "v0.1.6", 0},
		{"v0.2.0", "v0.1.9", 1},
		{"v1.0.0", "v0.9.9", 1},
		{"v0.1.5", "dev", -1}, // dev is treated as older than releases
		{"dev", "v0.1.5", 1},  // current dev allows upgrading to any release
	}
	for _, tc := range cases {
		got := CompareSemVer(tc.v1, tc.v2)
		if got != tc.want {
			t.Errorf("CompareSemVer(%q, %q) = %d, want %d", tc.v1, tc.v2, got, tc.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	if !IsNewer("v0.1.6", "v0.1.5") {
		t.Errorf("expected v0.1.6 to be newer than v0.1.5")
	}
	if IsNewer("v0.1.5", "v0.1.5") {
		t.Errorf("expected v0.1.5 to not be newer than v0.1.5")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/updater -run TestCompareSemVer -v`
Expected: FAIL with undefined `CompareSemVer`

- [ ] **Step 3: 实现 SemVer 解析与比较逻辑并统一客户端/服务端版本变量**

在 `internal/updater/version.go` 中实现 SemVer 解析（剔除 `v` 前缀，按 `major.minor.patch` 分解为数字比较；对 `"dev"` 进行专门兼容）；在 `cmd/statusline/main.go` 与 `cmd/statusline-server/main.go` 中定义可由 `ldflags` 注入的 `var Version = "dev"`。

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/updater -run TestCompareSemVer -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/updater/version.go internal/updater/version_test.go cmd/statusline/main.go cmd/statusline-server/main.go
git commit -m "feat(updater): add semver parsing and comparison logic"
```

---

### Task 2: 纯 Go 资产解压与 SHA256 散列校验 (`internal/updater/extract.go`)

**Files:**
- Create: `internal/updater/extract.go`
- Test: `internal/updater/extract_test.go`

**Interfaces:**
- Consumes: 标准库 `archive/tar`, `compress/gzip`, `archive/zip`, `crypto/sha256`
- Produces:
  - `updater.ExtractBinaryFromArchive(archivePath, targetBinaryName, destPath string) error`
  - `updater.VerifySHA256(filePath, expectedHash string) error`

- [ ] **Step 1: 编写解压与哈希校验的单元测试**

```go
// internal/updater/extract_test.go
package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFromTarGz(t *testing.T) {
	tmpDir := t.TempDir()
	tarPath := filepath.Join(tmpDir, "test.tar.gz")
	targetContent := "#!/bin/sh\necho statusline"

	// Create test .tar.gz
	f, err := os.Create(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{Name: "statusline_v0.1.6/statusline", Mode: 0755, Size: int64(len(targetContent))}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte(targetContent))
	_ = tw.Close()
	_ = gw.Close()
	_ = f.Close()

	destBinary := filepath.Join(tmpDir, "extracted_statusline")
	if err := ExtractBinaryFromArchive(tarPath, "statusline", destBinary); err != nil {
		t.Fatalf("failed to extract: %v", err)
	}

	content, _ := os.ReadFile(destBinary)
	if string(content) != targetContent {
		t.Errorf("extracted content mismatch, got %s", string(content))
	}
}

func TestVerifySHA256(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "hash.txt")
	_ = os.WriteFile(testFile, []byte("statusline test hash"), 0644)

	h := sha256.Sum256([]byte("statusline test hash"))
	expectedHex := hex.EncodeToString(h[:])

	if err := VerifySHA256(testFile, expectedHex); err != nil {
		t.Fatalf("expected verify pass, got %v", err)
	}
	if err := VerifySHA256(testFile, "0000000000000000"); err == nil {
		t.Fatalf("expected verify failure on mismatch hash")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/updater -run TestExtract -v`
Expected: FAIL with undefined `ExtractBinaryFromArchive`

- [ ] **Step 3: 实现 `ExtractBinaryFromArchive` 与 `VerifySHA256`**

在 `internal/updater/extract.go` 中实现：
1. 根据文件后缀（`.tar.gz` / `.tgz` 或 `.zip`）自适应分支；
2. 遍历 Header 或 Zip File List，以 `filepath.Base(name) == targetBinaryName` 为依据定位文件；
3. 将二进制写出至 `destPath`，设置权限为 `0755`；
4. 实现 `VerifySHA256` 流式读文件对比 64 位十六进制散列值。

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/updater -run "TestExtract|TestVerify" -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/updater/extract.go internal/updater/extract_test.go
git commit -m "feat(updater): implement pure-go archive extractor and sha256 verification"
```

---

### Task 3: 跨平台自替换引擎 (`internal/updater/replace.go`)

**Files:**
- Create: `internal/updater/replace.go`
- Create: `internal/updater/replace_windows.go`
- Test: `internal/updater/replace_test.go`

**Interfaces:**
- Produces:
  - `updater.GetExecutablePath() (string, error)` (解析物理真实路径)
  - `updater.ReplaceExecutable(targetExecPath, newBinaryPath string) error`
  - `updater.CleanupOldExecutables(execPath string)` (清理遗留 `.old`)

- [ ] **Step 1: 编写自替换引擎的单元测试**

```go
// internal/updater/replace_test.go
package updater

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	origPath := filepath.Join(tmpDir, "statusline")
	_ = os.WriteFile(origPath, []byte("v1"), 0755)

	newPath := filepath.Join(tmpDir, "statusline.upgrade.tmp")
	_ = os.WriteFile(newPath, []byte("v2"), 0755)

	if err := ReplaceExecutable(origPath, newPath); err != nil {
		t.Fatalf("ReplaceExecutable failed: %v", err)
	}

	content, _ := os.ReadFile(origPath)
	if string(content) != "v2" {
		t.Errorf("expected v2, got %s", string(content))
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Errorf("expected temp file to be consumed")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/updater -run TestReplaceExecutable -v`
Expected: FAIL with undefined `ReplaceExecutable`

- [ ] **Step 3: 实现跨平台原子替换与 Windows 突破锁逻辑**

- `internal/updater/replace.go`：
  - `GetExecutablePath()`：调用 `os.Executable()` 并通过 `filepath.EvalSymlinks()` 展开软链接；
  - 非 Windows 平台（`//go:build !windows`）：`os.Chmod(newBinary, 0755)` 后直接 `os.Rename(newBinary, targetExec)`；跨分区则 copy+rename；
- `internal/updater/replace_windows.go`：
  - Windows 平台（`//go:build windows`）：先将原文件重命名为 `.old`，再将新文件 `os.Rename` 到原文件；`CleanupOldExecutables()` 尝试删除 `.old`。

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/updater -run TestReplaceExecutable -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/updater/replace.go internal/updater/replace_windows.go internal/updater/replace_test.go
git commit -m "feat(updater): implement cross-platform atomic binary replacement"
```

---

### Task 4: 双源分发客户端探测与下载器 (`internal/updater/source*.go`)

**Files:**
- Create: `internal/updater/source.go`
- Create: `internal/updater/source_server.go`
- Create: `internal/updater/source_github.go`
- Test: `internal/updater/source_test.go`

**Interfaces:**
- Consumes: `config.Config`
- Produces:
  - `type ReleaseInfo struct`
  - `type Source interface`
  - `updater.ResolveSource(cfg *config.Config, forcedSource string) (Source, error)`

- [ ] **Step 1: 编写双源探测与回退机制的 Mock 测试**

```go
// internal/updater/source_test.go
package updater

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"statusline/internal/config"
)

func TestServerSourceSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/version" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"status":"ok","latest_version":"v0.1.6","upgrade_supported":true,"assets":{"linux_amd64":"/api/v1/upgrade/download?platform=linux_amd64"}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	cfg := &config.Config{
		Report: config.ReportConfig{Endpoint: ts.URL + "/api/v1/report"},
	}

	src, err := ResolveSource(cfg, "auto")
	if err != nil {
		t.Fatalf("failed to resolve source: %v", err)
	}
	if src.Name() != "server" {
		t.Errorf("expected server source, got %s", src.Name())
	}

	rel, err := src.CheckRelease(context.Background(), "latest")
	if err != nil {
		t.Fatalf("CheckRelease failed: %v", err)
	}
	if rel.Version != "v0.1.6" {
		t.Errorf("expected v0.1.6, got %s", rel.Version)
	}
}

func TestResolveSourceFallbackToGitHubWhenServerUnavailable(t *testing.T) {
	cfg := &config.Config{
		Report: config.ReportConfig{Endpoint: "http://127.0.0.1:54321/api/v1/report"}, // offline server
	}

	src, err := ResolveSource(cfg, "auto")
	if err != nil {
		t.Fatalf("failed to resolve source: %v", err)
	}
	if src.Name() != "github" {
		t.Errorf("expected fallback to github, got %s", src.Name())
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/updater -run TestServerSource -v`
Expected: FAIL with undefined `ResolveSource`

- [ ] **Step 3: 实现 `Source` 接口与 `ServerSource` / `GitHubSource`**

1. `source.go`: 定义 `ReleaseInfo`、`Source` 接口以及平台键转换（如 `runtime.GOOS + "_" + runtime.GOARCH`）；
2. `source_server.go`: 请求 `/api/v1/version`，解析返回，下载通过 `/api/v1/upgrade/download`；若设置了 API Key 自动通过 `X-API-Key` 携带；
3. `source_github.go`: 请求 GitHub Releases API (`repos/kirito-wumuxu/statusline/releases/latest`)，提取对应平台资产包与 `checksums.txt` 下载链接；
4. `ResolveSource`: 实现 3 秒连接探测与自适应降级。

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/updater -run "TestSource|TestResolve" -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/updater/source.go internal/updater/source_server.go internal/updater/source_github.go internal/updater/source_test.go
git commit -m "feat(updater): implement dual-source release resolver and downloaders"
```

---

### Task 5: 服务端发布包存储、下载与管理 API (`internal/server`)

**Files:**
- Create: `internal/server/release_scanner.go`
- Create: `internal/server/handlers_upgrade.go`
- Create: `internal/server/handlers_release_mgr.go`
- Modify: `internal/server/server.go` (注册 `/api/v1/version`, `/api/v1/upgrade/*` 路由)
- Test: `internal/server/handlers_upgrade_test.go`

**Interfaces:**
- Produces:
  - `GET /api/v1/version`
  - `GET /api/v1/upgrade/download`
  - `POST /api/v1/upgrade/upload` (API Key 保护)
  - `POST /api/v1/upgrade/sync-github` (API Key 保护)

- [ ] **Step 1: 编写服务端版本查询与下载的单元测试**

```go
// internal/server/handlers_upgrade_test.go
package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"statusline/internal/config"
	"statusline/internal/storage"
)

func TestHandlerVersionAndDownload(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "server_upgrade_test.db")
	store, _ := storage.NewStore(dbPath)
	defer store.Close()

	releasesDir := filepath.Join(tmpDir, "releases")
	_ = os.MkdirAll(releasesDir, 0755)
	dummyPkg := filepath.Join(releasesDir, "statusline_v0.1.6_linux_amd64.tar.gz")
	_ = os.WriteFile(dummyPkg, []byte("fake binary package"), 0644)

	cfg := &config.ServerConfig{
		Server:  config.ServerHTTPConfig{Host: "127.0.0.1", Port: 8628},
		Storage: config.StorageConfig{DBPath: dbPath, ReleasesDir: releasesDir},
	}

	srv := NewServer(cfg, store, nil)

	// Test GET /api/v1/version
	req := httptest.NewRequest("GET", "/api/v1/version", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Test GET /api/v1/upgrade/download
	reqDl := httptest.NewRequest("GET", "/api/v1/upgrade/download?platform=linux_amd64", nil)
	wDl := httptest.NewRecorder()
	srv.router.ServeHTTP(wDl, reqDl)

	if wDl.Code != http.StatusOK {
		t.Fatalf("expected 200 for download, got %d", wDl.Code)
	}
	if wDl.Body.String() != "fake binary package" {
		t.Errorf("download content mismatch")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/server -run TestHandlerVersionAndDownload -v`
Expected: FAIL (404 Not Found)

- [ ] **Step 3: 实现 ReleaseScanner 与升级路由处理函数**

1. `internal/config/config.go`: `StorageConfig` 增加 `ReleasesDir string`（默认 `~/.local/share/statusline/releases`）；
2. `internal/server/release_scanner.go`: 扫描 `releases_dir`，按 SemVer 维护版本索引并读取 `checksums.txt`；
3. `internal/server/handlers_upgrade.go`: 实现 `GET /api/v1/version` 与 `GET /api/v1/upgrade/download`；
4. `internal/server/handlers_release_mgr.go`: 实现 `POST /api/v1/upgrade/upload`（多文件上传）与 `POST /api/v1/upgrade/sync-github`（后台拉取 GitHub Release 资产）。

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/server -run TestHandlerVersionAndDownload -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/server/release_scanner.go internal/server/handlers_upgrade.go internal/server/handlers_release_mgr.go internal/server/handlers_upgrade_test.go internal/server/server.go internal/config/config.go
git commit -m "feat(server): add release distribution, upload, and github-sync endpoints"
```

---

### Task 6: 客户端命令行主编排与子命令接入 (`statusline upgrade`)

**Files:**
- Create: `internal/updater/updater.go`
- Modify: `cmd/statusline/main.go` (增加 `upgrade` / `update` 路由与帮助信息)
- Test: `cmd/statusline/main_test.go`

**Interfaces:**
- Consumes: `updater.ResolveSource`, `updater.ExtractBinaryFromArchive`, `updater.ReplaceExecutable`
- Produces: `updater.RunUpgrade(args []string, out io.Writer) error`

- [ ] **Step 1: 编写命令行子命令解析的单元测试**

```go
// cmd/statusline/upgrade_test.go
package main

import (
	"bytes"
	"testing"
)

func TestUpgradeCheckFlag(t *testing.T) {
	var buf bytes.Buffer
	// Running with -check should not perform download, should exit cleanly
	err := runUpgradeCommand([]string{"-check"}, &buf)
	if err != nil {
		t.Fatalf("runUpgradeCommand failed: %v", err)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./cmd/statusline -run TestUpgradeCheckFlag -v`
Expected: FAIL with undefined `runUpgradeCommand`

- [ ] **Step 3: 实现主升级编排器与 CLI 参数对接**

1. `internal/updater/updater.go`:
   - 解析命令行参数（`-check`, `-all`, `-version`, `-source`, `-y`）；
   - 获取当前运行可执行文件路径；
   - 检查同目录下是否存在 `statusline-server`（记录路径）；
   - 调用 `ResolveSource` 探测新版本；
   - 比对版本，若无需升级且非强制，友好提示并退出；
   - 若需升级且非 `-y`，向终端输出确认提示（`bufio.NewReader(os.Stdin)`）；
   - 下载压缩包至同目录临时文件（带下载进度条）；
   - 校验 SHA256；
   - 解压并原子替换 `statusline`（若 `--all` 且同目录有服务端，一并解压替换服务端）；
   - 打印升级完成信息。
2. `cmd/statusline/main.go`:
   - 补充 `case "upgrade", "update":` 路由入口；
   - 补充 `printHelp()` 中的 upgrade 使用说明。

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./cmd/statusline -run TestUpgrade -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/updater/updater.go cmd/statusline/main.go cmd/statusline/upgrade_test.go
git commit -m "feat(client): implement statusline upgrade and update CLI subcommand"
```

---

### Task 7: 服务端 Web 仪表盘发布包管理与一键同步界面 (`web/`)

**Files:**
- Modify: `cmd/statusline-server/web/index.html` (增加「📦 版本发布」管理弹窗)
- Modify: `cmd/statusline-server/web/app.js` (增加拖拽上传、包列表查看与 GitHub 一键同步)
- Modify: `cmd/statusline-server/web/style.css`

**Interfaces:**
- Consumes: `/api/v1/version`, `/api/v1/upgrade/upload`, `/api/v1/upgrade/sync-github`

- [ ] **Step 1: 编写 Web 前端 HTML 弹窗与控制逻辑**

在 `index.html` 顶部增加「📦 版本管理」按钮，弹窗中呈现：
1. 当前服务端托管的发布包列表（版本、文件大小、sha256、下载链接）；
2. 拖拽上传文件区（支持批量上传 `.tar.gz`, `.zip`, `checksums.txt`）；
3. **「↻ 从 GitHub 同步最新 Release」** 按钮，点击调用 `POST /api/v1/upgrade/sync-github`。

- [ ] **Step 2: 在 `app.js` 中实现上传与同步交互**

使用前面封装的 `requestWithAuth` 保证具备 API Key 保护，支持上传进度与状态实时回显。

- [ ] **Step 3: 验证 JS 语法与前端静态资源编译**

Run: `node -c cmd/statusline-server/web/app.js`
Expected: PASS (exit 0)

- [ ] **Step 4: 提交**

```bash
git add cmd/statusline-server/web/index.html cmd/statusline-server/web/app.js cmd/statusline-server/web/style.css
git commit -m "feat(web): add release packages management and github sync modal"
```

---

### Task 8: 构建发布脚本升级、E2E 测试与文档更新

**Files:**
- Modify: `scripts/build-release.sh` (注入 `-ldflags="-s -w -X 'main.Version=${VERSION}'"`，增加 `--upload` 参数)
- Modify: `README.md`
- Modify: `AGENTS.md`
- Test: `test/e2e/e2e_test.go`

- [ ] **Step 1: 升级 `scripts/build-release.sh`**

在构建循环中加入 `-ldflags` 注入，并添加 `--upload <url>` 与 `--api-key <key>` 支持。

- [ ] **Step 2: 编写完整的端到端自升级集成测试**

在 `test/e2e/e2e_test.go` 中验证从 Mock 服务端检测、下载、解压、校验、替换二进制的完整闭环。

- [ ] **Step 3: 运行全库测试与静态编译**

Run: `go test ./... -v`
Expected: 100% PASS

- [ ] **Step 4: 更新 README.md 与 AGENTS.md 文档**

在用户手册中详述 `statusline upgrade` 命令行用法、双源配置与服务端包管理。

- [ ] **Step 5: 提交**

```bash
git add scripts/build-release.sh README.md AGENTS.md test/e2e/e2e_test.go
git commit -m "feat: complete statusline upgrade system with e2e tests and documentation"
```
