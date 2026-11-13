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

type providerGroup struct {
	name      string
	shortName string
	color     string
	buckets   []bucketInfo
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

func getProviderColor(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "gemini"):
		return AnsiBrightCyan
	case strings.Contains(lower, "argon"):
		return AnsiLavender
	case strings.Contains(lower, "claude") || strings.Contains(lower, "3p"):
		return AnsiOrange
	default:
		return AnsiWhite
	}
}

func providerPriority(name string) int {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "gemini"):
		return 1
	case strings.Contains(lower, "argon"):
		return 2
	case strings.Contains(lower, "claude") || lower == "c" || strings.Contains(lower, "3p"):
		return 3
	default:
		return 10
	}
}

func extractProviderGroups(data *model.StatusLineData) []providerGroup {
	if data == nil || data.Quota == nil || len(data.Quota) == 0 {
		return nil
	}

	groupMap := make(map[string]*providerGroup)
	for k, b := range data.Quota {
		name, shortName, _ := MatchQuotaAlias(k)
		remPct := int(math.Round(b.RemainingFraction * 100))
		item := bucketInfo{key: k, bucket: b, remaining: remPct}

		grp, exists := groupMap[name]
		if !exists {
			color := getProviderColor(name)
			grp = &providerGroup{
				name:      name,
				shortName: shortName,
				color:     color,
			}
			groupMap[name] = grp
		}
		grp.buckets = append(grp.buckets, item)
	}

	var groups []providerGroup
	for _, grp := range groupMap {
		sort.Slice(grp.buckets, func(i, j int) bool {
			return grp.buckets[i].bucket.ResetInSeconds < grp.buckets[j].bucket.ResetInSeconds
		})
		groups = append(groups, *grp)
	}

	sort.Slice(groups, func(i, j int) bool {
		pI := providerPriority(groups[i].name)
		pJ := providerPriority(groups[j].name)
		if pI != pJ {
			return pI < pJ
		}
		return groups[i].name < groups[j].name
	})

	return groups
}

// FormatQuota renders the quota section:
// e.g. "Gemini: 93%(2h10m) 89%(5d18h) | Argon: 100%(4d4h) | Claude: 100%(4h59m) 99%(5d18h)"
// If no quota data exists for a provider, it is omitted.
// If no quota data exists at all, returns an empty string.
func FormatQuota(data *model.StatusLineData) string {
	groups := extractProviderGroups(data)
	if len(groups) == 0 {
		return ""
	}

	var parts []string
	for _, g := range groups {
		label := fmt.Sprintf("%s%s:%s", g.color, g.name, AnsiReset)
		bStr := formatProviderBuckets(g.buckets)
		parts = append(parts, fmt.Sprintf("%s %s", label, bStr))
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
