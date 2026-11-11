package render

import "regexp"

const (
	AnsiReset       = "\033[0m"
	AnsiBold        = "\033[1m"
	AnsiDim         = "\033[2m"

	// Standard ANSI foreground colors
	AnsiRed         = "\033[31m"
	AnsiGreen       = "\033[32m"
	AnsiYellow      = "\033[33m"
	AnsiBlue        = "\033[34m"
	AnsiMagenta     = "\033[35m"
	AnsiCyan        = "\033[36m"
	AnsiWhite       = "\033[37m"
	AnsiDarkGray    = "\033[90m"
	AnsiBrightWhite = "\033[1;37m"
	AnsiBrightCyan  = "\033[1;36m"
	AnsiBrightGreen = "\033[1;32m"

	// 256 colors
	AnsiSkyBlue     = "\033[38;5;39m"  // Directory indicator
	AnsiOrange      = "\033[38;5;208m" // Claude brand accent
	AnsiLavender    = "\033[38;5;141m" // Git / VCS branch accent
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI strips all ANSI escape codes from string s.
func StripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}
