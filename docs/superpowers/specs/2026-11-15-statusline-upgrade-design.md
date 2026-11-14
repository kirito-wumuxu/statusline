# statusline 客户端跨平台自我升级体系设计规范 (statusline upgrade)

- **Date**: 2026-11-15
- **Target Projects**: `statusline` (CLI Client) & `statusline-server` (Telemetry & Distribution Server)
- **Language**: Go 1.22+ (Static Compilation, Zero CGO, Pure Standard Library)
- **Design Path**: Architectural
- **Status**: Spec Ready for User Review

---

## 1. 目标与设计原则 (Objectives & Principles)

### 背景与痛点
当前 `statusline` 与 `statusline-server` 已发布至 `v0.1.5`。目前每次版本迭代，用户均需手动前往 GitHub Releases 下载对应操作系统与架构的压缩包，解压后手动覆盖本地二进制并重新赋予权限。在多节点或远程服务器环境下，手动更新操作繁琐且易遗漏。

### 设计原则
1. **独立命令，终端渲染零侵入**：
   - 升级操作由用户显式执行 `statusline upgrade` 触发；
   - 终端状态栏主渲染流程（`stdin -> stdout`，<2ms）保持绝对极速纯粹，不夹带任何阻塞式网络下载操作。
2. **纯 Go 零外部依赖 (Zero External Dependencies)**：
   - 资产包下载、解压（`.tar.gz` 与 `.zip`）、SHA256 完整性校验均基于 Go 标准库（`archive/tar`, `compress/gzip`, `archive/zip`, `crypto/sha256`）实现；
   - 绝不依赖宿主机的 `curl`, `wget`, `tar`, `unzip` 等系统外部命令。
3. **双源自适应分发 (Dual-Source Strategy)**：
   - **服务端优先**：若 `client.yaml` 配置了可访问的 `statusline-server`，优先尝试向自建服务端拉取升级包元数据与二进制（国内局域网/内网环境下载极速，不受 GitHub API 速率限制和网络抖动影响）；
   - **GitHub 自动回退**：若未配置服务端或服务端未托管升级资产包，自动无缝回退至 GitHub Releases 官方源。
4. **跨平台原子替换 (Cross-Platform Self-Replacement)**：
   - **Unix (Linux & macOS)**：利用 Unix 系统的 Inode 机制，通过同目录临时文件赋予 `0755` 权限后 `os.Rename` 原子覆盖当前运行文件；
   - **Windows**：突破正在运行 `.exe` 的锁定限制，通过 `os.Rename(execPath, execPath + ".old")` 腾出原路径，再写入新二进制，并在下一次启动时自动回收 `.old` 残留。
5. **清晰的职责边界与可控性**：
   - 默认仅升级当前执行的 `statusline` 客户端；
   - 若同目录下检测到存在 `statusline-server`，主动提示并支持通过 `--all` 参数一并原子升级服务端。

---

## 2. 命令行接口与交互规范 (CLI Specification)

### 2.1 命令语法与参数

```bash
statusline upgrade [options]
# 别名支持
statusline update [options]
```

| 参数选项 | 类型 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| `-check` | bool | `false` | 仅检查最新版本与变更说明，不执行下载与安装 |
| `-all` | bool | `false` | 若同目录下存在 `statusline-server`，同时将其升级至同版本 |
| `-version <tag>` | string | `"latest"` | 指定升级或回滚的目标版本（如 `v0.1.5`） |
| `-source <auto\|server\|github>` | string | `"auto"` | 强制指定升级下载源，默认自动双源探测 |
| `-y` / `--yes` | bool | `false` | 非交互式模式，跳过确认提示直接执行下载与替换 |

### 2.2 终端交互效果

#### 场景 1：发现新版本并确认升级
```text
$ statusline upgrade
[1/4] 检查当前环境...
      当前组件: statusline v0.1.5 (linux/amd64)
      文件路径: /home/user/.local/bin/statusline
[2/4] 正在探测可用更新源...
      ✓ 已连接 statusline-server (http://127.0.0.1:8628)，发现新版本 v0.1.6
[3/4] 发现新版本: v0.1.5 -> v0.1.6
      发布说明: 支持客户端自动升级体系，优化 Web 密钥认证提示
      下载资产: statusline_v0.1.6_linux_amd64.tar.gz (7.2 MB)
? 是否确认升级? [Y/n] y
[4/4] 正在下载升级包...
      下载进度: [====================================] 100% (7.2 MB / 7.2 MB)
      校验 SHA256 散列值 ... 匹配成功 (6f3d8a...b1)
      安全解压并替换本地程序 ... 完成
✓ 升级成功！当前运行版本: statusline version v0.1.6
```

#### 场景 2：当前已是最新版本
```text
$ statusline upgrade -check
[1/2] 当前组件: statusline v0.1.6 (linux/amd64)
[2/2] 正在检查更新...
✓ 当前已是最新版本 (v0.1.6)，无需升级。
```

#### 场景 3：检测到同目录存在服务端并使用 `-all`
```text
$ statusline upgrade --all -y
[1/4] 检查当前环境...
      客户端: statusline v0.1.5 (/opt/statusline/statusline)
      服务端: statusline-server v0.1.5 (/opt/statusline/statusline-server)
[2/4] 探测更新源: GitHub Releases (latest: v0.1.6)
[3/4] 正在下载 statusline_v0.1.6_linux_amd64.tar.gz ... 100%
      校验 SHA256 ... 匹配成功
[4/4] 原子替换可执行文件:
      ✓ statusline 已更新为 v0.1.6
      ✓ statusline-server 已更新为 v0.1.6
✓ 升级完成！
```

---

## 3. 系统架构与模块分工

```text
statusline/
├── cmd/
│   ├── statusline/
│   │   └── main.go                 # 路由增加 upgrade / update 子命令入口
│   └── statusline-server/
│       └── main.go                 # 增加 Version 变量与 --version 支持
├── internal/
│   ├── updater/                    # 核心自我升级子系统 (新增)
│   │   ├── updater.go              # Upgrade 主编排引擎 (Check -> Download -> Replace)
│   │   ├── source.go               # 更新源抽象接口 (Source)
│   │   ├── source_server.go        # statusline-server 本地/代理更新源
│   │   ├── source_github.go        # GitHub Releases 官方源
│   │   ├── extract.go              # 纯 Go 内存/流式解压器 (.tar.gz / .zip)
│   │   ├── replace.go              # 跨平台自替换引擎 (Unix atomic rename)
│   │   ├── replace_windows.go      # Windows .old 重命名突破锁机制
│   │   └── updater_test.go         # 单元测试与 Mock 下载集成测试
│   └── server/
│       ├── handlers_version.go     # 服务端 GET /api/v1/version 接口 (新增)
│       └── handlers_upgrade.go     # 服务端 GET /api/v1/upgrade/download 接口 (新增)
└── scripts/
    └── build-release.sh            # 完善 -ldflags="-X main.Version=${VERSION}"
```

---

## 4. 关键技术方案详述

### 4.1 更新源抽象与双源自适应算法

定义标准更新源接口 `Source`：

```go
type ReleaseInfo struct {
    Version      string            `json:"version"`       // e.g. "v0.1.6"
    TagName      string            `json:"tag_name"`
    ReleaseNotes string            `json:"release_notes"` // Markdown 或纯文本
    PublishedAt  string            `json:"published_at"`
    Assets       map[string]string `json:"assets"`        // key: "linux_amd64", value: download_url
    Checksums    map[string]string `json:"checksums"`     // key: filename, value: sha256_hex
}

type Source interface {
    Name() string
    CheckRelease(ctx context.Context, targetVersion string) (*ReleaseInfo, error)
    Download(ctx context.Context, assetURL string, destFile io.Writer, progressFn func(downloaded, total int64)) error
}
```

#### 双源探测逻辑 (`ResolveSource`)
1. 检查当前是否指定了 `-source server` 或 `-source github`，若指定则强制单一源；
2. 若为默认 `auto`：
   - 提取 `client.yaml` 中的 `report.endpoint`。若配置有效且未被禁用（不为 `off`/`none`），尝试向服务端请求 `GET /api/v1/version`（设置 3 秒连接超时）；
   - 若服务端响应成功且包含当前平台资产信息，选用 `ServerSource`；
   - 若连接失败、超时或服务端返回不支持，自动回退选用 `GitHubSource`。

### 4.2 服务端发布分发支持 (`statusline-server`)

服务端支持轻量级升级包分发与代理：
1. **路由定义**：
   - `GET /api/v1/version`：返回服务端自身运行版本及当前服务端已有的 Release 信息；
   - `GET /api/v1/upgrade/download?platform={os}_{arch}`：下载平台专属压缩包。
2. **托管机制**：
   - 服务端启动时自动检查其运行目录下的 `dist/` 文件夹或同目录下是否存在 `statusline_*_{os}_{arch}.(tar.gz|zip)`；
   - 若存在，自动解析出元数据并开放本地极速直连下载；
   - 若不存在本地分发包，接口明确返回 `{"status":"ok", "upgrade_supported": false}`，触发客户端无损回退至 GitHub。

### 4.3 纯 Go 解压与校验流 (`extract.go`)

在下载完成后，不解压整个压缩包到磁盘，而是直接流式定位目标二进制：
1. **`.tar.gz` 解压**：
   - 结合 `gzip.NewReader` 与 `tar.NewReader`；
   - 遍历 Header，精准匹配目标文件名（如 `statusline` 或 `statusline-server`）；
   - 使用 `io.Copy` 将目标文件内容解出至临时文件（如 `.statusline.upgrade.tmp`）。
2. **`.zip` 解压**：
   - 使用 `zip.NewReader` 检索对应的 `File` 项并流式读出。
3. **SHA256 校验**：
   - 下载过程中使用 `io.MultiWriter(tempFile, hasher)` 实时计算下载包 SHA256 哈希；
   - 对比 Release 中提供的 `checksums.txt`，若哈希不符立即终止并报警，严防网络劫持与下载损坏。

### 4.4 跨平台安全原子替换机制

#### 路径解析 (`SafeExecutablePath`)
通过 `os.Executable()` 获得当前可执行文件路径，并必须调用 `filepath.EvalSymlinks()` 展开软链接，获得真实物理文件的绝对路径。

#### Unix 实现 (`replace.go`):
```go
// 1. 设置可执行权限
if err := os.Chmod(tmpPath, 0755); err != nil {
    return err
}
// 2. 利用 Unix Inode 机制原子替换
if err := os.Rename(tmpPath, realExecPath); err != nil {
    // 检查是否因跨文件系统导致 EXDEV 错误，若跨分区则 fallback 为 copy + rename
    return fallbackCrossDeviceRename(tmpPath, realExecPath)
}
```

#### Windows 实现 (`replace_windows.go`):
```go
// 1. 尝试清理上一次可能遗留的 .old 文件
oldPath := realExecPath + ".old"
_ = os.Remove(oldPath)

// 2. 将正在运行的自身重命名为 .old (Windows 允许重命名打开的 exe)
if err := os.Rename(realExecPath, oldPath); err != nil {
    return fmt.Errorf("failed to move running executable to .old: %w", err)
}

// 3. 将新下载的文件写入原本的 exe 路径
if err := os.Rename(tmpPath, realExecPath); err != nil {
    // 若失败，尝试恢复原程序
    _ = os.Rename(oldPath, realExecPath)
    return fmt.Errorf("failed to place new executable: %w", err)
}

// 4. 清理旧文件（若仍被自身锁住，留待下次运行自动清理）
_ = os.Remove(oldPath)
```

### 4.5 编译与版本注入规范

在 `scripts/build-release.sh` 和本地开发构建命令中，更新 `ldflags`：
```bash
# 规范化语义版本号注入
LDFLAGS="-s -w -X 'main.Version=${VERSION}'"

go build -ldflags="${LDFLAGS}" -o bin/statusline ./cmd/statusline
go build -ldflags="${LDFLAGS}" -o bin/statusline-server ./cmd/statusline-server
```
将原本写死的 `var Version = "1.0.0"` 改为默认 `var Version = "dev"`，在构建 Release 时由外部参数动态注入（如 `v0.1.6`）。

---

## 5. 质量保证与测试策略

1. **单元测试 (`internal/updater/updater_test.go`)**：
   - **语义化版本比较**：测试 `v0.1.5 < v0.1.6`、`v0.1.6 == v0.1.6`、`v0.2.0 > v0.1.9` 等版本号比较与降级允许逻辑；
   - **纯 Go 解压测试**：使用标准库动态在内存中构造 `.tar.gz` 与 `.zip` 压缩包，测试流式精准提取二进制的能力；
   - **完整性校验**：测试 SHA256 一致时通过、散列篡改时报错拦截。
2. **模拟双源集成测试**：
   - 启动本地 Mock HTTP Server，分别模拟 `statusline-server` 的 `/api/v1/version` 与 GitHub API 的 Release 返回；
   - 验证服务端优先命中、服务端宕机无缝回退至 GitHub 的切换逻辑。
3. **跨平台全量编译与回归测试**：
   - Linux amd64、Windows amd64、macOS arm64 交叉编译测试；
   - 保证全库 `go test ./...` 100% 通过。
