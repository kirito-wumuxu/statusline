# Statusline

<div align="center">

[![Latest Release](https://img.shields.io/github/v/release/kirito-wumuxu/statusline?color=blue&logo=github)](https://github.com/kirito-wumuxu/statusline/releases/latest)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://golang.org)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20Windows%20%7C%20macOS-lightgrey)](#-下载与安装)
[![Zero CGO](https://img.shields.io/badge/CGO-Disabled%20(0)-brightgreen)](#-技术亮点与架构设计)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**面向 AI 命令行助手（Google Antigravity 与 Anthropic Claude Code）的高性能跨平台终端状态栏插件与集中遥测监控系统。**

[快速开始](#-快速开始) • [下载安装](#-下载与安装) • [配置接入](#-配置与接入) • [架构设计](#-技术亮点与架构设计) • [本地开发](#-本地编译与测试)

</div>

---

## 🌟 效果展示与终端美学

`statusline` 在标准输入中读取 CLI 传入的会话实时 JSON，在 **<2ms** 内向终端 `stdout` 输出高辨识度的 ANSI 彩色状态栏：

```text
[~] (main*) <kirito_wu@163.com> [Gemini 3.8 Flash (High)] \\\\\\\\ 22% [1M] | ↑1.5k ↓230 | Gemini: 93%(2h10m) 89%(5d18h) | Claude: 100%(4h59m) 99%(5d18h)
```

### 状态栏分段解析

| 分段图标 / 示例 | 含义与设计细节 |
| :--- | :--- |
| **`[~]` / `[/workspace]`** | **工作区目录**：天蓝色（Sky Blue `\033[38;5;39m`），自动将家目录折叠为 `~` |
| **`(main*)`** | **Git 仓库探针**：浅薰衣草紫（Lavender `\033[38;5;141m`），脏工作区带黄色星号标记（`*`），20ms 熔断保护 |
| **`<user@email.com>`** | **账号标识**：暖琥珀金（Amber `\033[38;5;214m`），有账号时呈现，无账号时自动跳过不占位 |
| **`[Gemini 3.8 Flash (High)]`** | **模型与思考等级**：粗体亮绿，自动提取并归一化 Effort 等级（Low / Medium / High） |
| **`\\\\\\\\ 22% [1M]`** | **上下文容量进度条**：8 段斜杠条。`<70%` 绿、`70-89%` 黄、`≥90%` 红；未用进度为暗灰，白字标注总窗口 |
| **`↑1.5k ↓230`** | **会话 Token 吞吐**：明青色 `↑` 输入 Token，品红色 `↓` 输出 Token，智能紧凑单位（`k`, `M`） |
| **`Gemini: 93%(2h10m)`** | **多阶配额与冷却倒计时**：明青色 Gemini 标签，支持 5 小时 / 周级余量百分比与重置倒计时 |
| **`Claude: 100%(4h59m)`** | **Claude 配额指示器**：暖橙色 Claude 标签，支持多阶使用率监控 |

> **窄屏自适应**：依据终端宽度（`TerminalWidth`）自动触发 Level 1 ~ 3 降级缩减策略，截断或隐藏次要段，确保在分屏、小窗口下绝不发生换行错位。

---

## 🚀 核心特性

- **⚡ 极致响应性能**：标准输入解析 + 终端 ANSI 排版输出全程低于 2ms，交互体验零延迟。
- **🛡️ 纯 Go 零 CGO 静态二进制**：依赖 `modernc.org/sqlite`，在 Windows、Linux 与 macOS 上均可单一二进制独立运行，无需安装 gcc、cgo 或任何运行时环境。
- **📡 严格超时异步上报**：终端状态输出完成后，上报请求受 100ms 严格超时控制，即使服务端宕机或网络中断也静默忽略，绝不阻塞用户终端。
- **📊 嵌入式 SQLite3 WAL 持久化**：服务端采用 WAL 高并发模式，4 张核心表（节点、会话快照、流水明细、账号配额）单事务 UPSERT 写入，支持高频写入无锁冲突。
- **💻 嵌入式深色实时 Web 仪表盘**：通过 Go 1.16+ `//go:embed` 将前端（HTML/CSS/JS）静态资产内嵌进二进制，单文件直接运行并提供现代深色 Web 监控看板。
- **🧹 编码清洗器**：自动修复 Windows 终端常见的 GBK/UTF-8 错位乱码（如 `agentops 涓婃姤` 自动清理），确保数据库存储安全。
- **🔒 跨平台机器指纹脱敏**：采集 Linux `/etc/machine-id` 或 Windows 注册表 `MachineGuid` 并进行 SHA256 脱敏哈希，保障多节点遥测隐私。

---

## 📦 下载与安装

请前往 [GitHub Releases](https://github.com/kirito-wumuxu/statusline/releases/latest) 下载对应平台架构的最新预编译包：

| 平台与架构 | 文件名称 | 格式 |
| :--- | :--- | :--- |
| **Linux (x86_64 / amd64)** | `statusline_v0.1.2_linux_amd64.tar.gz` | `.tar.gz` |
| **Linux (ARM64 / aarch64)** | `statusline_v0.1.2_linux_arm64.tar.gz` | `.tar.gz` |
| **Windows (x86_64 / amd64)** | `statusline_v0.1.2_windows_amd64.zip` | `.zip` |
| **macOS (Intel / amd64)** | `statusline_v0.1.2_darwin_amd64.tar.gz` | `.tar.gz` |
| **macOS (Apple Silicon / M系列)** | `statusline_v0.1.2_darwin_arm64.tar.gz` | `.tar.gz` |

### Linux / macOS 一键解压与放置

```bash
# 解压到本地 bin 目录 (以 Linux amd64 为例)
tar -xzf statusline_v0.1.2_linux_amd64.tar.gz
chmod +x statusline statusline-server
mv statusline statusline-server ~/.local/bin/
```

### Windows 安装

解压 `statusline_v0.1.2_windows_amd64.zip` 后，将 `statusline.exe` 与 `statusline-server.exe` 放置于系统 `PATH` 包含的目录（如 `C:\Users\<用户名>\.local\bin`）即可。

---

## 🔧 配置与接入

### 1. 一键自动配置 (`statusline init`)

推荐使用一键初始化命令完成 Antigravity CLI 与 Claude Code 的全自动配置：

```bash
statusline init
```

该命令具备高度智能与免侵入设计：
1. **原地绝对路径解析**：自动解析当前运行可执行文件的真实绝对路径（无需移动或复制二进制文件，放置在任意目录均可直接初始化）；
2. **多工具并发配置**：
   - **Antigravity CLI**：自动定位并安全配置 `~/.gemini/antigravity-cli/settings.json`（或 `~/.gemini/settings.json`），写入 `"statusLine"` 块，无损保留原有其他配置；
   - **Claude Code**：自动配置 `~/.claude/settings.json`（及当前工作区的 `.claude/settings.json`），写入标准 `"statusLine": {"type": "command", "command": "..."}` 配置；
3. 执行完成后，重启 Antigravity CLI 或 Claude Code 即可即时呈现状态栏。

> **高级选项**：
> - `statusline init -target claude`：仅配置 Claude Code；
> - `statusline init -target agy`：仅配置 Antigravity CLI；
> - `statusline init -settings /path/to/settings.json`：指定自定义配置文件路径；
> - `statusline init -bin /custom/bin/statusline`：指定自定义二进制路径。

### 2. 手动接入配置说明

若需手动维护配置文件：
- **Antigravity CLI**（`~/.gemini/antigravity-cli/settings.json`）：
  ```json
  {
    "statusLine": {
      "type": "command",
      "command": "/absolute/path/to/statusline",
      "enabled": true
    }
  }
  ```
- **Claude Code**（`~/.claude/settings.json`）：
  ```json
  {
    "statusLine": {
      "type": "command",
      "command": "/absolute/path/to/statusline"
    }
  }
  ```
  *(也可使用环境变量 `export CLAUDE_STATUSLINE_COMMAND="/absolute/path/to/statusline"`)*

### 3. 环境变量与无服务器模式 (Pure Local)

- **自定义服务端上报地址**：
  ```bash
  export STATUSLINE_SERVER_URL="http://127.0.0.1:8628/api/v1/report"
  ```
- **纯本地离线模式（完全停用遥测网络上报，仅作为本地终端状态栏渲染）**：
  ```bash
  export STATUSLINE_SERVER_URL="off"
  ```

### 4. 客户端配置与模块化排序 (`client.yaml`)

客户端配置默认自动从**二进制所在目录**下检索 `client.yaml`，亦支持当前工作目录 `./client.yaml` 或用户全局目录 `~/.config/statusline/client.yaml`（可通过环境变量 `STATUSLINE_CLIENT_CONFIG` 明确指定）：

```yaml
# 遥测数据上报
report:
  endpoint: "http://127.0.0.1:8628/api/v1/report" # 设为 "off" 可停用上报，纯本地离线渲染
  timeout_ms: 100                                  # 客户端严格上报超时，绝不阻塞终端
  api_key: ""                                      # 可选的上报认证密钥

# 节点指纹与元数据
node:
  id: ""           # 留空自动提取硬件指纹 SHA256 (node_...)
  custom_tags:
    env: "local"

# 终端状态栏渲染设置
display:
  enable_stdout: true
  # 状态栏模块化显示列表：可自由调换顺序，不需要的模块直接注释即可
  segments:
    - dir         # 工作区目录 [~] 或 [/path]
    - vcs         # Git 分支与修改标记 (main*)
    - account     # 账号信息 <user@email.com>（无账号时自动跳过）
    - model       # 模型名称与思考等级 [Gemini 3.8 Flash (High)]
    - context     # 上下文进度条 \\\\\\\\ 22% [1M]
    - tokens      # Token 吞吐 ↑1.5k ↓230
    - quota       # 配额余量与冷却倒计时
```

也可通过环境变量快速覆盖展示顺序：
```bash
export STATUSLINE_SEGMENTS="account,model,context,tokens,quota"
```

---

## 🖥️ 启动监控服务端 (`statusline-server`)

服务端负责接收多节点客户端上报、写入 SQLite3 数据库并提供实时 Web 监控大盘。

### 服务端配置 (`server.yaml`)
服务端配置默认自动从**二进制所在目录**下检索 `server.yaml`，亦支持当前工作目录 `./server.yaml` 或用户目录 `~/.config/statusline/server.yaml`（可通过 `-config` 命令行参数明确指定）：

```yaml
# HTTP 监控服务监听与认证
server:
  host: "0.0.0.0"                                  # 监听网卡地址
  port: 8628                                       # 监听端口
  api_key: ""                                      # 可选的服务端接入认证密钥

# 持久化存储引擎 (纯 Go 零 CGO 嵌入式 SQLite3 WAL 模式)
storage:
  db_path: "~/.local/share/statusline/statusline.db" # SQLite 数据库持久化路径
  wal_mode: true                                   # 启用 WAL 高并发写入模式
```

### 启动命令
```bash
# 使用默认配置启动 (优先读取二进制所在目录/当前目录的 server.yaml)
statusline-server

# 指定配置文件、端口或数据库路径
statusline-server -config server.yaml -port 8628 -db ~/.local/share/statusline/statusline.db
```

### Web 实时监控仪表盘

启动后使用浏览器打开 `http://127.0.0.1:8628`：

- **会话大盘**：实时展示当前活跃会话数、历史总累计 Token（输入/输出）、总花费（USD）与各节点活跃状态。
- **趋势分析**：SVG 实时动态渲染近 1 小时内 Token 与请求吞吐曲线。
- **会话快照列表**：查看每一个 `session_id` 的工作目录、Git 仓库分支、最新使用的模型、Context 占用比例及最后活跃时间。
- **账号配额监控池**：聚合展示多账号 Gemini 与 Claude 的剩余配额与冷却重置倒计时。

---

## 🏗️ 技术亮点与架构设计

### 系统数据流

```text
[CLI Stdin: AGY / Claude Code 实时上下文 JSON]
               │
               ▼
   ┌──────────────────────────────────────────────┐
   │         statusline (客户端终端插件)           │
   │  1. 字符编码清洗 (CleanUTF8 修复 GBK 乱码)    │
   │  2. 提取 Git 分支与状态 (20ms 超时探针)       │
   │  3. 终端宽度降级计算 (Level 1~3 排版)         │
   │  4. <2ms 写入 stdout 终端渲染                │
   └──────────────────────┬───────────────────────┘
                          │ 异步非阻塞上报 (100ms 严格超时)
                          ▼
   ┌──────────────────────────────────────────────┐
   │         statusline-server (服务端守护进程)     │
   │  1. POST /api/v1/report 接收标准信封          │
   │  2. 硬件指纹脱敏 (node_id SHA256 哈希)       │
   │  3. SQLite3 WAL 单事务 4 表 UPSERT 写入      │
   │  4. //go:embed 托管深色 Web 仪表盘           │
   └──────────────────────────────────────────────┘
```

### 数据库设计 (SQLite3 WAL)

- **`nodes`**：管理机器指纹（如 `node_8f9d2a1b4c3e5f6a`）、操作系统、架构与最后上报时间。
- **`sessions`**：会话维度快照表，存储工作目录、Git 仓库元数据、最新累计 Token、费用与活跃时间。通过 `ON CONFLICT(session_id) DO UPDATE` 幂等刷新。
- **`telemetry_events`**：瞬时流水明细表，记录单次问答触发的 Token 消耗、Context Window 使用率、缓存命中率及原始 JSON 备份。
- **`account_quotas`**：多账号配额实时状态表，聚合记录各账号的 Gemini / Claude 配额余量与重置时间。

---

## 🛠️ 本地编译与测试

本项目采用标准 Go Modules 管理（Go 1.22+）。

### 1. 运行测试套件

```bash
# 运行全部单元测试与端到端集成测试
go test ./... -v
```

### 2. 编译本地二进制 (零 CGO)

```bash
# Linux
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/statusline ./cmd/statusline
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/statusline-server ./cmd/statusline-server

# Windows
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/statusline.exe ./cmd/statusline
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/statusline-server.exe ./cmd/statusline-server

# macOS (Apple Silicon)
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o bin/statusline ./cmd/statusline
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o bin/statusline-server ./cmd/statusline-server
```

### 3. 一键打包全平台 Release

```bash
./scripts/build-release.sh v0.1.2
```
该脚本将自动在 `dist/` 目录下生成各平台压缩包并输出 `checksums.txt`。

---

## 📄 授权协议

本项目采用 [MIT License](LICENSE) 开源协议。
