package render

import (
	"fmt"
	"strings"

	"statusline/internal/model"
)

// AssembleLine builds the complete ANSI-formatted statusline, applying
// adaptive compaction if data.TerminalWidth is specified and too narrow.
func AssembleLine(data *model.StatusLineData, homeDir string) string {
	sep := fmt.Sprintf("%s|%s", AnsiDarkGray, AnsiReset)

	// Build standard components
	dirStr := FormatDir(data, homeDir)
	vcsStr := FormatVCS(data)
	modelStr := FormatModel(data)
	ctxStr := FormatContext(data)
	tokensStr := FormatTokens(data)
	quotaStr := FormatQuota(data)

	// Combine components
	var parts []string
	if dirStr != "" {
		parts = append(parts, dirStr)
	}
	if vcsStr != "" {
		parts = append(parts, vcsStr)
	}
	parts = append(parts, modelStr, ctxStr, sep, tokensStr, sep, quotaStr)
	fullLine := strings.Join(parts, " ")

	termWidth := 0
	if data != nil && data.TerminalWidth > 0 {
		termWidth = data.TerminalWidth
	}

	// If no width constraint or line fits, return standard full line
	if termWidth <= 0 || len(StripANSI(fullLine)) <= termWidth {
		return fullLine
	}

	// Level 1: Compact model and quota labels
	modelCompact := formatModelCompact(data)
	quotaCompact := formatQuotaCompact(data, false)
	parts = nil
	if dirStr != "" {
		parts = append(parts, dirStr)
	}
	if vcsStr != "" {
		parts = append(parts, vcsStr)
	}
	parts = append(parts, modelCompact, ctxStr, sep, tokensStr, sep, quotaCompact)
	l1 := strings.Join(parts, " ")
	if len(StripANSI(l1)) <= termWidth {
		return l1
	}

	// Level 2: Compact context bar (4 slashes) and single-bucket quota
	ctxCompact := formatContextCompact(data)
	quotaUltra := formatQuotaCompact(data, true)
	parts = nil
	if dirStr != "" {
		parts = append(parts, dirStr)
	}
	if vcsStr != "" {
		parts = append(parts, vcsStr)
	}
	parts = append(parts, modelCompact, ctxCompact, sep, tokensStr, sep, quotaUltra)
	l2 := strings.Join(parts, " ")
	if len(StripANSI(l2)) <= termWidth {
		return l2
	}

	// Level 3: Ultra narrow (e.g. < 70 chars) - drop dir and keep core stats
	parts = []string{modelCompact, ctxCompact, sep, tokensStr, sep, quotaUltra}
	return strings.Join(parts, " ")
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
	geminiBuckets := []bucketInfo{}
	claudeBuckets := []bucketInfo{}

	if data != nil && data.Quota != nil {
		for k, b := range data.Quota {
			lower := strings.ToLower(k)
			item := bucketInfo{key: k, bucket: b, remaining: int(b.RemainingFraction * 100)}
			if strings.Contains(lower, "gemini") {
				geminiBuckets = append(geminiBuckets, item)
			} else if strings.Contains(lower, "claude") || strings.Contains(lower, "sonnet") || strings.Contains(lower, "opus") || strings.Contains(lower, "3p") {
				claudeBuckets = append(claudeBuckets, item)
			}
		}
	}

	gStr := "100%"
	if len(geminiBuckets) > 0 {
		b := geminiBuckets[0]
		dur := FormatDurationCompact(b.bucket.ResetInSeconds)
		gStr = formatBucketItem(b.remaining, dur)
		if !singleBucketOnly && len(geminiBuckets) > 1 {
			b2 := geminiBuckets[1]
			dur2 := FormatDurationCompact(b2.bucket.ResetInSeconds)
			gStr = fmt.Sprintf("%s %s", gStr, formatBucketItem(b2.remaining, dur2))
		}
	}

	cStr := "100%"
	if len(claudeBuckets) > 0 {
		b := claudeBuckets[0]
		dur := FormatDurationCompact(b.bucket.ResetInSeconds)
		cStr = formatBucketItem(b.remaining, dur)
		if !singleBucketOnly && len(claudeBuckets) > 1 {
			b2 := claudeBuckets[1]
			dur2 := FormatDurationCompact(b2.bucket.ResetInSeconds)
			cStr = fmt.Sprintf("%s %s", cStr, formatBucketItem(b2.remaining, dur2))
		}
	}

	sep := fmt.Sprintf("%s|%s", AnsiDarkGray, AnsiReset)
	gLabel := fmt.Sprintf("%sG:%s", AnsiBrightCyan, AnsiReset)
	cLabel := fmt.Sprintf("%sC:%s", AnsiOrange, AnsiReset)

	return fmt.Sprintf("%s %s %s %s %s", gLabel, gStr, sep, cLabel, cStr)
}
