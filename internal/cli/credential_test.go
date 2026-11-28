package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	cmd := New()
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		// The app silences Cobra's own error output; surface the error text so
		// tests can assert on it.
		if errb.Len() == 0 {
			fmt.Fprint(&errb, err.Error())
		}
		return out.String(), errb.String(), 1
	}
	return out.String(), errb.String(), 0
}

// credentialFixture builds a workspace where the portal and one species share
// realm ibb with divergent COCO_PASSWORD copies, and a second species owns
// realm ibb_dmel. Realm ibb still has its temporary seed password; ibb_dmel's
// was replaced at first sign-in.
func credentialFixture(t *testing.T) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			fmt.Fprint(w, `{"access_token":"test-token","expires_in":300}`)
		case r.URL.Path == "/admin/realms/ibb/users":
			fmt.Fprint(w, `[{"id":"u-portal","username":"coco"}]`)
		case r.URL.Path == "/admin/realms/ibb/users/u-portal":
			fmt.Fprint(w, `{"id":"u-portal","username":"coco","requiredActions":["UPDATE_PASSWORD"]}`)
		case r.URL.Path == "/admin/realms/ibb_dmel/users":
			fmt.Fprint(w, `[{"id":"u-dmel","username":"coco"}]`)
		case r.URL.Path == "/admin/realms/ibb_dmel/users/u-dmel":
			fmt.Fprint(w, `{"id":"u-dmel","username":"coco","requiredActions":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	root := t.TempDir()
	mkCLIStack(t, root, "emobase-infra", testInfraCompose, strings.Join([]string{
		"EMOBASE_INFRA_NETWORK=emobase-infra",
		"MYSQL_ROOT_PASSWORD=root-secret",
		"",
	}, "\n"))
	mkCLIStack(t, root, "emobase-portal-deploy", testPortalCompose, strings.Join([]string{
		"PUBLIC_SERVER=http://localhost:9080",
		"EMOBASE_GENOMICS_MYSQL__DATABASE=emobase_genomics",
		"KEYCLOAK_PUBLIC_URL=" + srv.URL,
		"KEYCLOAK_INTERNAL_URL=http://keycloak:8080/keycloak",
		"KEYCLOAK_ADMIN=admin",
		"KEYCLOAK_ADMIN_PASSWORD=admin-pass",
		"KEYCLOAK_REALM=ibb",
		"KEYCLOAK_CLIENT_ID=ibb-client",
		"KEYCLOAK_CLIENT_SECRET=super-secret",
		"COCO_USERNAME=coco",
		"COCO_PASSWORD=portal-seed-123",
		"",
	}, "\n"))
	mkCLIStack(t, root, "emobase-deploy-tcas", testSpeciesCompose, strings.Join([]string{
		"MAIN_SPECIES=Tcas",
		"PUBLIC_SERVER=http://localhost:9090",
		"EMOBASE_ES_INDEX_PREFIX=tcas",
		"KEYCLOAK_REALM=ibb",
		"KEYCLOAK_CLIENT_ID=ibb-client",
		"KEYCLOAK_CLIENT_SECRET=super-secret",
		"COCO_USERNAME=coco",
		"COCO_PASSWORD=tcas-own-copy",
		"",
	}, "\n"))
	mkCLIStack(t, root, "emobase-deploy-dmel", testSpeciesCompose, strings.Join([]string{
		"MAIN_SPECIES=Dmel",
		"PUBLIC_SERVER=http://localhost:9091",
		"EMOBASE_ES_INDEX_PREFIX=dmel",
		"KEYCLOAK_REALM=ibb_dmel",
		"KEYCLOAK_CLIENT_ID=ibb-dmel-client",
		"KEYCLOAK_CLIENT_SECRET=dmel-secret",
		"COCO_USERNAME=coco",
		"COCO_PASSWORD=dmel-seed-456",
		"",
	}, "\n"))
	return root
}

func TestCredentialListHidesPasswords(t *testing.T) {
	root := credentialFixture(t)
	out, errb, code := runCLI(t, "--workspace", root, "--no-color", "credential", "show")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb)
	}
	for _, secret := range []string{"portal-seed-123", "dmel-seed-456", "tcas-own-copy", "admin-pass"} {
		if strings.Contains(out, secret) {
			t.Errorf("fleet view leaked %q:\n%s", secret, out)
		}
	}
	for _, want := range []string{"coco", "ibb", "ibb_dmel", "Tcas", "portal", "keycloak", "admin", "state"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	// Only the realm with divergent copies gets a note, and it never picks an
	// owner: several checkouts simply share the realm.
	if !strings.Contains(out, "checkouts sharing realm ibb store different COCO_PASSWORD values") {
		t.Errorf("expected a divergence note for realm ibb:\n%s", out)
	}
	if strings.Contains(out, "realm ibb_dmel store different") {
		t.Errorf("realm ibb_dmel has one checkout and must not be flagged:\n%s", out)
	}
	if strings.Contains(out, "joined") || strings.Contains(out, "authoritative") {
		t.Errorf("output must not model an owner:\n%s", out)
	}
}

func TestCredentialNamedStackPrintsItsOwnCopy(t *testing.T) {
	root := credentialFixture(t)
	out, errb, code := runCLI(t, "--workspace", root, "--no-color", "credential", "show", "tcas")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb)
	}
	if !strings.Contains(out, "tcas-own-copy") {
		t.Errorf("expected this checkout's own value:\n%s", out)
	}
	if strings.Contains(out, "portal-seed-123") {
		t.Errorf("must not substitute another checkout's copy:\n%s", out)
	}
	// The detail view is deliberately two lines: no stack, realm, source or
	// divergence commentary.
	for _, noise := range []string{"stack:", "realm:", "source:", "note:"} {
		if strings.Contains(out, noise) {
			t.Errorf("unexpected %q in the detail view:\n%s", noise, out)
		}
	}
	if lines := strings.Count(strings.TrimSpace(out), "\n") + 1; lines > 2 {
		t.Errorf("expected at most two lines, got %d:\n%s", lines, out)
	}
}

func TestCredentialChangedStateShowsStoredPassword(t *testing.T) {
	root := credentialFixture(t)
	out, errb, code := runCLI(t, "--workspace", root, "--no-color", "credential", "show", "dmel")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb)
	}
	if !strings.Contains(out, "coco") {
		t.Errorf("expected the username:\n%s", out)
	}
	// The stored copy stays visible for reference, but the state is unambiguous.
	for _, want := range []string{"password: dmel-seed-456 (seed)", "already changed at first sign-in"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	// The reset instruction must name the console, the realm and the user.
	for _, want := range []string{"Reset it at", "admin/", "realm ibb_dmel", "Users → coco → Credentials"} {
		if !strings.Contains(out, want) {
			t.Errorf("reset hint is missing %q:\n%s", want, out)
		}
	}
}

func TestCredentialJSONOmitsPasswordForFleet(t *testing.T) {
	root := credentialFixture(t)
	out, errb, code := runCLI(t, "--workspace", root, "--no-color", "--json", "credential", "show")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb)
	}
	if strings.Contains(out, "\"password\"") {
		t.Errorf("fleet JSON must not contain a password field:\n%s", out)
	}
	if !strings.Contains(out, "\"stack\"") {
		t.Errorf("expected credential rows:\n%s", out)
	}
}

// TestCredentialStackRefForms covers the accepted ways to name a stack: the
// fleet table label, the checkout name, the species code and (case-insensitive)
// the MAIN_SPECIES value.
func TestCredentialStackRefForms(t *testing.T) {
	root := credentialFixture(t)
	for _, ref := range []string{
		"species:emobase-deploy-tcas",
		"emobase-deploy-tcas",
		"tcas",
		"Tcas",
	} {
		t.Run(ref, func(t *testing.T) {
			out, errb, code := runCLI(t, "--workspace", root, "--no-color", "credential", "show", ref)
			if code != 0 {
				t.Fatalf("exit = %d, stderr = %s", code, errb)
			}
			if !strings.Contains(out, "tcas-own-copy") {
				t.Errorf("ref %q did not resolve to the tcas checkout:\n%s", ref, out)
			}
		})
	}
}

func TestCredentialUnknownStackFails(t *testing.T) {
	root := credentialFixture(t)
	_, errb, code := runCLI(t, "--workspace", root, "--no-color", "credential", "show", "nope")
	if code == 0 {
		t.Fatal("expected a non-zero exit for an unknown stack")
	}
	if !strings.Contains(errb, "no stack matches") {
		t.Fatalf("stderr = %s", errb)
	}
}

func TestCredentialKeycloakPrintsAdminLogin(t *testing.T) {
	root := credentialFixture(t)
	for _, ref := range []string{"keycloak", "infra"} {
		t.Run(ref, func(t *testing.T) {
			out, errb, code := runCLI(t, "--workspace", root, "--no-color", "credential", "show", ref)
			if code != 0 {
				t.Fatalf("exit = %d, stderr = %s", code, errb)
			}
			if !strings.Contains(out, "admin") || !strings.Contains(out, "admin-pass") {
				t.Errorf("expected the Keycloak admin login:\n%s", out)
			}
			if strings.Contains(out, "did not accept") {
				t.Errorf("working admin credentials must not warn:\n%s", out)
			}
		})
	}
}

func TestCredentialInfraReportsRejectedAdminLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	root := t.TempDir()
	mkCLIStack(t, root, "emobase-infra", testInfraCompose, strings.Join([]string{
		"EMOBASE_INFRA_NETWORK=emobase-infra",
		"MYSQL_ROOT_PASSWORD=root-secret",
		"",
	}, "\n"))
	mkCLIStack(t, root, "emobase-portal-deploy", testPortalCompose, strings.Join([]string{
		"KEYCLOAK_PUBLIC_URL=" + srv.URL,
		"KEYCLOAK_ADMIN=admin",
		"KEYCLOAK_ADMIN_PASSWORD=stale-admin-pass",
		"",
	}, "\n"))

	out, errb, code := runCLI(t, "--workspace", root, "--no-color", "credential", "show", "infra")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb)
	}
	if !strings.Contains(out, "stale-admin-pass") {
		t.Errorf("a named stack still prints the stored value:\n%s", out)
	}
	if !strings.Contains(out, "did not accept") {
		t.Errorf("expected a rejection notice:\n%s", out)
	}
}

// TestCredentialChangedSeedShowsCopyWithoutDivergenceWarning pins both rules:
// the detail view keeps the stored copy for reference, but the fleet view does
// not treat a post-first-sign-in copy as a live candidate.
func TestCredentialChangedSeedShowsCopyWithoutDivergenceWarning(t *testing.T) {
	row := credentialRow{
		Stack: "species:emobase-deploy-lsta", Realm: "emobase-portal", User: "coco",
		Seed: string(stateChanged), Source: "/srv/emobase/emobase-deploy-lsta",
		CopyDiffers: true, Password: "dead-seed",
	}
	if out := renderCredentialDetail(row, "https://auth.example.org/keycloak/admin/"); !strings.Contains(out, "dead-seed") {
		t.Errorf("a changed seed should still show the stored copy:\n%s", out)
	}
	if out := renderCredentialList([]credentialRow{row}); strings.Contains(out, "store different COCO_PASSWORD") {
		t.Errorf("changed seed must not carry a divergence note:\n%s", out)
	}
}

// TestCredentialJSONDetailIncludesStoredPassword pins the same rule for --json:
// a completed first sign-in keeps the stored copy and its state, because the
// user may have reused the same password.
func TestCredentialJSONDetailIncludesStoredPassword(t *testing.T) {
	root := credentialFixture(t)

	out, errb, code := runCLI(t, "--workspace", root, "--no-color", "--json", "credential", "show", "tcas")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb)
	}
	if !strings.Contains(out, "tcas-own-copy") {
		t.Errorf("unused seed should expose the password in JSON:\n%s", out)
	}

	out, errb, code = runCLI(t, "--workspace", root, "--no-color", "--json", "credential", "show", "dmel")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb)
	}
	if !strings.Contains(out, "dmel-seed-456") || !strings.Contains(out, "\"seed\": \"changed\"") {
		t.Errorf("changed seed should expose the stored password and its state:\n%s", out)
	}
}

// TestCredentialIdenticalCopiesAreNotFlagged is the counterpart of the
// divergence check: when a shared realm's copies agree, there is nothing to
// report and no owner to name.
func TestCredentialIdenticalCopiesAreNotFlagged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			fmt.Fprint(w, `{"access_token":"test-token","expires_in":300}`)
		case strings.HasSuffix(r.URL.Path, "/users"):
			fmt.Fprint(w, `[{"id":"u1","username":"coco"}]`)
		case strings.Contains(r.URL.Path, "/users/u1"):
			fmt.Fprint(w, `{"id":"u1","username":"coco","requiredActions":["UPDATE_PASSWORD"]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	root := t.TempDir()
	mkCLIStack(t, root, "emobase-infra", testInfraCompose, strings.Join([]string{
		"KEYCLOAK_PUBLIC_URL=" + srv.URL,
		"KEYCLOAK_ADMIN=admin",
		"KEYCLOAK_ADMIN_PASSWORD=admin-pass",
		"",
	}, "\n"))
	for _, name := range []string{"emobase-deploy-aaa", "emobase-deploy-zzz"} {
		mkCLIStack(t, root, name, testSpeciesCompose, strings.Join([]string{
			"KEYCLOAK_REALM=ibb_shared",
			"KEYCLOAK_CLIENT_ID=ibb-shared",
			"KEYCLOAK_CLIENT_SECRET=shared-secret",
			"COCO_USERNAME=coco",
			"COCO_PASSWORD=same-seed",
			"",
		}, "\n"))
	}

	out, errb, code := runCLI(t, "--workspace", root, "--no-color", "credential", "show")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errb)
	}
	if strings.Contains(out, "different COCO_PASSWORD") {
		t.Errorf("identical copies must not be flagged:\n%s", out)
	}
	if !strings.Contains(out, "ibb_shared") {
		t.Errorf("expected both stacks to be listed under the shared realm:\n%s", out)
	}
}
