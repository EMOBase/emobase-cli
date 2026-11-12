package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/EMOBase/emobase-cli/internal/live"
	"github.com/EMOBase/emobase-cli/internal/scaffold"
	"github.com/EMOBase/emobase-cli/internal/workspace"
)

// keycloakRunning reports whether the infra Keycloak container is up. Realm
// creation uses kcadm inside that container, so it cannot run before infra.
func keycloakRunning(ctx context.Context, ws *workspace.Workspace) bool {
	if ws.Infra == nil || ws.Infra.MissingEnv || !live.DockerAvailable(ctx) {
		return false
	}
	containers, err := live.ComposePS(ctx, ws.Infra.Dir)
	if err != nil {
		return false
	}
	for _, c := range containers {
		if c.Service != "keycloak" {
			continue
		}
		state := c.State
		if state == "" {
			state = c.Status
		}
		return isRunning(state, c.Status)
	}
	return false
}

// sharedRealmOrigin returns a stack's origin when it uses infra's shared
// realm. Dedicated realms are created by the species init.sh instead.
func sharedRealmOrigin(ws *workspace.Workspace, st *workspace.Stack) (string, bool) {
	if ws.Infra == nil || st == nil || st.MissingEnv {
		return "", false
	}
	infraRealm, _ := ws.Infra.Get("KEYCLOAK_REALM")
	if infraRealm == "" {
		infraRealm = "ibb"
	}
	realm, _ := st.Get("KEYCLOAK_REALM")
	origin, _ := st.Get("PUBLIC_SERVER")
	if realm != infraRealm || strings.TrimSpace(origin) == "" {
		return "", false
	}
	return origin, true
}

// ensureStackRealm creates the shared realm before a portal or shared-realm
// species is started. The operation is idempotent.
func ensureStackRealm(ctx context.Context, r *scaffold.Runner, ws *workspace.Workspace, st *workspace.Stack) error {
	if st == nil || st.Role == workspace.RoleInfra {
		return nil
	}
	origin, ok := sharedRealmOrigin(ws, st)
	if !ok {
		return nil
	}
	if !r.DryRun && !keycloakRunning(ctx, ws) {
		return fmt.Errorf("Keycloak is not running; start infra first with: emobase up --only infra")
	}
	return r.EnsureSharedRealm(ws.Infra.Dir, origin)
}
