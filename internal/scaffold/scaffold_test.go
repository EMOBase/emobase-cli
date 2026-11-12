package scaffold

import (
	"bytes"
	"strings"
	"testing"
)

func TestQuoteArg(t *testing.T) {
	tests := map[string]string{
		"plain":     "plain",
		"two words": "'two words'",
		"*":         "'*'",
		"":          "''",
		"a'b":       `'a'\''b'`,
		"/tmp/a b":  "'/tmp/a b'",
	}
	for in, want := range tests {
		if got := quoteArg(in); got != want {
			t.Errorf("quoteArg(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDryRunPrintsWithoutExecuting(t *testing.T) {
	var out, errb bytes.Buffer
	r := &Runner{DryRun: true, Stdout: &out, Stderr: &errb}
	if err := r.Compose("/tmp/does-not-exist", "--profile", "*", "pull"); err != nil {
		t.Fatal(err)
	}
	want := "+ cd /tmp/does-not-exist && docker compose --profile '*' pull\n"
	if out.String() != want {
		t.Fatalf("out = %q", out.String())
	}
	if errb.Len() != 0 {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestCloneRunsFromParent(t *testing.T) {
	var out bytes.Buffer
	r := &Runner{DryRun: true, Stdout: &out}
	if err := r.Clone("/tmp/work/emobase-deploy-tcas", "https://example.org/repo.git", "dev"); err != nil {
		t.Fatal(err)
	}
	line := out.String()
	for _, want := range []string{"cd /tmp/work", "git clone -b dev https://example.org/repo.git /tmp/work/emobase-deploy-tcas"} {
		if !strings.Contains(line, want) {
			t.Fatalf("%q does not contain %q", line, want)
		}
	}
}
