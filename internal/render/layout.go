package render

import (
	"fmt"
	"strings"

	"statusline/internal/model"
)

type SegmentState int

const (
	StateFull    SegmentState = 0
	StateShort   SegmentState = 1
	StateCompact SegmentState = 2
	StateHidden  SegmentState = 3
)

func canonicalSegmentName(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "dir", "cwd":
		return "dir"
	case "vcs", "git":
		return "vcs"
	case "account", "user", "email":
		return "account"
	case "model":
		return "model"
	case "context", "ctx":
		return "context"
	case "tokens":
		return "tokens"
	case "quota":
		return "quota"
	default:
		return strings.ToLower(strings.TrimSpace(s))
	}
}

// AssembleLine builds the complete ANSI-formatted statusline, applying
// modular segment rendering and priority-based progressive degradation.
func AssembleLine(data *model.StatusLineData, homeDir string, customOptions ...[]string) string {
	segments := []string{"dir", "vcs", "account", "model", "context", "tokens", "quota"}
	if len(customOptions) > 0 && len(customOptions[0]) > 0 {
		segments = customOptions[0]
	}

	var priorities []string
	if len(customOptions) > 1 && len(customOptions[1]) > 0 {
		priorities = customOptions[1]
	} else {
		priorities = []string{"model", "context", "quota", "vcs", "tokens", "account", "dir"}
	}

	return AssembleLineWithPriorities(data, homeDir, segments, priorities)
}

// AssembleLineWithPriorities implements the 3-phase greedy progressive degradation algorithm.
func AssembleLineWithPriorities(data *model.StatusLineData, homeDir string, segments []string, priorities []string) string {
	termWidth := 0
	if data != nil && data.TerminalWidth > 0 {
		termWidth = data.TerminalWidth
	}

	// Initialize all configured segments in StateFull
	states := make(map[string]SegmentState)
	segSet := make(map[string]bool)
	for _, s := range segments {
		clean := canonicalSegmentName(s)
		if clean != "|" && clean != "sep" {
			states[clean] = StateFull
			segSet[clean] = true
		}
	}

	fullLine := buildLineWithStates(data, homeDir, segments, states)
	if termWidth <= 0 || len(StripANSI(fullLine)) <= termWidth {
		return fullLine
	}

	// Build degrade order: reverse of priorities (lowest priority degrades first)
	// Any segment present in segments but missing in priorities is placed at the front (degraded earliest)
	var degradeOrder []string
	seen := make(map[string]bool)

	var revPriorities []string
	for i := len(priorities) - 1; i >= 0; i-- {
		revPriorities = append(revPriorities, canonicalSegmentName(priorities[i]))
	}

	// Segments not in priorities come first
	for _, s := range segments {
		clean := canonicalSegmentName(s)
		if clean == "|" || clean == "sep" {
			continue
		}
		inP := false
		for _, p := range priorities {
			if canonicalSegmentName(p) == clean {
				inP = true
				break
			}
		}
		if !inP && !seen[clean] {
			seen[clean] = true
			degradeOrder = append(degradeOrder, clean)
		}
	}

	for _, p := range revPriorities {
		if segSet[p] && !seen[p] {
			seen[p] = true
			degradeOrder = append(degradeOrder, p)
		}
	}

	tryFit := func() (string, bool) {
		line := buildLineWithStates(data, homeDir, segments, states)
		if len(StripANSI(line)) <= termWidth {
			return line, true
		}
		return "", false
	}

	// Phase 1: Progressive ShortName replacement
	for _, mod := range degradeOrder {
		switch mod {
		case "dir", "account", "model", "quota":
			if states[mod] == StateFull {
				states[mod] = StateShort
				if line, ok := tryFit(); ok {
					return line
				}
			}
		}
	}

	// Phase 2: Progressive structural compacting
	for _, mod := range degradeOrder {
		switch mod {
		case "context", "quota":
			if states[mod] < StateCompact {
				states[mod] = StateCompact
				if line, ok := tryFit(); ok {
					return line
				}
			}
		}
	}

	// Phase 3: Progressive module pruning / hiding (protect last remaining visible module)
	for _, mod := range degradeOrder {
		visibleCount := 0
		for m, st := range states {
			if segSet[m] && st != StateHidden {
				visibleCount++
			}
		}
		if visibleCount <= 1 {
			break
		}

		states[mod] = StateHidden
		if line, ok := tryFit(); ok {
			return line
		}
	}

	return buildLineWithStates(data, homeDir, segments, states)
}

func buildLineWithStates(data *model.StatusLineData, homeDir string, segments []string, states map[string]SegmentState) string {
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

		canonical := canonicalSegmentName(clean)
		state := states[canonical]
		if state == StateHidden {
			continue
		}

		txt := renderSegmentWithState(canonical, data, homeDir, state)
		if txt == "" {
			continue
		}

		// If no explicit separator was configured, auto-insert pipe before tokens and quota
		if !hasExplicitSep && (canonical == "tokens" || canonical == "quota") {
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

func buildLine(data *model.StatusLineData, homeDir string, segments []string, level int) string {
	states := make(map[string]SegmentState)
	for _, s := range segments {
		clean := canonicalSegmentName(s)
		if clean == "|" || clean == "sep" {
			continue
		}
		switch level {
		case 0:
			states[clean] = StateFull
		case 1:
			switch clean {
			case "dir", "account", "model", "quota":
				states[clean] = StateShort
			default:
				states[clean] = StateFull
			}
		case 2:
			switch clean {
			case "dir", "account", "model":
				states[clean] = StateShort
			case "context", "quota":
				states[clean] = StateCompact
			default:
				states[clean] = StateFull
			}
		case 3:
			switch clean {
			case "dir", "vcs", "account":
				states[clean] = StateHidden
			case "model":
				states[clean] = StateShort
			case "context", "quota":
				states[clean] = StateCompact
			default:
				states[clean] = StateFull
			}
		}
	}
	return buildLineWithStates(data, homeDir, segments, states)
}

func renderSegment(name string, data *model.StatusLineData, homeDir string, level int) string {
	state := StateFull
	if level == 1 {
		state = StateShort
	} else if level == 2 {
		state = StateCompact
	} else if level == 3 {
		state = StateHidden
	}
	return renderSegmentWithState(canonicalSegmentName(name), data, homeDir, state)
}

func renderSegmentWithState(name string, data *model.StatusLineData, homeDir string, state SegmentState) string {
	if state == StateHidden {
		return ""
	}
	switch name {
	case "dir":
		if state == StateFull {
			return FormatDir(data, homeDir)
		}
		return FormatDirCompact(data, homeDir)
	case "vcs":
		return FormatVCS(data)
	case "account":
		if state == StateFull {
			return FormatAccount(data)
		}
		return formatAccountCompact(data)
	case "model":
		if state == StateFull {
			return FormatModel(data)
		}
		return formatModelCompact(data)
	case "context":
		if state <= StateShort {
			return FormatContext(data)
		}
		return formatContextCompact(data)
	case "tokens":
		return FormatTokens(data)
	case "quota":
		if state == StateFull {
			return FormatQuota(data)
		} else if state == StateShort {
			return formatQuotaCompact(data, true, false)
		}
		return formatQuotaCompact(data, true, true)
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

	_, shortName := MatchModelAlias(data.Model.ID, data.Model.DisplayName)
	short := shortName

	if short == "" {
		nameLower := strings.ToLower(name)
		short = "Flash"
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

func formatQuotaCompact(data *model.StatusLineData, useShortLabel bool, singleBucketOnly bool) string {
	groups := extractProviderGroups(data)
	if len(groups) == 0 {
		return ""
	}

	var parts []string
	for _, g := range groups {
		if len(g.buckets) == 0 {
			continue
		}
		var bItems []string
		limit := 2
		if singleBucketOnly || len(g.buckets) < 2 {
			limit = 1
		}
		for i := 0; i < limit && i < len(g.buckets); i++ {
			b := g.buckets[i]
			dur := FormatDurationCompact(b.bucket.ResetInSeconds)
			bItems = append(bItems, formatBucketItem(b.remaining, dur))
		}
		labelName := g.name
		if useShortLabel && g.shortName != "" {
			labelName = g.shortName
		}
		label := fmt.Sprintf("%s%s:%s", g.color, labelName, AnsiReset)
		parts = append(parts, fmt.Sprintf("%s %s", label, strings.Join(bItems, " ")))
	}

	sep := fmt.Sprintf(" %s|%s ", AnsiDarkGray, AnsiReset)
	return strings.Join(parts, sep)
}
