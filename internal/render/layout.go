package render

import (
	"fmt"
	"strings"

	"statusline/internal/model"
)

// AssembleLine builds the complete ANSI-formatted statusline, applying
// modular segment rendering and adaptive compaction if data.TerminalWidth is specified.
func AssembleLine(data *model.StatusLineData, homeDir string, customSegments ...[]string) string {
	segments := []string{"dir", "vcs", "account", "model", "context", "tokens", "quota"}
	if len(customSegments) > 0 && len(customSegments[0]) > 0 {
		segments = customSegments[0]
	}

	fullLine := buildLine(data, homeDir, segments, 0)

	termWidth := 0
	if data != nil && data.TerminalWidth > 0 {
		termWidth = data.TerminalWidth
	}

	// If no width constraint or line fits, return standard full line
	if termWidth <= 0 || len(StripANSI(fullLine)) <= termWidth {
		return fullLine
	}

	// Level 1: Compact model, account, and quota labels
	l1 := buildLine(data, homeDir, segments, 1)
	if len(StripANSI(l1)) <= termWidth {
		return l1
	}

	// Level 2: Compact context bar (4 slashes) and single-bucket quota
	l2 := buildLine(data, homeDir, segments, 2)
	if len(StripANSI(l2)) <= termWidth {
		return l2
	}

	// Level 3: Ultra narrow (e.g. < 70 chars) - drop dir, vcs, account and keep core stats
	return buildLine(data, homeDir, segments, 3)
}

func buildLine(data *model.StatusLineData, homeDir string, segments []string, level int) string {
	sep := fmt.Sprintf("%s|%s", AnsiDarkGray, AnsiReset)

	hasExplicitSep := false
	for _, s := range segments {
		clean := strings.TrimSpace(s)
		if clean == "|" || clean == "sep" {
			hasExplicitSep = true
			break
		}
	}

	var parts []string
	needSep := false

	for _, rawSeg := range segments {
		clean := strings.ToLower(strings.TrimSpace(rawSeg))
		if clean == "|" || clean == "sep" {
			if len(parts) > 0 {
				needSep = true
			}
			continue
		}

		// Level 3 (ultra narrow): drop non-essential contextual segments
		if level == 3 && (clean == "dir" || clean == "cwd" || clean == "vcs" || clean == "git" || clean == "account" || clean == "user" || clean == "email") {
			continue
		}

		txt := renderSegment(clean, data, homeDir, level)
		if txt == "" {
			continue
		}

		// If no explicit separator was configured, auto-insert pipe before tokens and quota
		if !hasExplicitSep && (clean == "tokens" || clean == "quota") {
			if len(parts) > 0 && parts[len(parts)-1] != sep {
				needSep = true
			}
		}

		if needSep && len(parts) > 0 && parts[len(parts)-1] != sep {
			parts = append(parts, sep)
		}
		needSep = false

		parts = append(parts, txt)
	}

	return strings.Join(parts, " ")
}

func renderSegment(name string, data *model.StatusLineData, homeDir string, level int) string {
	switch name {
	case "dir", "cwd":
		return FormatDir(data, homeDir)
	case "vcs", "git":
		return FormatVCS(data)
	case "account", "user", "email":
		if level == 0 {
			return FormatAccount(data)
		}
		return formatAccountCompact(data)
	case "model":
		if level == 0 {
			return FormatModel(data)
		}
		return formatModelCompact(data)
	case "context", "ctx":
		if level <= 1 {
			return FormatContext(data)
		}
		return formatContextCompact(data)
	case "tokens":
		return FormatTokens(data)
	case "quota":
		if level == 0 {
			return FormatQuota(data)
		} else if level == 1 {
			return formatQuotaCompact(data, false)
		}
		return formatQuotaCompact(data, true)
	default:
		return ""
	}
}

func formatModelCompact(data *model.StatusLineData) string {
	if data == nil {
		return fmt.Sprintf("%s[%sFlash%s]%s", AnsiDarkGray, AnsiBrightGreen, AnsiDarkGray, AnsiReset)
	}
	name := data.Model.DisplayName
	if name == "" {
		name = data.Model.ID
	}
	nameLower := strings.ToLower(name)

	short := "Flash"
	if strings.Contains(nameLower, "pro") {
		short = "Pro"
	} else if strings.Contains(nameLower, "sonnet") {
		short = "Sonnet"
	} else if strings.Contains(nameLower, "opus") {
		short = "Opus"
	} else if strings.Contains(nameLower, "haiku") {
		short = "Haiku"
	} else if strings.Contains(nameLower, "flash") {
		short = "Flash"
	}

	if data.Model.Effort != "" && !strings.Contains(strings.ToLower(short), strings.ToLower(data.Model.Effort)) {
		effortShort := "H"
		if strings.ToLower(data.Model.Effort) == "medium" || strings.ToLower(data.Model.Effort) == "med" {
			effortShort = "M"
		} else if strings.ToLower(data.Model.Effort) == "low" {
			effortShort = "L"
		}
		short = fmt.Sprintf("%s-%s", short, effortShort)
	}

	return fmt.Sprintf("%s[%s%s%s]%s", AnsiDarkGray, AnsiBrightGreen, short, AnsiDarkGray, AnsiReset)
}

func formatContextCompact(data *model.StatusLineData) string {
	pct := 0
	if data != nil {
		pct = int(data.ContextWindow.UsedPercentage)
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}

	filled := (pct * 4) / 100
	if pct > 0 && filled == 0 {
		filled = 1
	}

	color := AnsiGreen
	if pct >= 90 {
		color = AnsiRed
	} else if pct >= 70 {
		color = AnsiYellow
	}

	var sb strings.Builder
	sb.WriteString(color)
	for i := 0; i < filled; i++ {
		sb.WriteString(`\`)
	}
	sb.WriteString(AnsiDarkGray)
	for i := filled; i < 4; i++ {
		sb.WriteString(`\`)
	}
	sb.WriteString(AnsiReset)
	sb.WriteString(fmt.Sprintf(" %s%d%%%s", AnsiBrightWhite, pct, AnsiReset))
	return sb.String()
}

func formatQuotaCompact(data *model.StatusLineData, singleBucketOnly bool) string {
	geminiBuckets, claudeBuckets := extractProviderBuckets(data)
	if len(geminiBuckets) == 0 && len(claudeBuckets) == 0 {
		return ""
	}

	var parts []string
	if len(geminiBuckets) > 0 {
		b := geminiBuckets[0]
		dur := FormatDurationCompact(b.bucket.ResetInSeconds)
		gStr := formatBucketItem(b.remaining, dur)
		if !singleBucketOnly && len(geminiBuckets) > 1 {
			b2 := geminiBuckets[1]
			dur2 := FormatDurationCompact(b2.bucket.ResetInSeconds)
			gStr = fmt.Sprintf("%s %s", gStr, formatBucketItem(b2.remaining, dur2))
		}
		gLabel := fmt.Sprintf("%sG:%s", AnsiBrightCyan, AnsiReset)
		parts = append(parts, fmt.Sprintf("%s %s", gLabel, gStr))
	}

	if len(claudeBuckets) > 0 {
		b := claudeBuckets[0]
		dur := FormatDurationCompact(b.bucket.ResetInSeconds)
		cStr := formatBucketItem(b.remaining, dur)
		if !singleBucketOnly && len(claudeBuckets) > 1 {
			b2 := claudeBuckets[1]
			dur2 := FormatDurationCompact(b2.bucket.ResetInSeconds)
			cStr = fmt.Sprintf("%s %s", cStr, formatBucketItem(b2.remaining, dur2))
		}
		cLabel := fmt.Sprintf("%sC:%s", AnsiOrange, AnsiReset)
		parts = append(parts, fmt.Sprintf("%s %s", cLabel, cStr))
	}

	sep := fmt.Sprintf(" %s|%s ", AnsiDarkGray, AnsiReset)
	return strings.Join(parts, sep)
}
