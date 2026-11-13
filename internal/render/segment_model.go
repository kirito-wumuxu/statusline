package render

import (
	"fmt"
	"strings"

	"statusline/internal/model"
)

// FormatModel formats the model display name and reasoning effort level,
// colored in Bold Bright Green.
func FormatModel(data *model.StatusLineData) string {
	if data == nil || (data.Model.DisplayName == "" && data.Model.ID == "") {
		return fmt.Sprintf("%s[Gemini]%s", AnsiBrightGreen, AnsiReset)
	}

	name := data.Model.DisplayName
	if name == "" || name == data.Model.ID {
		aliasName, _ := MatchModelAlias(data.Model.ID, data.Model.DisplayName)
		if aliasName != "" {
			name = aliasName
		} else if name == "" {
			name = data.Model.ID
		}
	}

	effort := strings.TrimSpace(data.Model.Effort)
	var res string
	if effort != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(effort)) {
		res = fmt.Sprintf("[%s (%s)]", name, effort)
	} else {
		res = fmt.Sprintf("[%s]", name)
	}

	return fmt.Sprintf("%s%s%s", AnsiBrightGreen, res, AnsiReset)
}
