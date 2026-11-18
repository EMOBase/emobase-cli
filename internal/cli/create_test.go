package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpeciesDirNameKeepsCase(t *testing.T) {
	tests := map[string]string{
		"Tcas":         "Tcas",
		" lsta ":       "lsta",
		"dmel":         "dmel",
		"T. castaneum": "tcastaneum",
	}
	for code, want := range tests {
		if got := speciesDirName(code); got != want {
			t.Errorf("speciesDirName(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestCreateSpeciesUsesCodeAsDirectory(t *testing.T) {
	root := t.TempDir()
	cmd := New()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--workspace", root, "--yes", "create", "species", "--species", "Tcas", "--local", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "Tcas"); !strings.Contains(out.String(), want) {
		t.Fatalf("expected the checkout at %s, output:\n%s", want, out.String())
	}
}

func TestPromptCheckoutDirDefaultsWithoutATerminal(t *testing.T) {
	root := t.TempDir()
	def := filepath.Join(root, "infra")

	cmd := New()
	cmd.SetIn(strings.NewReader("")) // not a terminal
	a := &App{}
	got, err := a.promptCheckoutDir(cmd, root, def)
	if err != nil {
		t.Fatal(err)
	}
	if got != def {
		t.Errorf("dir = %q, want the default %q", got, def)
	}

	yes := &App{Yes: true}
	if got, err := yes.promptCheckoutDir(cmd, root, def); err != nil || got != def {
		t.Errorf("--yes dir = %q (err %v), want %q", got, err, def)
	}
}

func TestCreateSpeciesUsesDiscoveredInfraDir(t *testing.T) {
	root := t.TempDir()
	// A custom directory name must still be recognised as the infra checkout.
	mkCLIStack(t, root, "shared-infra", testInfraCompose, "KEYCLOAK_PUBLIC_URL=http://localhost:8181/keycloak\n")

	cmd := New()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--workspace", root, "--yes", "create", "species", "--species", "Tcas", "--local", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "shared-infra"); !strings.Contains(out.String(), "--infra-dir "+want) {
		t.Fatalf("expected the discovered infra dir %s, output:\n%s", want, out.String())
	}
}

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
