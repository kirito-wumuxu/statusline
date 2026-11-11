package render

import (
	"fmt"
	"strings"

	"statusline/internal/model"
)

// FormatDir formats the current working directory, replacing the user home directory with '~',
// and colors it with Sky Blue.
func FormatDir(data *model.StatusLineData, homeDir string) string {
	dir := ""
	if data != nil {
		if data.Workspace.CurrentDir != "" {
			dir = data.Workspace.CurrentDir
		} else if data.CWD != "" {
			dir = data.CWD
		}
	}

	var res string
	if dir == "" || dir == homeDir {
		res = "[~]"
	} else if homeDir != "" && strings.HasPrefix(dir, homeDir) {
		rel := strings.TrimPrefix(dir, homeDir)
		if strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
			res = fmt.Sprintf("[~%s]", rel)
		} else {
			res = fmt.Sprintf("[~/%s]", rel)
		}
	} else {
		res = fmt.Sprintf("[%s]", dir)
	}

	return fmt.Sprintf("%s%s%s", AnsiSkyBlue, res, AnsiReset)
}
