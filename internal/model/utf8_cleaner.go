package model

import (
	"strings"
	"unicode/utf8"
)

// CleanUTF8 sanitizes invalid UTF-8 bytes to ensure safe storage and display.
func CleanUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "")
}
