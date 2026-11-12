package cli

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/EMOBase/emobase-cli/internal/envfile"
	"github.com/EMOBase/emobase-cli/internal/workspace"
)

func domainTestStack(role workspace.Role, name, dir string, env map[string]string) *workspace.Stack {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(env[k])
		b.WriteString("\n")
	}
	path := dir + "/.env"
	return &workspace.Stack{
		Role:    role,
		Name:    name,
		Dir:     dir,
		EnvPath: path,
		Env:     envfile.Parse(path, b.String()),
	}
}

func domainFixture() *workspace.Workspace {
	infra := domainTestStack(workspace.RoleInfra, "emobase-infra", "/srv/emobase/emobase-infra", map[string]string{
		"KEYCLOAK_PUBLIC_URL": "http://localhost:8081/keycloak",
		"KEYCLOAK_HOST_PORT":  "8081",
		"KEYCLOAK_REALM":      "ibb",
	})
	portal := domainTestStack(workspace.RolePortal, "emobase-portal-deploy", "/srv/emobase/emobase-portal-deploy", map[string]string{
		"PUBLIC_SERVER":       "http://localhost:9080",
		"KEYCLOAK_PUBLIC_URL": "http://localhost:8081/keycloak",
		"KEYCLOAK_REALM":      "ibb",
	})
	species := domainTestStack(workspace.RoleSpecies, "emobase-deploy-tcas", "/srv/emobase/emobase-deploy-tcas", map[string]string{
		"MAIN_SPECIES":             "Tcas",
		"PUBLIC_SERVER":            "http://localhost:9090",
		"PORTAL_SERVER":            "http://localhost:9080",
		"PORTAL_INTERNAL_URL":      "http://host.docker.internal:9080",
		"KEYCLOAK_PUBLIC_URL":      "http://localhost:8081/keycloak",
		"KEYCLOAK_REALM":           "ibb",
		"INTERNAL_KEYCLOAK_ISSUER": "http://host.docker.internal:8081/keycloak/realms/ibb",
	})
	return &workspace.Workspace{Root: "/srv/emobase", Infra: infra, Portal: portal, Species: []*workspace.Stack{species}}
}

func domainValuesFor(ws *workspace.Workspace) *domainValues {
	vals := &domainValues{species: map[*workspace.Stack]string{}}
	vals.auth, _ = ws.Infra.Get("KEYCLOAK_PUBLIC_URL")
	if ws.Portal != nil {
		vals.portal, _ = ws.Portal.Get("PUBLIC_SERVER")
	}
	for _, sp := range ws.Species {
		vals.species[sp], _ = sp.Get("PUBLIC_SERVER")
	}
	return vals
}

func TestDomainPlanAuthChangeUpdatesEveryStack(t *testing.T) {
	ws := domainFixture()
	vals := domainValuesFor(ws)
	vals.auth = "https://auth.example.org/keycloak"

	plan, err := buildDomainPlan(ws, vals)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.AuthChanged {
		t.Fatal("expected auth change")
	}
	for _, want := range []string{
		"infra/KEYCLOAK_PUBLIC_URL",
		"portal/KEYCLOAK_PUBLIC_URL",
		"species:emobase-deploy-tcas/KEYCLOAK_PUBLIC_URL",
		"species:emobase-deploy-tcas/INTERNAL_KEYCLOAK_ISSUER",
	} {
		if !hasDomainChange(plan, want) {
			t.Errorf("missing %s", want)
		}
	}
	if len(plan.Origins) != 0 {
		t.Fatalf("auth-only change should not add origins: %#v", plan.Origins)
	}
	if len(plan.Restarts) != 3 {
		t.Fatalf("restarts = %d", len(plan.Restarts))
	}
	if got := findDomainChange(plan, "species:emobase-deploy-tcas/INTERNAL_KEYCLOAK_ISSUER").New; got != "https://auth.example.org/keycloak/realms/ibb" {
		t.Fatalf("issuer = %q", got)
	}
}

func TestDomainPlanPortalChangeUpdatesSpeciesPortalServer(t *testing.T) {
	ws := domainFixture()
	vals := domainValuesFor(ws)
	vals.portal = "https://portal.example.org"

	plan, err := buildDomainPlan(ws, vals)
	if err != nil {
		t.Fatal(err)
	}
	if plan.AuthChanged {
		t.Fatal("did not expect auth change")
	}
	if !hasDomainChange(plan, "portal/PUBLIC_SERVER") || !hasDomainChange(plan, "species:emobase-deploy-tcas/PORTAL_SERVER") {
		t.Fatalf("changes = %#v", plan.Changes)
	}
	if hasDomainChange(plan, "infra/KEYCLOAK_PUBLIC_URL") {
		t.Fatal("portal change must not touch the Keycloak URL")
	}
	if len(plan.Origins) != 1 || plan.Origins[0].Origin != "https://portal.example.org" {
		t.Fatalf("origins = %#v", plan.Origins)
	}
	if len(plan.Restarts) != 2 {
		t.Fatalf("restarts = %d", len(plan.Restarts))
	}
}

func TestDomainPlanSpeciesChangeIsLocal(t *testing.T) {
	ws := domainFixture()
	vals := domainValuesFor(ws)
	vals.species[ws.Species[0]] = "https://tcas.example.org"

	plan, err := buildDomainPlan(ws, vals)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Key != "PUBLIC_SERVER" {
		t.Fatalf("changes = %#v", plan.Changes)
	}
	if len(plan.Origins) != 1 || plan.Origins[0].Realm != "ibb" {
		t.Fatalf("origins = %#v", plan.Origins)
	}
	if len(plan.Restarts) != 1 || plan.Restarts[0] != ws.Species[0] {
		t.Fatalf("restarts = %#v", plan.Restarts)
	}
}

func TestDomainPlanNoChanges(t *testing.T) {
	ws := domainFixture()
	plan, err := buildDomainPlan(ws, domainValuesFor(ws))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 0 || len(plan.Origins) != 0 || len(plan.Restarts) != 0 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestDomainURLValidation(t *testing.T) {
	if got, err := normalizeAuthURL("https://auth.example.org"); err != nil || got != "https://auth.example.org/keycloak" {
		t.Fatalf("auth = %q, %v", got, err)
	}
	if _, err := normalizeAuthURL("https://auth.example.org/other"); err == nil {
		t.Fatal("expected auth path error")
	}
	if _, err := normalizeOrigin("https://portal.example.org/path"); err == nil {
		t.Fatal("expected origin path error")
	}
	if got, err := normalizeOrigin("https://portal.example.org/"); err != nil || got != "https://portal.example.org" {
		t.Fatalf("origin = %q, %v", got, err)
	}
}

func TestDerivedInternalIssuer(t *testing.T) {
	if got := derivedInternalIssuer("http://localhost:8081/keycloak", "8081", "ibb"); got != "http://host.docker.internal:8081/keycloak/realms/ibb" {
		t.Fatalf("loopback issuer = %q", got)
	}
	if got := derivedInternalIssuer("https://auth.example.org/keycloak", "8081", "ibb_tcas"); got != "https://auth.example.org/keycloak/realms/ibb_tcas" {
		t.Fatalf("public issuer = %q", got)
	}
}

func TestDomainPlanCommand(t *testing.T) {
	root := t.TempDir()
	mkCLIStack(t, root, "emobase-infra", testInfraCompose, strings.Join([]string{
		"KEYCLOAK_PUBLIC_URL=http://localhost:8081/keycloak",
		"KEYCLOAK_REALM=ibb",
		"KEYCLOAK_HOST_PORT=8081",
		"",
	}, "\n"))
	mkCLIStack(t, root, "emobase-portal-deploy", testPortalCompose, strings.Join([]string{
		"PUBLIC_SERVER=http://localhost:9080",
		"KEYCLOAK_PUBLIC_URL=http://localhost:8081/keycloak",
		"KEYCLOAK_REALM=ibb",
		"",
	}, "\n"))
	mkCLIStack(t, root, "emobase-deploy-tcas", testSpeciesCompose, strings.Join([]string{
		"MAIN_SPECIES=Tcas",
		"PUBLIC_SERVER=http://localhost:9090",
		"PORTAL_SERVER=http://localhost:9080",
		"KEYCLOAK_PUBLIC_URL=http://localhost:8081/keycloak",
		"KEYCLOAK_REALM=ibb",
		"",
	}, "\n"))

	cmd := New()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{
		"--workspace", root, "--no-color",
		"domain", "plan", "--portal", "https://portal.example.org",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v\n%s", err, errb.String())
	}
	for _, want := range []string{"portal", "PUBLIC_SERVER", "https://portal.example.org", "species:emobase-deploy-tcas"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func hasDomainChange(plan *domainPlan, id string) bool {
	return findDomainChange(plan, id) != nil
}

func findDomainChange(plan *domainPlan, id string) *domainChange {
	for i := range plan.Changes {
		if plan.Changes[i].Scope+"/"+plan.Changes[i].Key == id {
			return &plan.Changes[i]
		}
	}
	return nil
}
