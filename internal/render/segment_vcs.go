package render

import (
	"fmt"

	"statusline/internal/model"
)

// FormatVCS renders the Git branch and status if available:
// e.g. "(main)" when clean, "(main*)" with yellow * when dirty.
// Returns empty string if no branch information is present.
func FormatVCS(data *model.StatusLineData) string {
	if data == nil || data.VCS.Branch == "" {
		return ""
	}

	branch := data.VCS.Branch
	if data.VCS.Dirty {
		return fmt.Sprintf("%s(%s%s*%s)%s", AnsiLavender, branch, AnsiYellow, AnsiLavender, AnsiReset)
	}
	return fmt.Sprintf("%s(%s)%s", AnsiLavender, branch, AnsiReset)
}
