package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

const infraCompose = `services:
  keycloak:
    image: kc
  mysql:
    image: mysql
`

const portalCompose = `name: emobase-portal-deploy
services:
  genomicsservice:
    image: g
`

const speciesCompose = `services:
  directus:
    image: d
`

func mkStack(t *testing.T, root, name, compose, env string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	if env != "" {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDiscoverRolesAndOrder(t *testing.T) {
	root := t.TempDir()
	mkStack(t, root, "infra", infraCompose, "KEYCLOAK_REALM=ibb\n")
	mkStack(t, root, "portal", portalCompose, "PUBLIC_SERVER=http://localhost:9080\n")
	mkStack(t, root, "emobase-deploy-tcas", speciesCompose, "MAIN_SPECIES=Tcas\n")
	mkStack(t, root, "emobase-deploy-dmel", speciesCompose, "MAIN_SPECIES=Dmel\n")
	mkStack(t, root, "unrelated", "services:\n  web:\n    image: nginx\n", "")

	ws, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Infra == nil || ws.Infra.Name != "infra" {
		t.Fatalf("infra = %#v", ws.Infra)
	}
	if ws.Portal == nil || ws.Portal.Name != "portal" {
		t.Fatalf("portal = %#v", ws.Portal)
	}
	if len(ws.Species) != 2 {
		t.Fatalf("species = %d", len(ws.Species))
	}
	if ws.Species[0].Name != "emobase-deploy-dmel" || ws.Species[1].Name != "emobase-deploy-tcas" {
		t.Fatalf("species not sorted: %s, %s", ws.Species[0].Name, ws.Species[1].Name)
	}
	if got := len(ws.All()); got != 4 {
		t.Fatalf("All() = %d", got)
	}
	if got := len(ws.Deploy()); got != 3 {
		t.Fatalf("Deploy() = %d", got)
	}
}

func TestDiscoverRootItselfIsStack(t *testing.T) {
	root := t.TempDir()
	mkStack(t, root, "only", speciesCompose, "MAIN_SPECIES=Tcas\n")
	ws, err := Discover(filepath.Join(root, "only"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Species) != 1 || ws.Species[0].Name != "only" {
		t.Fatalf("species = %#v", ws.Species)
	}
}

func TestDiscoverMissingEnv(t *testing.T) {
	root := t.TempDir()
	mkStack(t, root, "infra", infraCompose, "")
	ws, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if !ws.Infra.MissingEnv {
		t.Fatal("expected MissingEnv")
	}
	if ws.Infra.Env == nil {
		t.Fatal("Env should be editable even when the file is missing")
	}
}

func TestDiscoverRejectsDuplicateRoles(t *testing.T) {
	root := t.TempDir()
	mkStack(t, root, "a", infraCompose, "A=1\n")
	mkStack(t, root, "b", infraCompose, "A=2\n")
	if _, err := Discover(root); err == nil {
		t.Fatal("expected an error for two infra checkouts")
	}
}

func TestDiscoverNoStacks(t *testing.T) {
	if _, err := Discover(t.TempDir()); err == nil {
		t.Fatal("expected an error for an empty workspace")
	}
}

func TestProjectNameFallbacks(t *testing.T) {
	root := t.TempDir()
	mkStack(t, root, "portal", portalCompose, "A=1\n")
	mkStack(t, root, "emobase-deploy-tcas", speciesCompose, "MAIN_SPECIES=Tcas\n")
	mkStack(t, root, "emobase-deploy-dmel", speciesCompose, "MAIN_SPECIES=Dmel\nCOMPOSE_PROJECT_NAME=dmel-prod\n")
	ws, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Portal.ProjectName != "emobase-portal-deploy" {
		t.Fatalf("portal project = %q", ws.Portal.ProjectName)
	}
	byName := map[string]string{}
	for _, sp := range ws.Species {
		byName[sp.Name] = sp.ProjectName
	}
	if byName["emobase-deploy-tcas"] != "emobase-deploy-tcas" {
		t.Fatalf("tcas project = %q", byName["emobase-deploy-tcas"])
	}
	if byName["emobase-deploy-dmel"] != "dmel-prod" {
		t.Fatalf("dmel project = %q", byName["emobase-deploy-dmel"])
	}
}

func TestFindSpecies(t *testing.T) {
	root := t.TempDir()
	dir := mkStack(t, root, "emobase-deploy-tcas", speciesCompose, "MAIN_SPECIES=Tcas\nCOMPOSE_PROJECT_NAME=tcas\n")
	ws, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"Tcas", "tcas", "emobase-deploy-tcas", dir} {
		if ws.FindSpecies(ref) == nil {
			t.Errorf("FindSpecies(%q) = nil", ref)
		}
	}
	if ws.FindSpecies("dmel") != nil {
		t.Fatal("unexpected match")
	}
}

func TestResolveRootFromStackDirectory(t *testing.T) {
	root := t.TempDir()
	dir := mkStack(t, root, "emobase-deploy-tcas", speciesCompose, "MAIN_SPECIES=Tcas\n")
	t.Chdir(dir)
	got, err := ResolveRoot("")
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("ResolveRoot = %q, want %q", got, root)
	}
}

func TestResolveRootExplicitFlag(t *testing.T) {
	root := t.TempDir()
	got, err := ResolveRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("ResolveRoot = %q", got)
	}
}

func TestStackHas(t *testing.T) {
	root := t.TempDir()
	mkStack(t, root, "emobase-deploy-tcas", speciesCompose, "A=1\nB=\n")
	ws, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	sp := ws.Species[0]
	if !sp.Has("A") {
		t.Fatal("A should be present")
	}
	if sp.Has("B") || sp.Has("C") {
		t.Fatal("empty and absent keys should not be present")
	}
}
