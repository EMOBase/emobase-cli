package cli

import (
	"bytes"
	"testing"

	"github.com/EMOBase/emobase-cli/internal/envfile"
	"github.com/EMOBase/emobase-cli/internal/scaffold"
	"github.com/EMOBase/emobase-cli/internal/workspace"
)

func realmStack(role workspace.Role, dir, realm, origin string) *workspace.Stack {
	env := envfile.Parse(dir+"/.env", "KEYCLOAK_REALM="+realm+"\nPUBLIC_SERVER="+origin+"\n")
	return &workspace.Stack{Role: role, Name: dir, Dir: dir, Env: env, EnvPath: dir + "/.env"}
}

func TestSharedRealmOrigin(t *testing.T) {
	infra := realmStack(workspace.RoleInfra, "/srv/emobase/emobase-infra", "ibb", "https://portal.example.org")
	portal := realmStack(workspace.RolePortal, "/srv/emobase/emobase-portal-deploy", "ibb", "https://portal.example.org")
	shared := realmStack(workspace.RoleSpecies, "/srv/emobase/emobase-deploy-tcas", "ibb", "https://tcas.example.org")
	dedicated := realmStack(workspace.RoleSpecies, "/srv/emobase/emobase-deploy-dmel", "ibb_dmel", "https://dmel.example.org")
	ws := &workspace.Workspace{Root: "/srv/emobase", Infra: infra, Portal: portal, Species: []*workspace.Stack{shared, dedicated}}

	if origin, ok := sharedRealmOrigin(ws, portal); !ok || origin != "https://portal.example.org" {
		t.Fatalf("portal origin = %q, %v", origin, ok)
	}
	if origin, ok := sharedRealmOrigin(ws, shared); !ok || origin != "https://tcas.example.org" {
		t.Fatalf("shared species origin = %q, %v", origin, ok)
	}
	if _, ok := sharedRealmOrigin(ws, dedicated); ok {
		t.Fatal("dedicated realm must not use the shared realm")
	}
}

func TestEnsureStackRealmDryRun(t *testing.T) {
	infra := realmStack(workspace.RoleInfra, "/srv/emobase/emobase-infra", "ibb", "")
	portal := realmStack(workspace.RolePortal, "/srv/emobase/emobase-portal-deploy", "ibb", "https://portal.example.org")
	ws := &workspace.Workspace{Infra: infra, Portal: portal}
	var out bytes.Buffer
	r := &scaffold.Runner{DryRun: true, Stdout: &out, Stderr: &out}

	if err := ensureStackRealm(t.Context(), r, ws, portal); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("./scripts/ensure-shared-realm.sh https://portal.example.org")) {
		t.Fatalf("out = %q", out.String())
	}
}
