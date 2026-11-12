package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newTestCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetIn(new(bytes.Buffer))
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	return cmd
}

func TestReadPromptLineDoesNotReadAhead(t *testing.T) {
	r := strings.NewReader("Tcas\nsecond value\n")
	first, err := readPromptLine(r)
	if err != nil {
		t.Fatal(err)
	}
	if first != "Tcas" {
		t.Fatalf("first = %q", first)
	}
	second, err := readPromptLine(r)
	if err != nil && second == "" {
		t.Fatal(err)
	}
	if second != "second value" {
		t.Fatalf("second = %q", second)
	}
}

func TestPromptRefusesNonTerminalInput(t *testing.T) {
	cmd := newTestCommand()
	value := ""
	if err := prompt(cmd, "Value", "default", &value); err == nil {
		t.Fatal("expected an error for a non-terminal reader")
	}
	if value != "" {
		t.Fatalf("value = %q", value)
	}
}

func TestPromptKeepsExplicitValue(t *testing.T) {
	cmd := newTestCommand()
	value := "flag"
	if err := prompt(cmd, "Value", "default", &value); err != nil {
		t.Fatal(err)
	}
	if value != "flag" {
		t.Fatalf("value = %q", value)
	}
}

func TestCanPromptRejectsBuffers(t *testing.T) {
	if canPrompt(new(bytes.Buffer)) {
		t.Fatal("a buffer is not a terminal")
	}
}
