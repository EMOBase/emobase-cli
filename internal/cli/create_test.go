package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestCreateInfraYesClonesMissingCheckout(t *testing.T) {
	root := t.TempDir()
	cmd := New()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--workspace", root, "--yes", "create", "infra", "--local", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "git clone") {
		t.Fatalf("expected --yes to clone the missing checkout, output:\n%s", out.String())
	}
}

func TestCreateInfraNonInteractiveRequiresClone(t *testing.T) {
	root := t.TempDir()
	cmd := New()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--workspace", root, "create", "infra", "--local", "--dry-run"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--clone") {
		t.Fatalf("expected a --clone hint, got %v", err)
	}
}
