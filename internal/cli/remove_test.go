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

// fakeDockerWithES reports a running infra Elasticsearch so cleanup runs
// immediately instead of being deferred to prune.
func fakeDockerWithES(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	script := filepath.Join(bin, "docker")
	body := `#!/bin/sh
case "$*" in
  *"compose ps"*) printf '%s\n' '[{"Service":"elasticsearch","Name":"emobase-infra-elasticsearch","State":"running","Health":"healthy"}]' ;;
esac
exit 0
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
}

func TestSpeciesRemovePurgeDeletesESIndices(t *testing.T) {
	root := matrixFixture(t)
	fakeDockerWithES(t)

	marker := filepath.Join(t.TempDir(), "es-args")
	script := filepath.Join(root, "emobase-infra", "scripts", "delete-es-indices.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$*\" > "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := runSpeciesRemove(t, "--workspace", root, "--no-color", "--yes", "species", "remove", "tcas", "--purge"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal("delete-es-indices.sh was not called:", err)
	}
	if string(got) != "--yes tcas" {
		t.Fatalf("args = %q", got)
	}
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
