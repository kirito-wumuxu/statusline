package render

import (
	"fmt"
	"strings"

	"statusline/internal/model"
)

// FormatAccount formats the account email into angle brackets <user@email.com>.
// Returns empty string if data or email is not present.
func FormatAccount(data *model.StatusLineData) string {
	if data == nil || data.Email == "" {
		return ""
	}
	email := strings.TrimSpace(data.Email)
	if email == "" {
		return ""
	}
	return fmt.Sprintf("%s<%s%s%s>%s", AnsiDarkGray, AnsiAmber, email, AnsiDarkGray, AnsiReset)
}

// formatAccountCompact formats only the username/prefix before @ for narrow screens.
func formatAccountCompact(data *model.StatusLineData) string {
	if data == nil || data.Email == "" {
		return ""
	}
	email := strings.TrimSpace(data.Email)
	if email == "" {
		return ""
	}
	prefix := email
	if idx := strings.Index(email, "@"); idx > 0 {
		prefix = email[:idx]
	}
	return fmt.Sprintf("%s<%s%s%s>%s", AnsiDarkGray, AnsiAmber, prefix, AnsiDarkGray, AnsiReset)
}
