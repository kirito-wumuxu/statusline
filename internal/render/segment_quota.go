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

// FormatQuota renders the quota section:
// e.g. "Gemini: 93%(2h10m) 89%(5d18h) | Claude: 100%(4h59m) 99%(5d18h)"
func FormatQuota(data *model.StatusLineData) string {
	geminiBuckets := []bucketInfo{}
	claudeBuckets := []bucketInfo{}

	if data != nil && data.Quota != nil {
		for k, b := range data.Quota {
			lower := strings.ToLower(k)
			remPct := int(math.Round(b.RemainingFraction * 100))
			item := bucketInfo{key: k, bucket: b, remaining: remPct}

			if strings.Contains(lower, "gemini") {
				geminiBuckets = append(geminiBuckets, item)
			} else if strings.Contains(lower, "claude") || strings.Contains(lower, "sonnet") || strings.Contains(lower, "opus") || strings.Contains(lower, "3p") {
				claudeBuckets = append(claudeBuckets, item)
			}
		}
	}

	sort.Slice(geminiBuckets, func(i, j int) bool {
		return geminiBuckets[i].bucket.ResetInSeconds < geminiBuckets[j].bucket.ResetInSeconds
	})
	sort.Slice(claudeBuckets, func(i, j int) bool {
		return claudeBuckets[i].bucket.ResetInSeconds < claudeBuckets[j].bucket.ResetInSeconds
	})

	geminiStr := formatProviderBuckets(geminiBuckets, "2h0m", "7d0h")
	claudeStr := formatProviderBuckets(claudeBuckets, "5h0m", "7d0h")

	geminiLabel := fmt.Sprintf("%sGemini:%s", AnsiBrightCyan, AnsiReset)
	claudeLabel := fmt.Sprintf("%sClaude:%s", AnsiOrange, AnsiReset)
	sep := fmt.Sprintf("%s|%s", AnsiDarkGray, AnsiReset)

	return fmt.Sprintf("%s %s %s %s %s", geminiLabel, geminiStr, sep, claudeLabel, claudeStr)
}

func formatProviderBuckets(buckets []bucketInfo, defaultShortDuration, defaultLongDuration string) string {
	shortStr := formatBucketItem(100, defaultShortDuration)
	longStr := formatBucketItem(100, defaultLongDuration)

	if len(buckets) >= 1 {
		shortStr = formatBucketItem(buckets[0].remaining, FormatDurationCompact(buckets[0].bucket.ResetInSeconds))
	}
	if len(buckets) >= 2 {
		longStr = formatBucketItem(buckets[1].remaining, FormatDurationCompact(buckets[1].bucket.ResetInSeconds))
	}

	return fmt.Sprintf("%s %s", shortStr, longStr)
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
