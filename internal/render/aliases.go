package render

import (
	"strings"

	"statusline/internal/config"
	"statusline/internal/model"
)

var defaultAliases = config.DisplayAliasesConfig{
	Quotas: []config.DisplayAliasItem{
		{Key: "gemini", Name: "Gemini", ShortName: "G"},
		{Key: "gemini-argon", Name: "Argon", ShortName: "Ar"},
		{Key: "3p", Name: "Claude", ShortName: "C"},
		{Key: "claude", Name: "Claude", ShortName: "C"},
	},
	Models: []config.DisplayAliasItem{
		{Key: "gemini-3.8-flash", Name: "Gemini 3.8 Flash", ShortName: "Flash"},
		{Key: "gemini-3.8-pro", Name: "Gemini 3.8 Pro", ShortName: "Pro"},
		{Key: "claude-3-7-sonnet", Name: "Claude 3.7 Sonnet", ShortName: "Sonnet"},
		{Key: "claude-opus", Name: "Claude Opus", ShortName: "Opus"},
	},
	Accounts: []config.DisplayAliasItem{},
}

var currentAliases = defaultAliases

// SetAliases updates the active aliases configuration used by rendering functions.
func SetAliases(aliases config.DisplayAliasesConfig) {
	if len(aliases.Quotas) > 0 || len(aliases.Models) > 0 || len(aliases.Accounts) > 0 {
		currentAliases = aliases
	}
}

// MatchQuotaAlias strips duration suffix and matches the longest prefix in quota aliases.
func MatchQuotaAlias(rawKey string) (name, shortName, window string) {
	prefix, win := model.SplitQuotaKeySuffix(rawKey)
	prefixLower := strings.ToLower(prefix)

	bestMatchLen := -1
	var bestItem *config.DisplayAliasItem

	for i := range currentAliases.Quotas {
		item := &currentAliases.Quotas[i]
		kLower := strings.ToLower(item.Key)
		if prefixLower == kLower || strings.HasPrefix(prefixLower, kLower) {
			if len(kLower) > bestMatchLen {
				bestMatchLen = len(kLower)
				bestItem = item
			}
		}
	}

	if bestItem != nil {
		return bestItem.Name, bestItem.ShortName, win
	}

	// Fallback: title case prefix
	capitalized := strings.Title(prefix)
	short := capitalized
	if len(capitalized) > 2 {
		short = capitalized[:2]
	}
	return capitalized, short, win
}

// MatchModelAlias matches model ID or display name against model aliases.
func MatchModelAlias(modelID, displayName string) (name, shortName string) {
	target := strings.ToLower(modelID)
	if target == "" {
		target = strings.ToLower(displayName)
	}

	for _, item := range currentAliases.Models {
		kLower := strings.ToLower(item.Key)
		if strings.Contains(target, kLower) || strings.Contains(strings.ToLower(displayName), kLower) {
			return item.Name, item.ShortName
		}
	}

	return displayName, ""
}

// MatchAccountAlias matches email/account against account aliases.
func MatchAccountAlias(email string) (name, shortName string) {
	if email == "" {
		return "", ""
	}
	lower := strings.ToLower(strings.TrimSpace(email))

	for _, item := range currentAliases.Accounts {
		kLower := strings.ToLower(item.Key)
		if lower == kLower || strings.HasPrefix(lower, kLower) {
			return item.Name, item.ShortName
		}
	}

	return email, ""
}
