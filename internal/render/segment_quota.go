package render

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"statusline/internal/model"
)

type bucketInfo struct {
	key       string
	bucket    model.QuotaBucket
	remaining int
}

// FormatDurationCompact converts seconds to compact format:
// e.g. 7800s -> "2h10m", 496800s -> "5d18h"
func FormatDurationCompact(seconds int64) string {
	if seconds <= 0 {
		return "0m"
	}

	days := seconds / 86400
	remSeconds := seconds % 86400
	hours := remSeconds / 3600
	remMinutes := (remSeconds % 3600) / 60

	if days > 0 {
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, remMinutes)
	}
	return fmt.Sprintf("%dm", remMinutes)
}

func extractProviderBuckets(data *model.StatusLineData) (gemini, claude []bucketInfo) {
	if data == nil || data.Quota == nil {
		return nil, nil
	}
	for k, b := range data.Quota {
		lower := strings.ToLower(k)
		remPct := int(math.Round(b.RemainingFraction * 100))
		item := bucketInfo{key: k, bucket: b, remaining: remPct}

		if strings.Contains(lower, "gemini") {
			gemini = append(gemini, item)
		} else if strings.Contains(lower, "claude") || strings.Contains(lower, "sonnet") || strings.Contains(lower, "opus") || strings.Contains(lower, "3p") {
			claude = append(claude, item)
		}
	}

	sort.Slice(gemini, func(i, j int) bool {
		return gemini[i].bucket.ResetInSeconds < gemini[j].bucket.ResetInSeconds
	})
	sort.Slice(claude, func(i, j int) bool {
		return claude[i].bucket.ResetInSeconds < claude[j].bucket.ResetInSeconds
	})

	return gemini, claude
}

// FormatQuota renders the quota section:
// e.g. "Gemini: 93%(2h10m) 89%(5d18h) | Claude: 100%(4h59m) 99%(5d18h)"
// If no quota data exists for a provider, it is omitted.
// If no quota data exists at all, returns an empty string.
func FormatQuota(data *model.StatusLineData) string {
	geminiBuckets, claudeBuckets := extractProviderBuckets(data)
	if len(geminiBuckets) == 0 && len(claudeBuckets) == 0 {
		return ""
	}

	var parts []string
	if len(geminiBuckets) > 0 {
		geminiLabel := fmt.Sprintf("%sGemini:%s", AnsiBrightCyan, AnsiReset)
		geminiStr := formatProviderBuckets(geminiBuckets)
		parts = append(parts, fmt.Sprintf("%s %s", geminiLabel, geminiStr))
	}

	if len(claudeBuckets) > 0 {
		claudeLabel := fmt.Sprintf("%sClaude:%s", AnsiOrange, AnsiReset)
		claudeStr := formatProviderBuckets(claudeBuckets)
		parts = append(parts, fmt.Sprintf("%s %s", claudeLabel, claudeStr))
	}

	sep := fmt.Sprintf(" %s|%s ", AnsiDarkGray, AnsiReset)
	return strings.Join(parts, sep)
}

func formatProviderBuckets(buckets []bucketInfo) string {
	if len(buckets) == 0 {
		return ""
	}
	var items []string
	limit := 2
	if len(buckets) < limit {
		limit = len(buckets)
	}
	for i := 0; i < limit; i++ {
		items = append(items, formatBucketItem(buckets[i].remaining, FormatDurationCompact(buckets[i].bucket.ResetInSeconds)))
	}
	return strings.Join(items, " ")
}

func formatBucketItem(remaining int, duration string) string {
	color := AnsiGreen
	if remaining < 10 {
		color = AnsiRed
	} else if remaining < 30 {
		color = AnsiYellow
	}

	return fmt.Sprintf("%s%d%%%s%s(%s)%s", color, remaining, AnsiReset, AnsiDarkGray, duration, AnsiReset)
}
