package main

import (
	"bytes"
	"testing"
)

func TestServerUpgradeCheckFlag(t *testing.T) {
	var buf bytes.Buffer
	err := runUpgradeCommand([]string{"-check"}, &buf)
	if err != nil {
		t.Fatalf("runUpgradeCommand -check failed: %v", err)
	}
	out := buf.String()
	if out == "" {
		t.Errorf("expected output from -check")
	}
}

func TestServerUpgradeHelpFlag(t *testing.T) {
	var buf bytes.Buffer
	_ = runUpgradeCommand([]string{"--help"}, &buf)
	out := buf.String()
	if out == "" {
		t.Errorf("expected help output")
	}
}
