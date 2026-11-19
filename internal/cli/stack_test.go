package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EMOBase/emobase-cli/internal/workspace"
)

func TestSelectStacksByRoleAndReference(t *testing.T) {
	root := sharedEnvFixture(t)
	envPath := filepath.Join(root, "emobase-deploy-tcas", ".env")
	f, err := os.OpenFile(envPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("COMPOSE_PROJECT_NAME=tcas-test\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	ws, err := workspace.Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		only string
		want []string
	}{
		{"default", "", []string{"emobase-infra", "emobase-portal-deploy", "emobase-deploy-tcas"}},
		{"all alias", "all", []string{"emobase-infra", "emobase-portal-deploy", "emobase-deploy-tcas"}},
		{"infra role", "infra", []string{"emobase-infra"}},
		{"portal role", "portal", []string{"emobase-portal-deploy"}},
		{"species role", "species", []string{"emobase-deploy-tcas"}},
		{"species code", "Tcas", []string{"emobase-deploy-tcas"}},
		{"lowercase code", "tcas", []string{"emobase-deploy-tcas"}},
		{"checkout name", "emobase-deploy-tcas", []string{"emobase-deploy-tcas"}},
		{"project name", "tcas-test", []string{"emobase-deploy-tcas"}},
		{"fleet label", "species:emobase-deploy-tcas", []string{"emobase-deploy-tcas"}},
		{"trimmed role", " species ", []string{"emobase-deploy-tcas"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectStacks(ws, tc.only)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("selectStacks(%q) returned %d stacks, want %d", tc.only, len(got), len(tc.want))
			}
			for i, st := range got {
				if st.Name != tc.want[i] {
					t.Fatalf("selectStacks(%q)[%d] = %q, want %q", tc.only, i, st.Name, tc.want[i])
				}
			}
		})
	}

	if _, err := selectStacks(ws, "missing-stack"); err == nil {
		t.Fatal("expected an error for an unknown stack")
	}
}

func TestBootstrapOnlyOneSpecies(t *testing.T) {
	root := t.TempDir()
	mkCLIStack(t, root, "infra", testInfraCompose, "MYSQL_ROOT_PASSWORD=root\n")
	mkCLIStack(t, root, "portal", testPortalCompose, "PUBLIC_SERVER=http://localhost:9080\n")
	env := func(code, db string) string {
		return strings.Join([]string{
			"MAIN_SPECIES=" + code,
			"PUBLIC_SERVER=http://localhost:9090",
			"DIRECTUS_DB_DATABASE=" + db,
			"KEYCLOAK_REALM=" + strings.ToLower(code),
			"KEYCLOAK_CLIENT_ID=" + strings.ToLower(code) + "-client",
			"KEYCLOAK_CLIENT_SECRET=secret",
			"COCO_USERNAME=coco",
			"COCO_PASSWORD=pass",
			"COMPOSE_PROJECT_NAME=" + strings.ToLower(code),
			"",
		}, "\n")
	}
	mkCLIStack(t, root, "Tcas", testSpeciesCompose, env("Tcas", "directus_tcas"))
	mkCLIStack(t, root, "Dmel", testSpeciesCompose, env("Dmel", "directus_dmel"))

	out, errb, code := runCLI(t, "--workspace", root, "--no-color", "bootstrap", "--only", "Tcas", "--dry-run")
	if code != 0 {
		t.Fatalf("bootstrap failed: %s", errb)
	}
	if !strings.Contains(out, "Species Tcas") || !strings.Contains(out, "directus_tcas") {
		t.Fatalf("Tcas was not selected:\n%s", out)
	}
	if strings.Contains(out, "Species Dmel") || strings.Contains(out, "directus_dmel") {
		t.Fatalf("Dmel should not be selected:\n%s", out)
	}
}
