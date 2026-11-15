package updater

import "testing"

func TestCompareSemVer(t *testing.T) {
	cases := []struct {
		v1, v2 string
		want   int
	}{
		{"v0.1.5", "v0.1.6", -1},
		{"v0.1.6", "v0.1.6", 0},
		{"0.1.6", "v0.1.6", 0},
		{"v0.2.0", "v0.1.9", 1},
		{"v1.0.0", "v0.9.9", 1},
		{"v0.1.5", "dev", 1},  // release is treated as newer than dev
		{"dev", "v0.1.5", -1}, // dev is treated as older than any release
	}
	for _, tc := range cases {
		got := CompareSemVer(tc.v1, tc.v2)
		if got != tc.want {
			t.Errorf("CompareSemVer(%q, %q) = %d, want %d", tc.v1, tc.v2, got, tc.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	if !IsNewer("v0.1.6", "v0.1.5") {
		t.Errorf("expected v0.1.6 to be newer than v0.1.5")
	}
	if IsNewer("v0.1.5", "v0.1.5") {
		t.Errorf("expected v0.1.5 to not be newer than v0.1.5")
	}
	if IsNewer("v0.1.4", "v0.1.5") {
		t.Errorf("expected v0.1.4 to not be newer than v0.1.5")
	}
}

func TestCleanVersion(t *testing.T) {
	if CleanVersion("v1.2.3") != "1.2.3" {
		t.Errorf("expected 1.2.3, got %s", CleanVersion("v1.2.3"))
	}
	if CleanVersion("1.2.3") != "1.2.3" {
		t.Errorf("expected 1.2.3, got %s", CleanVersion("1.2.3"))
	}
}
