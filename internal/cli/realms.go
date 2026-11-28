package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/EMOBase/emobase-cli/internal/live"
	"github.com/EMOBase/emobase-cli/internal/scaffold"
	"github.com/EMOBase/emobase-cli/internal/workspace"
)

// infraServiceRunning reports whether one infra Compose service is up.
func infraServiceRunning(ctx context.Context, ws *workspace.Workspace, service string) bool {
	if ws.Infra == nil || ws.Infra.MissingEnv || !live.DockerAvailable(ctx) {
		return false
	}
	containers, err := live.ComposePS(ctx, ws.Infra.Dir)
	if err != nil {
		return false
	}
	for _, c := range containers {
		if c.Service != service {
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

// keycloakRunning reports whether the portal Keycloak container is up. Realm
// creation runs inside that container, so it cannot start before the portal.
func keycloakRunning(ctx context.Context, ws *workspace.Workspace) bool {
	if ws.Portal == nil || ws.Portal.MissingEnv {
		return false
	}
	return stackServiceRunning(ctx, ws.Portal, "keycloak")
}

// stackServiceRunning reports whether one Compose service in a stack is up.
func stackServiceRunning(ctx context.Context, st *workspace.Stack, service string) bool {
	if st == nil || st.MissingEnv || !live.DockerAvailable(ctx) {
		return false
	}
	containers, err := live.ComposePS(ctx, st.Dir)
	if err != nil {
		return false
	}
	for _, c := range containers {
		if c.Service != service {
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

// infraReady reports whether the shared storage needed by species setup is up.
func infraReady(ctx context.Context, ws *workspace.Workspace) bool {
	return infraServiceRunning(ctx, ws, "mysql")
}

// stackRealm returns the realm and client credentials owned by a stack.
func stackRealm(st *workspace.Stack) (realm, clientID, clientSecret, cocoUser, cocoPass string, ok bool) {
	if st == nil || st.MissingEnv {
		return "", "", "", "", "", false
	}
	realm, _ = st.Get("KEYCLOAK_REALM")
	clientID, _ = st.Get("KEYCLOAK_CLIENT_ID")
	clientSecret, _ = st.Get("KEYCLOAK_CLIENT_SECRET")
	cocoUser, _ = st.Get("COCO_USERNAME")
	cocoPass, _ = st.Get("COCO_PASSWORD")
	if strings.TrimSpace(realm) == "" || strings.TrimSpace(clientID) == "" || strings.TrimSpace(clientSecret) == "" {
		return "", "", "", "", "", false
	}
	if cocoUser == "" {
		cocoUser = "coco"
	}
	return realm, clientID, clientSecret, cocoUser, cocoPass, true
}

// ensurePortalDatabase creates the portal's MySQL database. Infra no longer
// creates application databases.
func ensurePortalDatabase(r *scaffold.Runner, ws *workspace.Workspace, st *workspace.Stack) error {
	if ws.Infra == nil {
		return fmt.Errorf("no emobase-infra checkout found")
	}
	db, _ := st.Get("EMOBASE_GENOMICS_MYSQL__DATABASE")
	if strings.TrimSpace(db) == "" {
		return fmt.Errorf("%s has no EMOBASE_GENOMICS_MYSQL__DATABASE", st.Name)
	}
	return r.CreateDatabase(ws.Infra.Dir, db)
}

// ensureStackRealm creates or updates the stack's own realm and origin. The
// operation is idempotent and never rotates an existing seed password.
func ensureStackRealm(ctx context.Context, r *scaffold.Runner, ws *workspace.Workspace, st *workspace.Stack) error {
	if st == nil || st.Role == workspace.RoleInfra {
		return nil
	}
	if st.Role != workspace.RolePortal {
		// Species realms are created on the portal host; a species stack only
		// consumes the credentials it was given.
		return nil
	}
	if ws.Portal == nil || ws.Portal.MissingEnv {
		return fmt.Errorf("no portal checkout found; create the realm on the portal host")
	}
	realm, clientID, clientSecret, cocoUser, cocoPass, ok := stackRealm(st)
	if !ok {
		return fmt.Errorf("%s has no realm client credentials; run its init.sh first", st.Name)
	}
	origin, _ := st.Get("PUBLIC_SERVER")
	if strings.TrimSpace(origin) == "" {
		return fmt.Errorf("%s has no PUBLIC_SERVER", st.Name)
	}
	if !r.DryRun && !keycloakRunning(ctx, ws) {
		return fmt.Errorf("Keycloak is not running; start infra first with: emobase up --only infra")
	}
	return r.EnsureRealm(ws.Portal.Dir, realm, origin, clientID, clientSecret, cocoUser, cocoPass)
}
