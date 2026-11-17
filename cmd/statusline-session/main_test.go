package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestPrintHelp(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printHelp()

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "statusline-session") {
		t.Errorf("expected title in help, got: %s", output)
	}
	if !strings.Contains(output, "-all-nodes") {
		t.Errorf("expected -all-nodes in help, got: %s", output)
	}
	if !strings.Contains(output, "恢复接管") {
		t.Errorf("expected keybindings in help, got: %s", output)
	}
}
