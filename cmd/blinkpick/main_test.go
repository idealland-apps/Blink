package main

import (
	"bytes"
	"testing"
)

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer

	exitCode := run([]string{"--help"}, &bytes.Buffer{}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %s", exitCode, stderr.String())
	}
	if got := stdout.String(); got == "" {
		t.Fatal("help output is empty")
	}
}
