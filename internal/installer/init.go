package installer

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Options holds configuration for statusline initialization.
type Options struct {
	SettingsPath string
	BinPath      string
	NoCopy       bool
}

// RunInit executes the `statusline init` command.
func RunInit(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(out)

	var opts Options
	fs.StringVar(&opts.SettingsPath, "settings", "", "Custom path to settings.json")
	fs.StringVar(&opts.BinPath, "bin", "", "Custom binary path to configure")
	fs.BoolVar(&opts.NoCopy, "no-copy", false, "Skip copying binary to ~/.local/bin")

	if err := fs.Parse(args); err != nil {
		return err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("unable to determine home directory: %w", err)
	}

	// 1. Determine target binary path
	targetBin := opts.BinPath
	if targetBin == "" {
		currentExe, err := os.Executable()
		if err == nil {
			if resolved, err := filepath.EvalSymlinks(currentExe); err == nil {
				currentExe = resolved
			}
		}

		installDir := filepath.Join(homeDir, ".local", "bin")
		binName := "statusline"
		if filepath.Ext(currentExe) == ".exe" {
			binName = "statusline.exe"
		}
		destBin := filepath.Join(installDir, binName)

		if !opts.NoCopy && currentExe != "" && currentExe != destBin {
			if err := copyExecutable(currentExe, destBin); err == nil {
				targetBin = destBin
				fmt.Fprintf(out, "✔ Installed binary to: %s\n", destBin)
			} else {
				// Fallback to currentExe if copying fails
				targetBin = currentExe
				fmt.Fprintf(out, "ℹ Using current binary path: %s\n", currentExe)
			}
		} else if currentExe != "" {
			targetBin = currentExe
		} else {
			targetBin = destBin
		}
	}

	// 2. Identify settings.json files to configure
	var settingsFiles []string
	if opts.SettingsPath != "" {
		settingsFiles = append(settingsFiles, opts.SettingsPath)
	} else {
		// Standard Antigravity settings path
		agySettings := filepath.Join(homeDir, ".gemini", "antigravity-cli", "settings.json")
		geminiSettings := filepath.Join(homeDir, ".gemini", "settings.json")

		if _, err := os.Stat(agySettings); err == nil {
			settingsFiles = append(settingsFiles, agySettings)
		} else if _, err := os.Stat(geminiSettings); err == nil {
			settingsFiles = append(settingsFiles, geminiSettings)
		} else {
			// If neither exists, default to creating the standard Antigravity CLI path
			settingsFiles = append(settingsFiles, agySettings)
		}
	}

	// 3. Configure each settings file
	for _, sf := range settingsFiles {
		if err := configureSettingsFile(sf, targetBin); err != nil {
			return fmt.Errorf("failed to configure %s: %w", sf, err)
		}
		fmt.Fprintf(out, "✔ Configured Antigravity settings: %s\n", sf)
		fmt.Fprintf(out, "  - statusLine.command: %s\n", targetBin)
		fmt.Fprintf(out, "  - statusLine.enabled: true\n")
	}

	fmt.Fprintln(out, "🎉 Statusline initialized successfully! Restart Antigravity CLI to apply changes.")
	return nil
}

func copyExecutable(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Remove destination first in case it's currently running or has different permissions
	_ = os.Remove(dst)

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Sync()
}

func configureSettingsFile(filePath, binPath string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	settings := make(map[string]any)

	if data, err := os.ReadFile(filePath); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &settings); err != nil {
			// If invalid JSON, backup original file
			_ = os.WriteFile(filePath+".bak", data, 0644)
			settings = make(map[string]any)
		}
	}

	var statusLine map[string]any
	if existing, ok := settings["statusLine"].(map[string]any); ok {
		statusLine = existing
	} else {
		statusLine = make(map[string]any)
	}

	if _, ok := statusLine["type"]; !ok {
		statusLine["type"] = ""
	}
	statusLine["command"] = binPath
	statusLine["enabled"] = true

	settings["statusLine"] = statusLine

	formatted, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	formatted = append(formatted, '\n')

	return os.WriteFile(filePath, formatted, 0644)
}
