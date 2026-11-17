# statusline-session (sls) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an independent high-performance TUI binary (`statusline-session` / `sls`) based on Bubble Tea & Lip Gloss to manage, fuzzy-filter, preview, and instantly resume AI CLI sessions (Google Antigravity & Claude Code), strictly filtered by the current machine's hardware fingerprint (Node ID).

**Architecture:** Dual-pane responsive TUI (Scheme B): left pane displays session card list sorted by recency with live fuzzy search; right pane displays metadata card and last turn dialogue preview (User Prompt & Model Response). Resumes sessions by gracefully releasing the terminal and executing the CLI in the target working directory, or attaching via tmux/clipboard.

**Tech Stack:** Go 1.22+, `github.com/charmbracelet/bubbletea` (v1.3.4), `github.com/charmbracelet/lipgloss` (v1.0.0), `github.com/charmbracelet/bubbles` (v0.20.0), `modernc.org/sqlite` (zero CGO).

**Spec:** `docs/superpowers/specs/2026-11-18-statusline-session-tui-design.md`

## Global Constraints

- Standalone executable in `cmd/statusline-session/`, compiled as `bin/statusline-session` (Windows: `statusline-session.exe`).
- Zero CGO dependency (`CGO_ENABLED=0`), cross-platform support (Linux amd64/arm64, macOS amd64/arm64, Windows amd64).
- Must default to filtering sessions by the local device's Node ID (`internal/node.GetNodeInfo`), with `--all-nodes` / `a` toggle.
- Non-blocking and responsive: TUI cold start under 15ms; dialogue transcript preview loaded asynchronously.
- Terminal safety: Must cleanly release the alternate screen buffer (`tea.ExitAltScreen`) before launching sub-process commands.

## Review Focus

- Node ID Isolation: sessions from other machines in central DB must NEVER show up in default local view.
- Non-existent CWD handling: if a session's directory was deleted, pressing Enter must not crash, but display a clear warning.
- Empty Transcript / Incomplete JSON handling: corrupt or missing `transcript_path` must gracefully display "(暂无转录预览)" without throwing errors.
- Narrow terminal adaptation: when terminal width < 80 columns, layout must gracefully compact instead of breaking.
- Terminal raw mode restoration: pressing Ctrl+C or quitting must leave the user's shell in a healthy state.

---

### Task 1: Session Data Service & Local Node Filtering

**Files:**
- Create: `internal/session/service.go`
- Create: `internal/session/preview.go`
- Test: `internal/session/service_test.go`
- Test: `internal/session/preview_test.go`

**Interfaces:**
- Consumes:
  - `storage.Store`: `GetSessions() ([]storage.SessionRecord, error)`
  - `config.Config`: `Report.Endpoint`, `Storage.DBPath`
  - `node.GetNodeInfo(customID, version string) node.NodeInfo`
- Produces:
  - `type SessionItem struct { Record storage.SessionRecord; IsLocalNode bool; HasValidCWD bool; RelativeTime string }`
  - `type SessionService interface { ListSessions(ctx context.Context, onlyLocalNode bool) ([]SessionItem, error) }`
  - `func LoadTranscriptPreview(transcriptPath string, maxLines int) (userPrompt, assistantReply string, err error)`

- [ ] **Step 1: Write failing tests for SessionService and Preview**
  - Verify `ListSessions` filters records by `node_id == CurrentNodeID` when `onlyLocalNode=true`.
  - Verify `ListSessions` includes foreign nodes when `onlyLocalNode=false`.
  - Verify `LoadTranscriptPreview` parses JSONL tail into User Prompt & Assistant response.

- [ ] **Step 2: Run test to verify it fails**
  - Run: `go test -v ./internal/session/...`
  - Expected: FAIL with package not found.

- [ ] **Step 3: Implement SessionService and TranscriptPreview**
  - Implement `SessionService` supporting SQLite connection and remote HTTP fallback.
  - Implement `LoadTranscriptPreview` reading backwards up to 2MB to extract the last User and Assistant turns.
  - Implement relative time formatter (`3m ago`, `2h ago`, `昨天`, `3d ago`).

- [ ] **Step 4: Run tests to verify they pass**
  - Run: `go test -v ./internal/session/...`
  - Expected: PASS.

- [ ] **Step 5: Commit changes**
  - `git add internal/session/ && git commit -m "feat(session): implement session data service with node_id filter and transcript preview"`

---

### Task 2: Launcher & Session Recovery Dispatcher

**Files:**
- Create: `internal/session/launcher.go`
- Test: `internal/session/launcher_test.go`

**Interfaces:**
- Consumes:
  - `SessionItem` from `internal/session/service.go`
- Produces:
  - `func BuildResumeCommand(item SessionItem) (cmdName string, cmdArgs []string, workingDir string, fullShellCmd string)`
  - `func CheckWorkingDir(cwd string) bool`
  - `func CopyToClipboard(text string) error`
  - `func LaunchInTmux(fullCmd, sessionName, cwd string) error`

- [ ] **Step 1: Write failing test for Launcher**
  - Test Claude Code resume command generation (`claude --resume <id>` vs `claude`).
  - Test Antigravity command generation (`agy`).
  - Test shell-escaped command string generation (`cd '/path/with spaces' && claude --resume sess_123`).
  - Test directory existence validator.

- [ ] **Step 2: Run test to verify it fails**
  - Run: `go test -v ./internal/session -run TestBuildResumeCommand`
  - Expected: FAIL.

- [ ] **Step 3: Implement Launcher logic**
  - Implement `BuildResumeCommand` with proper platform quoting.
  - Implement `CheckWorkingDir` with `os.Stat`.
  - Implement `CopyToClipboard` via OSC 52 / platform fallback (`pbcopy`, `xclip`, `wl-copy`, `clip.exe`).
  - Implement `LaunchInTmux` issuing `tmux new-window -c <cwd> -n <name> <cmd>`.

- [ ] **Step 4: Run tests to verify they pass**
  - Run: `go test -v ./internal/session -run TestBuildResumeCommand`
  - Expected: PASS.

- [ ] **Step 5: Commit changes**
  - `git add internal/session/launcher* && git commit -m "feat(session): implement session launcher and resume command builder"`

---

### Task 3: Bubble Tea UI Components & Lip Gloss Styles

**Files:**
- Create: `cmd/statusline-session/ui/styles.go`
- Create: `cmd/statusline-session/ui/list_pane.go`
- Create: `cmd/statusline-session/ui/detail_pane.go`
- Create: `cmd/statusline-session/ui/preview_pane.go`
- Test: `cmd/statusline-session/ui/ui_test.go`

**Interfaces:**
- Consumes:
  - `SessionItem` from `internal/session`
  - `lipgloss` styling primitives
- Produces:
  - `RenderSessionList(items []SessionItem, cursor int, width, height int) string`
  - `RenderDetailPane(item *SessionItem, width int) string`
  - `RenderPreviewPane(userPrompt, assistantReply string, width, height int) string`
  - `RenderHelpBar(width int, searchActive, onlyLocalNode bool) string`

- [ ] **Step 1: Write UI component tests**
  - Verify list rendering highlights the active cursor item with `>`.
  - Verify detail pane shows CWD warning if directory is missing.
  - Verify badges format correctly (`[AGY]` cyan, `[Claude]` amber).

- [ ] **Step 2: Run test to verify it fails**
  - Run: `go test -v ./cmd/statusline-session/ui`
  - Expected: FAIL.

- [ ] **Step 3: Implement Lip Gloss styles & view panes**
  - Implement `styles.go`: colors consistent with `statusline` ANSI theme.
  - Implement `list_pane.go`: responsive session card items.
  - Implement `detail_pane.go`: formatted cards for metadata.
  - Implement `preview_pane.go`: viewport wrapping prompt & completion text.
  - Implement `help_bar`: bottom guide with dynamic keybinding reminders.

- [ ] **Step 4: Run tests to verify they pass**
  - Run: `go test -v ./cmd/statusline-session/ui`
  - Expected: PASS.

- [ ] **Step 5: Commit changes**
  - `git add cmd/statusline-session/ui/ && git commit -m "feat(tui): implement Lip Gloss styles and dual-pane views"`

---

### Task 4: Interactive TUI Application State Machine

**Files:**
- Create: `cmd/statusline-session/app.go`
- Test: `cmd/statusline-session/app_test.go`

**Interfaces:**
- Consumes:
  - `internal/session/service.go`
  - `internal/session/launcher.go`
  - `cmd/statusline-session/ui`
  - `github.com/charmbracelet/bubbletea`
- Produces:
  - `type AppModel struct { ... }`
  - `func NewAppModel(service session.SessionService, initialOnlyLocal bool) *AppModel`
  - `func (m *AppModel) Init() tea.Cmd`
  - `func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd)`
  - `func (m *AppModel) View() string`
  - `type LaunchAction struct { Item session.SessionItem; Mode string }`

- [ ] **Step 1: Write state machine tests**
  - Test arrow up/down updates selected index.
  - Test `/` toggles search mode and typing filters the list.
  - Test `a` toggles `onlyLocalNode` filter and triggers reload.
  - Test `Enter` emits launch command with selected `SessionItem`.

- [ ] **Step 2: Run test to verify it fails**
  - Run: `go test -v ./cmd/statusline-session -run TestAppModel`
  - Expected: FAIL.

- [ ] **Step 3: Implement Bubble Tea Model, Update, and View**
  - Implement fuzzy matching filter on project, branch, tool, and session ID.
  - Connect textinput for search queries.
  - Handle window resizing dynamically (`tea.WindowSizeMsg`).
  - Wire async transcript preview loading on cursor change (debounced 50ms).
  - Handle `Enter`, `t`, `c`, `r`, `a`, `q` keys.

- [ ] **Step 4: Run tests to verify they pass**
  - Run: `go test -v ./cmd/statusline-session -run TestAppModel`
  - Expected: PASS.

- [ ] **Step 5: Commit changes**
  - `git add cmd/statusline-session/app* && git commit -m "feat(tui): implement Bubble Tea interactive app state machine"`

---

### Task 5: CLI Entrypoint, Build Integration & E2E Validation

**Files:**
- Create: `cmd/statusline-session/main.go`
- Test: `cmd/statusline-session/main_test.go`
- Modify: `scripts/build-release.sh`
- Modify: `AGENTS.md`
- Test: `test/e2e/session_tui_test.go`

**Interfaces:**
- Produces:
  - Standalone binary executable `bin/statusline-session`
  - Full release packages in `dist/` containing `statusline-session`

- [ ] **Step 1: Implement main.go CLI router and flags**
  - Flags: `--all-nodes` / `-a`, `--db <path>`, `--config <path>`, `--version` / `-v`, `--help` / `-h`.
  - Handle post-TUI execution: if `Enter` was pressed, perform terminal takeover `exec` into `CWD`.

- [ ] **Step 2: Write CLI unit and integration tests**
  - Test `--help` and `--version` output.
  - Test dry run / mock run of `main.go`.

- [ ] **Step 3: Update build script and documentation**
  - In `scripts/build-release.sh`, add `statusline-session` compilation for all 5 platforms.
  - In `AGENTS.md`, document `statusline-session` usage, flags, and shortcuts.

- [ ] **Step 4: Verify full test suite across entire repository**
  - Run: `go test -v -count=1 ./...`
  - Expected: 100% PASS.

- [ ] **Step 5: Compile locally and test manual invocation**
  - Build `bin/statusline-session`.
  - Run `./bin/statusline-session --help`.
  - Run `./bin/statusline-session` in dry-run/mock mode.

- [ ] **Step 6: Commit and Push**
  - `git add cmd/statusline-session/ scripts/build-release.sh AGENTS.md && git commit -m "feat: complete statusline-session TUI binary and build integration"`
  - `git push origin main`
