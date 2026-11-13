package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func fakeDocker(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	script := filepath.Join(bin, "docker")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
}

func runSpeciesRemove(t *testing.T, args ...string) error {
	t.Helper()
	cmd := New()
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestSpeciesRemovePurgeDeletesCheckout(t *testing.T) {
	root := matrixFixture(t)
	fakeDocker(t)
	dir := filepath.Join(root, "emobase-deploy-tcas")

	if err := runSpeciesRemove(t, "--workspace", root, "--no-color", "--yes", "species", "remove", "tcas", "--purge"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("checkout still exists after purge: %v", err)
	}
}

func TestSpeciesRemoveKeepDirKeepsCheckout(t *testing.T) {
	root := matrixFixture(t)
	fakeDocker(t)
	dir := filepath.Join(root, "emobase-deploy-tcas")

	if err := runSpeciesRemove(t, "--workspace", root, "--no-color", "--yes", "species", "remove", "tcas", "--purge", "--keep-dir"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("checkout was removed despite --keep-dir: %v", err)
	}
}
