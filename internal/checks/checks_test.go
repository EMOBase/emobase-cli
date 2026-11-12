package checks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EMOBase/emobase-cli/internal/workspace"
)

const (
	infraCompose   = "services:\n  keycloak:\n    image: kc\n  mysql:\n    image: mysql\n"
	portalCompose  = "services:\n  genomicsservice:\n    image: g\n"
	speciesCompose = "services:\n  directus:\n    image: d\n"
)

func infraEnv() map[string]string {
	return map[string]string{
		"KEYCLOAK_PUBLIC_URL":     "http://localhost:8081/keycloak",
		"KEYCLOAK_INTERNAL_URL":   "http://keycloak:8080/keycloak",
		"KEYCLOAK_HOST_PORT":      "8081",
		"KEYCLOAK_ADMIN":          "admin",
		"KEYCLOAK_ADMIN_PASSWORD": "admin-secret",
		"COCO_USERNAME":           "coco",
		"COCO_PASSWORD":           "coco-secret",
		"MYSQL_ROOT_PASSWORD":     "root-secret",
		"EMOBASE_INFRA_NETWORK":   "emobase-infra",
		"KEYCLOAK_REALM":          "ibb",
		"KEYCLOAK_CLIENT_ID":      "ibb-client",
		"KEYCLOAK_CLIENT_SECRET":  "client-secret",
		"PORTAL_PUBLIC_SERVER":    "http://localhost:9080",
	}
}

func portalEnv() map[string]string {
	return map[string]string{
		"PUBLIC_SERVER":                    "http://localhost:9080",
		"HTTP_PORT":                        "9080",
		"PUBLIC_DATA_DIR":                  "./data/public",
		"EMOBASE_GENOMICS_MYSQL__HOST":     "mysql",
		"EMOBASE_GENOMICS_MYSQL__PORT":     "3306",
		"EMOBASE_GENOMICS_MYSQL__USER":     "root",
		"EMOBASE_GENOMICS_MYSQL__DATABASE": "emobase_genomics",
		"AUTH_SECRET":                      "portal-secret",
		"EMOBASE_GENOMICS_IMAGE":           "g:dev",
		"EMOBASE_PORTAL_WEB_IMAGE":         "p:dev",
		"MYSQL_ROOT_PASSWORD":              "root-secret",
		"KEYCLOAK_PUBLIC_URL":              "http://localhost:8081/keycloak",
		"KEYCLOAK_INTERNAL_URL":            "http://keycloak:8080/keycloak",
		"EMOBASE_INFRA_NETWORK":            "emobase-infra",
		"KEYCLOAK_REALM":                   "ibb",
		"KEYCLOAK_CLIENT_ID":               "ibb-client",
		"KEYCLOAK_CLIENT_SECRET":           "client-secret",
	}
}

func speciesEnv() map[string]string {
	return map[string]string{
		"MAIN_SPECIES":                  "Tcas",
		"PUBLIC_SERVER":                 "http://localhost:9090",
		"HTTP_PORT":                     "9090",
		"PORTAL_SERVER":                 "http://localhost:9080",
		"PORTAL_INTERNAL_URL":           "http://host.docker.internal:9080",
		"DIRECTUS_DB_DATABASE":          "directus_tcas",
		"DIRECTUS_SECRET":               "d-secret",
		"DIRECTUS_ADMIN_EMAIL":          "admin@example.org",
		"DIRECTUS_ADMIN_PASSWORD":       "d-admin",
		"COCO_USERNAME":                 "coco",
		"COCO_PASSWORD":                 "coco-secret",
		"EMOBASE_WEB_IMAGE":             "w:dev",
		"COMPOSE_PROJECT_NAME":          "tcas",
		"SSR_OIDC_SESSION_SECRET":       "ssr-secret",
		"INTERNAL_API_GENOMICS_SERVICE": "http://genomicsservice/api",
		"INTERNAL_KEYCLOAK_ISSUER":      "http://host.docker.internal:8081/keycloak/realms/ibb",
		"INTERNAL_DIRECTUS_URL":         "http://directus:8055",
		"PUBLIC_DATA_DIR":               "./data/public",
		"PRIVATE_DATA_DIR":              "./data/private",
		"IMAGE_DIR":                     "./data/images",
		"UID":                           "1000",
		"GID":                           "1000",
		"EMOBASE_INFRA_NETWORK":         "emobase-infra",
		"MYSQL_ROOT_PASSWORD":           "root-secret",
		"KEYCLOAK_PUBLIC_URL":           "http://localhost:8081/keycloak",
		"KEYCLOAK_INTERNAL_URL":         "http://keycloak:8080/keycloak",
		"KEYCLOAK_REALM":                "ibb",
		"KEYCLOAK_CLIENT_ID":            "ibb-client",
		"KEYCLOAK_CLIENT_SECRET":        "client-secret",
	}
}

func writeEnv(t *testing.T, dir string, kv map[string]string) {
	t.Helper()
	var b strings.Builder
	for k, v := range kv {
		b.WriteString(k + "=" + v + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mkDir(t *testing.T, root, name, compose string, kv map[string]string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	if kv != nil {
		writeEnv(t, dir, kv)
	}
	return dir
}

// fixture builds a consistent workspace and returns it with per-stack env maps
// the caller can mutate before calling Discover.
func fixture(t *testing.T) (string, map[string]map[string]string) {
	t.Helper()
	root := t.TempDir()
	envs := map[string]map[string]string{
		"emobase-infra":         infraEnv(),
		"emobase-portal-deploy": portalEnv(),
		"emobase-deploy-tcas":   speciesEnv(),
	}
	mkDir(t, root, "emobase-infra", infraCompose, envs["emobase-infra"])
	mkDir(t, root, "emobase-portal-deploy", portalCompose, envs["emobase-portal-deploy"])
	mkDir(t, root, "emobase-deploy-tcas", speciesCompose, envs["emobase-deploy-tcas"])
	return root, envs
}

func discover(t *testing.T, root string, envs map[string]map[string]string) *workspace.Workspace {
	t.Helper()
	for name, kv := range envs {
		writeEnv(t, filepath.Join(root, name), kv)
	}
	ws, err := workspace.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func reload(t *testing.T, root string) *workspace.Workspace {
	t.Helper()
	ws, err := workspace.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func find(rep *Report, check string) *Finding {
	for i := range rep.Findings {
		if rep.Findings[i].Check == check {
			return &rep.Findings[i]
		}
	}
	return nil
}

func TestConsistentWorkspaceHasNoErrors(t *testing.T) {
	root, envs := fixture(t)
	rep := Run(discover(t, root, envs))
	if rep.HasErrors() {
		for _, f := range rep.Findings {
			t.Logf("%s %s: %s", f.Severity, f.Scope, f.Message)
		}
		t.Fatal("expected a consistent workspace")
	}
}

func TestMissingSharedKeyIsSafeFix(t *testing.T) {
	root, envs := fixture(t)
	delete(envs["emobase-portal-deploy"], "MYSQL_ROOT_PASSWORD")
	ws := discover(t, root, envs)

	rep := Run(ws)
	f := find(rep, "shared.MYSQL_ROOT_PASSWORD")
	if f == nil {
		t.Fatal("expected shared.MYSQL_ROOT_PASSWORD")
	}
	if f.Fix != FixSafe || !f.CanApply(false) {
		t.Fatalf("fix = %v, applicable = %v", f.Fix, f.CanApply(false))
	}

	changes, err := Apply(ws, rep, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("changes = %#v", changes)
	}
	if changes[0].Secret == false {
		t.Fatal("secret change should be marked secret")
	}
	ws = reload(t, root)
	if got, _ := ws.Portal.Get("MYSQL_ROOT_PASSWORD"); got != "root-secret" {
		t.Fatalf("value = %q", got)
	}
	if _, err := os.Stat(filepath.Join(ws.Portal.Dir, ".env.bak")); err != nil {
		t.Fatal("expected a backup")
	}
	if find(Run(ws), "shared.MYSQL_ROOT_PASSWORD") != nil {
		t.Fatal("finding should be resolved")
	}
}

func TestConflictingSharedKeyNeedsYes(t *testing.T) {
	root, envs := fixture(t)
	envs["emobase-portal-deploy"]["MYSQL_ROOT_PASSWORD"] = "wrong"
	ws := discover(t, root, envs)

	rep := Run(ws)
	f := find(rep, "shared.MYSQL_ROOT_PASSWORD")
	if f == nil {
		t.Fatal("expected a conflict finding")
	}
	if f.Fix != FixChoose || f.CanApply(false) {
		t.Fatalf("fix = %v, applicable without --yes = %v", f.Fix, f.CanApply(false))
	}
	if changes, err := Apply(ws, rep, false); err != nil || len(changes) != 0 {
		t.Fatalf("Apply without yes = %#v, %v", changes, err)
	}
	if got, _ := ws.Portal.Get("MYSQL_ROOT_PASSWORD"); got != "wrong" {
		t.Fatal("choose-class value changed without --yes")
	}

	changes, err := Apply(ws, rep, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("changes = %#v", changes)
	}
	ws = reload(t, root)
	if got, _ := ws.Portal.Get("MYSQL_ROOT_PASSWORD"); got != "root-secret" {
		t.Fatalf("value = %q", got)
	}
}

func TestEmptyComposeProjectNameIsFilled(t *testing.T) {
	root, envs := fixture(t)
	envs["emobase-deploy-tcas"]["COMPOSE_PROJECT_NAME"] = ""
	ws := discover(t, root, envs)

	rep := Run(ws)
	f := find(rep, "required.COMPOSE_PROJECT_NAME")
	if f == nil {
		t.Fatal("expected COMPOSE_PROJECT_NAME finding")
	}
	if f.Fix != FixSafe {
		t.Fatalf("fix = %v", f.Fix)
	}
	if _, err := Apply(ws, rep, false); err != nil {
		t.Fatal(err)
	}
	ws = reload(t, root)
	if got, _ := ws.Species[0].Get("COMPOSE_PROJECT_NAME"); got != "tcas" {
		t.Fatalf("value = %q", got)
	}
}

func TestTrailingSlashIsSafeFix(t *testing.T) {
	root, envs := fixture(t)
	envs["emobase-infra"]["KEYCLOAK_PUBLIC_URL"] = "http://localhost:8081/keycloak/"
	// Keep every stack in sync so only the trailing-slash finding remains.
	envs["emobase-portal-deploy"]["KEYCLOAK_PUBLIC_URL"] = "http://localhost:8081/keycloak/"
	envs["emobase-deploy-tcas"]["KEYCLOAK_PUBLIC_URL"] = "http://localhost:8081/keycloak/"
	ws := discover(t, root, envs)

	rep := Run(ws)
	f := find(rep, "url.trailing_slash.KEYCLOAK_PUBLIC_URL")
	if f == nil {
		t.Fatal("expected a trailing-slash finding")
	}
	if f.Fix != FixSafe {
		t.Fatalf("fix = %v", f.Fix)
	}
	if _, err := Apply(ws, rep, false); err != nil {
		t.Fatal(err)
	}
	ws = reload(t, root)
	if got, _ := ws.Infra.Get("KEYCLOAK_PUBLIC_URL"); got != "http://localhost:8081/keycloak" {
		t.Fatalf("value = %q", got)
	}
}

func TestPortalPublicServerSeedsInfra(t *testing.T) {
	root, envs := fixture(t)
	delete(envs["emobase-infra"], "PORTAL_PUBLIC_SERVER")
	ws := discover(t, root, envs)

	rep := Run(ws)
	f := find(rep, "portal.public_server")
	if f == nil {
		t.Fatal("expected portal.public_server")
	}
	if f.Fix != FixSafe {
		t.Fatalf("fix = %v", f.Fix)
	}
	if _, err := Apply(ws, rep, false); err != nil {
		t.Fatal(err)
	}
	ws = reload(t, root)
	if got, _ := ws.Infra.Get("PORTAL_PUBLIC_SERVER"); got != "http://localhost:9080" {
		t.Fatalf("value = %q", got)
	}
}

func TestPortalOriginChangeIsDestructive(t *testing.T) {
	root, envs := fixture(t)
	envs["emobase-infra"]["PORTAL_PUBLIC_SERVER"] = "https://old.example.org"
	ws := discover(t, root, envs)

	rep := Run(ws)
	f := find(rep, "portal.public_server")
	if f == nil {
		t.Fatal("expected portal.public_server")
	}
	if f.Fix != FixDestructive || f.CanApply(true) || f.CanApply(false) {
		t.Fatalf("destructive finding must never be applicable: %v", f.Fix)
	}
	if changes, err := Apply(ws, rep, true); err != nil || len(changes) != 0 {
		t.Fatalf("changes = %#v, %v", changes, err)
	}
	if got, _ := ws.Infra.Get("PORTAL_PUBLIC_SERVER"); got != "https://old.example.org" {
		t.Fatal("destructive value was changed")
	}
}

func TestDuplicatePortsAndDatabases(t *testing.T) {
	root, envs := fixture(t)
	envs["emobase-portal-deploy"]["HTTP_PORT"] = "9090"
	mkDir(t, root, "emobase-deploy-dmel", speciesCompose, map[string]string{
		"MAIN_SPECIES":                  "Dmel",
		"PUBLIC_SERVER":                 "http://localhost:9091",
		"HTTP_PORT":                     "9091",
		"PORTAL_SERVER":                 "http://localhost:9080",
		"PORTAL_INTERNAL_URL":           "http://host.docker.internal:9080",
		"DIRECTUS_DB_DATABASE":          "directus_tcas",
		"DIRECTUS_SECRET":               "x",
		"DIRECTUS_ADMIN_EMAIL":          "a@b.c",
		"DIRECTUS_ADMIN_PASSWORD":       "x",
		"COCO_USERNAME":                 "coco",
		"COCO_PASSWORD":                 "coco-secret",
		"EMOBASE_WEB_IMAGE":             "w:dev",
		"COMPOSE_PROJECT_NAME":          "dmel",
		"SSR_OIDC_SESSION_SECRET":       "x",
		"INTERNAL_API_GENOMICS_SERVICE": "http://g",
		"INTERNAL_KEYCLOAK_ISSUER":      "http://host.docker.internal:8081/keycloak/realms/ibb",
		"INTERNAL_DIRECTUS_URL":         "http://d",
		"PUBLIC_DATA_DIR":               "./p",
		"PRIVATE_DATA_DIR":              "./pr",
		"IMAGE_DIR":                     "./i",
		"UID":                           "1000",
		"GID":                           "1000",
		"EMOBASE_INFRA_NETWORK":         "emobase-infra",
		"MYSQL_ROOT_PASSWORD":           "root-secret",
		"KEYCLOAK_PUBLIC_URL":           "http://localhost:8081/keycloak",
		"KEYCLOAK_INTERNAL_URL":         "http://keycloak:8080/keycloak",
		"KEYCLOAK_REALM":                "ibb",
		"KEYCLOAK_CLIENT_ID":            "ibb-client",
		"KEYCLOAK_CLIENT_SECRET":        "client-secret",
	})
	ws := discover(t, root, envs)
	rep := Run(ws)
	if find(rep, "unique.http_port") == nil {
		t.Fatal("expected unique.http_port")
	}
	if find(rep, "unique.directus_db") == nil {
		t.Fatal("expected unique.directus_db")
	}
}

func TestDedicatedRealmNeedsClient(t *testing.T) {
	root, envs := fixture(t)
	envs["emobase-deploy-tcas"]["KEYCLOAK_REALM"] = "ibb_tcas"
	envs["emobase-deploy-tcas"]["INTERNAL_KEYCLOAK_ISSUER"] = "http://host.docker.internal:8081/keycloak/realms/ibb_tcas"
	delete(envs["emobase-deploy-tcas"], "KEYCLOAK_CLIENT_ID")
	delete(envs["emobase-deploy-tcas"], "KEYCLOAK_CLIENT_SECRET")
	ws := discover(t, root, envs)

	rep := Run(ws)
	if find(rep, "realm.client") == nil {
		t.Fatal("expected realm.client")
	}
	// A dedicated realm must not be forced to match infra realm values.
	if find(rep, "shared.KEYCLOAK_CLIENT_ID") != nil {
		t.Fatal("dedicated realm should own its client")
	}
	if find(rep, "issuer.INTERNAL_KEYCLOAK_ISSUER") != nil {
		t.Fatal("matching issuer reported")
	}
}

func TestIssuerMismatchWarns(t *testing.T) {
	root, envs := fixture(t)
	envs["emobase-deploy-tcas"]["INTERNAL_KEYCLOAK_ISSUER"] = "http://host.docker.internal:8081/keycloak/realms/other"
	ws := discover(t, root, envs)
	f := find(Run(ws), "issuer.INTERNAL_KEYCLOAK_ISSUER")
	if f == nil || f.Severity != Warn {
		t.Fatalf("finding = %#v", f)
	}
}

func TestMissingEnvIsReportedOnce(t *testing.T) {
	root, envs := fixture(t)
	delete(envs, "emobase-portal-deploy")
	os.Remove(filepath.Join(root, "emobase-portal-deploy", ".env"))
	ws, err := workspace.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	rep := Run(ws)
	count := 0
	for _, f := range rep.Findings {
		if f.Check == "env.missing" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("env.missing count = %d", count)
	}
	if !rep.HasErrors() {
		t.Fatal("missing .env must be an error")
	}
}

func TestLocalKeycloakPortMismatch(t *testing.T) {
	root, envs := fixture(t)
	for _, name := range []string{"emobase-infra", "emobase-portal-deploy", "emobase-deploy-tcas"} {
		envs[name]["KEYCLOAK_PUBLIC_URL"] = "http://localhost:8082/keycloak"
	}
	ws := discover(t, root, envs)
	f := find(Run(ws), "local.keycloak_port")
	if f == nil || f.Severity != Warn {
		t.Fatalf("finding = %#v", f)
	}
}

func TestInternalLoopbackWarns(t *testing.T) {
	root, envs := fixture(t)
	envs["emobase-deploy-tcas"]["INTERNAL_DIRECTUS_URL"] = "http://localhost:8055"
	ws := discover(t, root, envs)
	f := find(Run(ws), "internal.loopback.INTERNAL_DIRECTUS_URL")
	if f == nil || f.Severity != Warn {
		t.Fatalf("finding = %#v", f)
	}
}

func TestChangeJSONMasksSecrets(t *testing.T) {
	c := Change{Scope: "portal", Key: "MYSQL_ROOT_PASSWORD", Old: "old-secret", New: "new-secret", Secret: true}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "secret") {
		t.Fatalf("secret leaked: %s", b)
	}
	if !strings.Contains(string(b), "********") {
		t.Fatalf("mask missing: %s", b)
	}

	plain := Change{Scope: "infra", Key: "KEYCLOAK_PUBLIC_URL", Old: "a/", New: "a"}
	b, err = json.Marshal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"new":"a"`) {
		t.Fatalf("non-secret change was masked: %s", b)
	}
}

func TestReportSortPutsErrorsFirst(t *testing.T) {
	rep := &Report{Findings: []Finding{
		{Check: "b", Severity: Info},
		{Check: "a", Severity: Error},
		{Check: "c", Severity: Warn},
	}}
	rep.Sort()
	if rep.Findings[0].Severity != Error || rep.Findings[2].Severity != Info {
		t.Fatalf("order = %v", rep.Findings)
	}
}
