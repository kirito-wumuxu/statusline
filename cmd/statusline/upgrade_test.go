package main

import (
	"bytes"
	"testing"
)

func TestUpgradeCheckFlag(t *testing.T) {
	var buf bytes.Buffer
	// Running upgrade with -check should succeed without downloading
	err := runUpgradeCommand([]string{"-check"}, &buf)
	if err != nil {
		t.Fatalf("runUpgradeCommand -check failed: %v", err)
	}
	out := buf.String()
	if out == "" {
		t.Errorf("expected output from -check")
	}
}

func TestUpgradeHelpFlag(t *testing.T) {
	var buf bytes.Buffer
	_ = runUpgradeCommand([]string{"--help"}, &buf)
	out := buf.String()
	if out == "" {
		t.Errorf("expected help output")
	}
}
