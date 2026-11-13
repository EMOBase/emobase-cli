package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/EMOBase/emobase-cli/internal/envfile"
	"github.com/EMOBase/emobase-cli/internal/scaffold"
	"github.com/EMOBase/emobase-cli/internal/workspace"
)

func realmStack(role workspace.Role, dir, realm, client, secret, origin string) *workspace.Stack {
	env := envfile.Parse(dir+"/.env", strings.Join([]string{
		"KEYCLOAK_REALM=" + realm,
		"KEYCLOAK_CLIENT_ID=" + client,
		"KEYCLOAK_CLIENT_SECRET=" + secret,
		"COCO_USERNAME=coco",
		"COCO_PASSWORD=coco-secret",
		"PUBLIC_SERVER=" + origin,
		"",
	}, "\n"))
	return &workspace.Stack{Role: role, Name: dir, Dir: dir, Env: env, EnvPath: dir + "/.env"}
}

func TestStackRealm(t *testing.T) {
	portal := realmStack(workspace.RolePortal, "/srv/emobase/emobase-portal-deploy",
		"emobase-portal", "emobase-portal-client", "portal-secret", "https://portal.example.org")
	realm, client, secret, user, pass, ok := stackRealm(portal)
	if !ok || realm != "emobase-portal" || client != "emobase-portal-client" || secret != "portal-secret" || user != "coco" || pass != "coco-secret" {
		t.Fatalf("stackRealm = %q %q %q %q %q %v", realm, client, secret, user, pass, ok)
	}
	if _, _, _, _, _, ok := stackRealm(nil); ok {
		t.Fatal("nil stack must not have realm credentials")
	}
}

func TestEnsureStackRealmDryRun(t *testing.T) {
	infra := realmStack(workspace.RoleInfra, "/srv/emobase/emobase-infra", "", "", "", "")
	portal := realmStack(workspace.RolePortal, "/srv/emobase/emobase-portal-deploy",
		"emobase-portal", "emobase-portal-client", "portal-secret", "https://portal.example.org")
	ws := &workspace.Workspace{Infra: infra, Portal: portal}
	var out bytes.Buffer
	r := &scaffold.Runner{DryRun: true, Stdout: &out, Stderr: &out}

	if err := ensureStackRealm(t.Context(), r, ws, portal); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"./scripts/create-realm.sh",
		"--realm emobase-portal",
		"--origin https://portal.example.org",
		"--client-id emobase-portal-client",
	} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Errorf("out missing %q:\n%s", want, out.String())
		}
	}
}
