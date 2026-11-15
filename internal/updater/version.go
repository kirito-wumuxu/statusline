package updater

import (
	"strconv"
	"strings"
)

// CleanVersion strips leading 'v' or 'V' and trims whitespace.
func CleanVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}

// SemVer represents major.minor.patch numeric components.
type SemVer struct {
	Major int
	Minor int
	Patch int
	Raw   string
	IsDev bool
}

// ParseSemVer parses a version string into SemVer components.
func ParseSemVer(v string) SemVer {
	cleaned := CleanVersion(v)
	if cleaned == "" || strings.EqualFold(cleaned, "dev") || strings.EqualFold(cleaned, "none") {
		return SemVer{Raw: v, IsDev: true}
	}

	// Remove any prerelease or build metadata for numeric comparison (e.g. 1.2.3-beta.1 -> 1.2.3)
	mainPart := cleaned
	if idx := strings.IndexAny(mainPart, "-+"); idx != -1 {
		mainPart = mainPart[:idx]
	}

	parts := strings.Split(mainPart, ".")
	var major, minor, patch int
	if len(parts) > 0 {
		major, _ = strconv.Atoi(parts[0])
	}
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		patch, _ = strconv.Atoi(parts[2])
	}

	return SemVer{
		Major: major,
		Minor: minor,
		Patch: patch,
		Raw:   v,
		IsDev: false,
	}
}

// CompareSemVer compares v1 and v2.
// Returns:
//   -1 if v1 < v2
//    0 if v1 == v2
//    1 if v1 > v2
// Special handling for "dev":
// If v1 is "dev" and v2 is a release version, v1 is considered older (returns -1) unless v1 is target.
// Note: when v1="dev" and v2="v0.1.5", we treat v1 as -1 (older than v0.1.5 release) or vice versa.
func CompareSemVer(v1, v2 string) int {
	s1 := ParseSemVer(v1)
	s2 := ParseSemVer(v2)

	if s1.IsDev && s2.IsDev {
		return 0
	}
	if s1.IsDev && !s2.IsDev {
		// v1 is dev, v2 is release: dev is older than any formal release
		return -1
	}
	if !s1.IsDev && s2.IsDev {
		// v1 is release, v2 is dev: release is newer than dev
		return 1
	}

	if s1.Major != s2.Major {
		if s1.Major > s2.Major {
			return 1
		}
		return -1
	}
	if s1.Minor != s2.Minor {
		if s1.Minor > s2.Minor {
			return 1
		}
		return -1
	}
	if s1.Patch != s2.Patch {
		if s1.Patch > s2.Patch {
			return 1
		}
		return -1
	}

	return 0
}

// IsNewer returns true if targetVersion is strictly newer than currentVersion.
func IsNewer(targetVersion, currentVersion string) bool {
	// If current is "dev", any formal release target is considered newer.
	sCurrent := ParseSemVer(currentVersion)
	sTarget := ParseSemVer(targetVersion)
	if sCurrent.IsDev && !sTarget.IsDev {
		return true
	}
	return CompareSemVer(targetVersion, currentVersion) > 0
}
