package updater

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"statusline/internal/config"
)

// UpgradeOptions specifies options for upgrading statusline.
type UpgradeOptions struct {
	CheckOnly        bool
	UpgradeAll       bool
	TargetTag        string
	ForcedSource     string
	Yes              bool
	CurrentVer       string
	TargetExecutable string               // optional override for testing or custom paths
	ClientConfig     *config.ClientConfig // optional client config for testing
}

// RunUpgrade parses CLI flags and executes the self-upgrade process.
func RunUpgrade(args []string, currentVer string, out io.Writer) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(out)

	checkFlag := fs.Bool("check", false, "Only check for new releases without downloading or installing")
	allFlag := fs.Bool("all", false, "Also upgrade statusline-server if present in the same directory")
	versionFlag := fs.String("version", "latest", "Target version tag to install (default: latest)")
	sourceFlag := fs.String("source", "auto", "Update source: auto, server, or github (default: auto)")
	yesFlag := fs.Bool("y", false, "Skip confirmation prompts")
	fs.BoolVar(yesFlag, "yes", false, "Skip confirmation prompts")

	if err := fs.Parse(args); err != nil {
		return err
	}

	opts := UpgradeOptions{
		CheckOnly:    *checkFlag,
		UpgradeAll:   *allFlag,
		TargetTag:    *versionFlag,
		ForcedSource: *sourceFlag,
		Yes:          *yesFlag,
		CurrentVer:   currentVer,
	}

	return ExecuteUpgrade(opts, out)
}

// ExecuteUpgrade carries out the upgrade workflow with the given options.
func ExecuteUpgrade(opts UpgradeOptions, out io.Writer) error {
	execPath := opts.TargetExecutable
	var err error
	if execPath == "" {
		execPath, err = GetExecutablePath()
		if err != nil {
			return fmt.Errorf("failed to locate current executable: %w", err)
		}
	}

	baseName := strings.ToLower(filepath.Base(execPath))
	isServer := strings.HasPrefix(baseName, "statusline-server")

	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}

	var primaryName, primaryLabel string
	var siblingName, siblingLabel string
	if isServer {
		primaryName = "statusline-server" + ext
		primaryLabel = "statusline-server"
		siblingName = "statusline" + ext
		siblingLabel = "statusline"
	} else {
		primaryName = "statusline" + ext
		primaryLabel = "statusline"
		siblingName = "statusline-server" + ext
		siblingLabel = "statusline-server"
	}

	CleanupOldExecutables(execPath)

	fmt.Fprintf(out, "[1/4] 当前环境: %s %s (%s/%s)\n", primaryLabel, opts.CurrentVer, runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(out, "      可执行文件: %s\n", execPath)

	execDir := filepath.Dir(execPath)
	siblingPath := filepath.Join(execDir, siblingName)
	hasSiblingInSameDir := false
	if _, err := os.Stat(siblingPath); err == nil {
		hasSiblingInSameDir = true
		fmt.Fprintf(out, "      关联组件: 检测到同目录下存在 %s\n", siblingName)
	}

	// 2. Resolve update source
	fmt.Fprintf(out, "[2/4] 正在探测可用更新源 (模式: %s)...\n", opts.ForcedSource)
	cfg := opts.ClientConfig
	if cfg == nil {
		cfg, _ = config.LoadClientConfig("")
	}
	if isServer && cfg != nil && cfg.Report.Endpoint == "http://127.0.0.1:8628/api/v1/report" {
		if serverCfg, sErr := config.LoadServerConfig(""); sErr == nil && serverCfg != nil && serverCfg.Server.Port > 0 && serverCfg.Server.Port != 8628 {
			cfg.Report.Endpoint = fmt.Sprintf("http://127.0.0.1:%d/api/v1/report", serverCfg.Server.Port)
		}
	}
	src, err := ResolveSource(cfg, opts.ForcedSource)
	if err != nil {
		return fmt.Errorf("无法确定更新源: %w", err)
	}
	fmt.Fprintf(out, "      ✓ 使用更新源: %s\n", src.Name())

	// 3. Query release info
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	rel, err := src.CheckRelease(ctx, opts.TargetTag)
	if err != nil {
		return fmt.Errorf("查询新版本失败: %w", err)
	}

	platformKey := CurrentPlatform()
	downloadURL, ok := rel.Assets[platformKey]
	if !ok || downloadURL == "" {
		return fmt.Errorf("版本 %s 未提供当前平台 (%s) 的安装包", rel.Version, platformKey)
	}

	isNew := IsNewer(rel.Version, opts.CurrentVer)
	isCustomVersion := opts.TargetTag != "" && opts.TargetTag != "latest"

	if !isNew && !isCustomVersion {
		fmt.Fprintf(out, "✓ 当前已是最新版本 (%s)，无需升级。\n", opts.CurrentVer)
		return nil
	}

	fmt.Fprintf(out, "[3/4] 发现可用版本: %s -> %s\n", opts.CurrentVer, rel.Version)
	if rel.ReleaseNotes != "" {
		// Output up to 3 lines of release notes
		lines := strings.Split(strings.TrimSpace(rel.ReleaseNotes), "\n")
		fmt.Fprintf(out, "      发布说明: %s\n", lines[0])
	}
	fmt.Fprintf(out, "      下载文件: %s\n", filepath.Base(downloadURL))

	if opts.CheckOnly {
		fmt.Fprintf(out, "✓ 检查完成 (-check 模式，不执行下载与安装)。\n")
		return nil
	}

	// Confirm prompt unless -y
	if !opts.Yes {
		fmt.Fprintf(out, "? 是否确认升级到 %s? [Y/n]: ", rel.Version)
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "" && input != "y" && input != "yes" {
			fmt.Fprintf(out, "升级已取消。\n")
			return nil
		}
	}

	// 4. Download asset package to temporary file in same directory
	fmt.Fprintf(out, "[4/4] 正在下载升级包...\n")
	archiveExt := ".tar.gz"
	if runtime.GOOS == "windows" {
		archiveExt = ".zip"
	}
	tmpArchive := filepath.Join(execDir, fmt.Sprintf(".statusline_upgrade_%s%s", rel.Version, archiveExt))
	defer os.Remove(tmpArchive)

	lastPct := -1
	err = src.Download(ctx, downloadURL, tmpArchive, func(downloaded, total int64) {
		if total > 0 {
			pct := int((downloaded * 100) / total)
			if pct != lastPct && pct%20 == 0 {
				lastPct = pct
				fmt.Fprintf(out, "      下载进度: %d%% (%d / %d KB)\n", pct, downloaded/1024, total/1024)
			}
		}
	})
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}

	// Verify SHA256 if available
	pkgFilename := filepath.Base(downloadURL)
	if expectedHash, ok := rel.Checksums[pkgFilename]; ok && expectedHash != "" {
		fmt.Fprintf(out, "      校验 SHA256 完整性散列值...\n")
		if vErr := VerifySHA256(tmpArchive, expectedHash); vErr != nil {
			return fmt.Errorf("安全校验失败: %w", vErr)
		}
		fmt.Fprintf(out, "      ✓ 校验通过 (%s)\n", expectedHash[:12])
	}

	// Extract and atomically replace primary binary
	tmpPrimary := filepath.Join(execDir, fmt.Sprintf(".%s.new.tmp", primaryLabel))
	defer os.Remove(tmpPrimary)

	if err := ExtractBinaryFromArchive(tmpArchive, primaryName, tmpPrimary); err != nil {
		return fmt.Errorf("解压 %s 二进制失败: %w", primaryLabel, err)
	}

	if err := ReplaceExecutable(execPath, tmpPrimary); err != nil {
		return fmt.Errorf("替换 %s 失败: %w", primaryLabel, err)
	}
	fmt.Fprintf(out, "      ✓ %s 已成功替换为 %s\n", primaryLabel, rel.Version)

	// If -all and sibling exists in same directory, upgrade it too
	if opts.UpgradeAll && hasSiblingInSameDir {
		tmpSibling := filepath.Join(execDir, fmt.Sprintf(".%s.new.tmp", siblingLabel))
		defer os.Remove(tmpSibling)

		if err := ExtractBinaryFromArchive(tmpArchive, siblingName, tmpSibling); err == nil {
			if sErr := ReplaceExecutable(siblingPath, tmpSibling); sErr == nil {
				fmt.Fprintf(out, "      ✓ %s 已同时成功替换为 %s\n", siblingLabel, rel.Version)
			} else {
				fmt.Fprintf(out, "      ! 替换 %s 失败: %v\n", siblingLabel, sErr)
			}
		} else {
			fmt.Fprintf(out, "      ! 压缩包中未找到 %s: %v\n", siblingName, err)
		}
	} else if hasSiblingInSameDir && !opts.UpgradeAll {
		fmt.Fprintf(out, "      (提示: 同目录下存在 %s，如需一同升级请使用 %s upgrade --all)\n", siblingName, primaryLabel)
	}

	if isServer {
		fmt.Fprintf(out, "      (提示: 若 %s 作为系统常驻服务运行，请执行相应命令重启服务使新版本生效，如: systemctl restart %s)\n", primaryLabel, primaryLabel)
	}

	fmt.Fprintf(out, "✓ 升级全部完成！新版本已即时生效。\n")
	return nil
}
