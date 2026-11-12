package cli

import (
	"path/filepath"

	"github.com/EMOBase/emobase-cli/internal/checks"
	"github.com/EMOBase/emobase-cli/internal/envfile"
	"github.com/EMOBase/emobase-cli/internal/workspace"
)

const previewRoot = "/srv/emobase"

const infraEnv = `KEYCLOAK_REALM=ibb
KEYCLOAK_CLIENT_ID=emobase-portal
KEYCLOAK_CLIENT_SECRET=infra-secret
MYSQL_ROOT_PASSWORD=root-secret
KEYCLOAK_PUBLIC_URL=https://auth.example.org/keycloak
KEYCLOAK_INTERNAL_URL=http://keycloak:8080/keycloak
KEYCLOAK_HOST_PORT=8081
KEYCLOAK_ADMIN=admin
KEYCLOAK_ADMIN_PASSWORD=admin-secret
COCO_USERNAME=coco
COCO_PASSWORD=coco-secret
EMOBASE_INFRA_NETWORK=emobase-infra
PORTAL_PUBLIC_SERVER=https://portal.example.org
`

const portalEnv = `PUBLIC_SERVER=https://portal.example.org
HTTP_PORT=9080
PUBLIC_DATA_DIR=/srv/emobase/data
EMOBASE_GENOMICS_MYSQL__HOST=mysql
EMOBASE_GENOMICS_MYSQL__PORT=3306
EMOBASE_GENOMICS_MYSQL__USER=genomics
EMOBASE_GENOMICS_MYSQL__DATABASE=emobase_genomics
AUTH_SECRET=portal-auth-secret
EMOBASE_GENOMICS_IMAGE=ghcr.io/emobase/genomics:latest
EMOBASE_PORTAL_WEB_IMAGE=ghcr.io/emobase/portal-web:latest
KEYCLOAK_REALM=ibb
KEYCLOAK_CLIENT_ID=emobase-portal
KEYCLOAK_CLIENT_SECRET=infra-secret
MYSQL_ROOT_PASSWORD=root-secret
KEYCLOAK_PUBLIC_URL=https://auth.example.org/keycloak
KEYCLOAK_INTERNAL_URL=http://keycloak:8080/keycloak
EMOBASE_INFRA_NETWORK=emobase-infra
`

const tcasEnv = `MAIN_SPECIES=Tcas
PUBLIC_SERVER=https://tcas.example.org
HTTP_PORT=9090
PORTAL_SERVER=https://portal.example.org
PORTAL_INTERNAL_URL=http://portal:3000
DIRECTUS_DB_DATABASE=directus_tcas
DIRECTUS_SECRET=directus-tcas-secret
DIRECTUS_ADMIN_EMAIL=admin@example.org
DIRECTUS_ADMIN_PASSWORD=directus-admin-secret
COCO_USERNAME=coco
COCO_PASSWORD=coco-secret
EMOBASE_WEB_IMAGE=ghcr.io/emobase/web:latest
SSR_OIDC_SESSION_SECRET=ssr-tcas-secret
INTERNAL_API_GENOMICS_SERVICE=http://genomicsservice:8000
INTERNAL_KEYCLOAK_ISSUER=http://keycloak:8080/keycloak/realms/ibb_tcas
INTERNAL_DIRECTUS_URL=http://directus:8055
PUBLIC_DATA_DIR=/srv/emobase/tcas/public
PRIVATE_DATA_DIR=/srv/emobase/tcas/private
IMAGE_DIR=/srv/emobase/tcas/images
UID=1000
GID=1000
KEYCLOAK_REALM=ibb_tcas
KEYCLOAK_CLIENT_ID=emobase-tcas
KEYCLOAK_CLIENT_SECRET=tcas-secret
MYSQL_ROOT_PASSWORD=root-secret
KEYCLOAK_PUBLIC_URL=https://auth.example.org/keycloak
KEYCLOAK_INTERNAL_URL=http://keycloak:8080/keycloak
EMOBASE_INFRA_NETWORK=emobase-infra
COMPOSE_PROJECT_NAME=tcas
`

func previewStack(role workspace.Role, name, env, project string) *workspace.Stack {
	dir := filepath.Join(previewRoot, name)
	return &workspace.Stack{
		Role:        role,
		Dir:         dir,
		Name:        name,
		EnvPath:     filepath.Join(dir, ".env"),
		Env:         envfile.Parse(filepath.Join(dir, ".env"), env),
		ProjectName: project,
	}
}

func healthyWorkspace() *workspace.Workspace {
	return &workspace.Workspace{
		Root:   previewRoot,
		Infra:  previewStack(workspace.RoleInfra, "emobase-infra", infraEnv, "emobase"),
		Portal: previewStack(workspace.RolePortal, "emobase-portal-deploy", portalEnv, "emobase"),
		Species: []*workspace.Stack{
			previewStack(workspace.RoleSpecies, "emobase-deploy-tcas", tcasEnv, "tcas"),
		},
	}
}

func findingsWorkspace() *workspace.Workspace {
	ws := healthyWorkspace()
	infra := ws.Infra
	infra.Env.Set("KEYCLOAK_ADMIN_PASSWORD", "")
	ws.Portal.Env.Set("MYSQL_ROOT_PASSWORD", "wrong-secret")

	tcas := ws.Species[0]
	tcas.Env.Set("COMPOSE_PROJECT_NAME", "")
	tcas.Env.Set("PORTAL_SERVER", "")
	tcas.Env.Set("PUBLIC_SERVER", "https://tcas.example.org/")
	tcas.Env.Set("INTERNAL_DIRECTUS_URL", "http://localhost:8055")
	tcas.Env.Set("DIRECTUS_ADMIN_PASSWORD", "change-me")
	return ws
}

func conflictWorkspace() *workspace.Workspace {
	ws := healthyWorkspace()
	ws.Infra.Env.Set("PORTAL_PUBLIC_SERVER", "https://old-portal.example.org")

	dmel := previewStack(workspace.RoleSpecies, "emobase-deploy-dmel", tcasEnv, "tcas")
	dmel.Env.Set("MAIN_SPECIES", "Dmel")
	dmel.Env.Set("PUBLIC_SERVER", "https://dmel.example.org")
	dmel.Env.Set("KEYCLOAK_REALM", "ibb_dmel")
	dmel.Env.Set("KEYCLOAK_CLIENT_ID", "emobase-dmel")
	dmel.Env.Set("KEYCLOAK_CLIENT_SECRET", "dmel-secret")
	dmel.Env.Set("INTERNAL_KEYCLOAK_ISSUER", "http://keycloak:8080/keycloak/realms/ibb_dmel")
	dmel.Env.Set("DIRECTUS_DB_DATABASE", "directus_tcas")
	dmel.Env.Set("HTTP_PORT", "9090")
	dmel.Env.Set("COMPOSE_PROJECT_NAME", "tcas")
	ws.Species = append(ws.Species, dmel)
	return ws
}

func previewReport(ws *workspace.Workspace) *checks.Report {
	rep := checks.Run(ws)
	rep.Sort()
	return rep
}

func noticesReport() *checks.Report {
	return &checks.Report{
		Root: previewRoot,
		Findings: []checks.Finding{
			checks.Advisory("live.portal", "portal", checks.Info, "portal returned HTTP 204", ""),
			checks.Advisory("live.redirect", "species:emobase-deploy-tcas", checks.Info, "redirect URI was not exercised", ""),
		},
	}
}

func statusRunning() statusReport {
	return statusReport{
		Workspace: previewRoot,
		Docker:    true,
		Network:   "emobase-infra",
		NetworkOK: true,
		Databases: []string{"directus_tcas", "emobase_genomics"},
		Realms:    map[string]bool{"ibb": true, "ibb_tcas": true},
		Portal:    "HTTP 200",
		Stacks: []statusStack{
			{Role: "infra", Name: "emobase-infra", Running: 3, Total: 3, URL: "https://auth.example.org/keycloak"},
			{Role: "portal", Name: "emobase-portal-deploy", Running: 5, Total: 5, URL: "https://portal.example.org"},
			{Role: "species", Name: "emobase-deploy-tcas", Running: 4, Total: 4, URL: "https://tcas.example.org"},
		},
	}
}

func statusPartial() statusReport {
	sr := statusRunning()
	sr.NetworkOK = false
	sr.Realms = map[string]bool{"ibb": true, "ibb_tcas": false}
	sr.Portal = "unreachable"
	sr.Stacks[1].Running, sr.Stacks[1].Unhealthy = 4, 1
	sr.Stacks[2].Running, sr.Stacks[2].Total = 0, 0
	sr.Stacks[2].URL = ""
	return sr
}

func statusDockerDown() statusReport {
	return statusReport{
		Workspace: previewRoot,
		Stacks: []statusStack{
			{Role: "infra", Name: "emobase-infra", Detail: "docker unavailable"},
			{Role: "portal", Name: "emobase-portal-deploy", Detail: "docker unavailable"},
			{Role: "species", Name: "emobase-deploy-tcas", Detail: "docker unavailable"},
		},
	}
}

func previewCases() []previewCase {
	matrix := func(ws *workspace.Workspace) string {
		headers, rows := matrixRows(ws)
		return renderMatrix(headers, rows)
	}
	return []previewCase{
		{Name: "dashboard-healthy", Group: "dashboard", Description: "3 stacks, no findings", Render: func() string {
			ws := healthyWorkspace()
			return renderDashboard(ws, previewReport(ws))
		}},
		{Name: "dashboard-findings", Group: "dashboard", Description: "mixed severities, hints and fix classes", Render: func() string {
			ws := findingsWorkspace()
			return renderDashboard(ws, previewReport(ws))
		}},
		{Name: "dashboard-empty", Group: "dashboard", Description: "no checkouts discovered", Render: func() string {
			ws := &workspace.Workspace{Root: previewRoot}
			return renderDashboard(ws, previewReport(ws))
		}},

		{Name: "doctor-clean", Group: "doctor", Description: "consistent configuration", Render: func() string {
			ws := healthyWorkspace()
			return renderDoctor(previewReport(ws), nil, false, false)
		}},
		{Name: "doctor-findings", Group: "doctor", Description: "safe, warning and missing-key findings", Render: func() string {
			ws := findingsWorkspace()
			return renderDoctor(previewReport(ws), nil, false, false)
		}},
		{Name: "doctor-conflicts", Group: "doctor", Description: "choose and destructive findings", Render: func() string {
			ws := conflictWorkspace()
			return renderDoctor(previewReport(ws), nil, false, false)
		}},
		{Name: "doctor-notices", Group: "doctor", Description: "informational findings only", Render: func() string {
			return renderDoctor(noticesReport(), nil, false, false)
		}},
		{Name: "doctor-fix-applied", Group: "doctor", Description: "applied fixes with masked secrets", Render: func() string {
			ws := healthyWorkspace()
			applied := []checks.Change{
				{Scope: "species:emobase-deploy-tcas", Key: "COMPOSE_PROJECT_NAME", Old: "", New: "tcas"},
				{Scope: "species:emobase-deploy-tcas", Key: "PORTAL_SERVER", Old: "", New: "https://portal.example.org"},
				{Scope: "species:emobase-deploy-tcas", Key: "DIRECTUS_SECRET", Old: "", New: "new-secret", Secret: true},
				{Scope: "portal", Key: "MYSQL_ROOT_PASSWORD", Old: "wrong-secret", New: "root-secret", Secret: true},
			}
			return renderDoctor(previewReport(ws), applied, true, true)
		}},

		{Name: "matrix-healthy", Group: "matrix", Description: "3 stacks with masked secrets", Render: func() string {
			return matrix(healthyWorkspace())
		}},
		{Name: "matrix-missing", Group: "matrix", Description: "absent values shown as dashes", Render: func() string {
			return matrix(findingsWorkspace())
		}},
		{Name: "matrix-conflicts", Group: "matrix", Description: "4 stacks with duplicated values", Render: func() string {
			return matrix(conflictWorkspace())
		}},

		{Name: "species-list", Group: "species", Description: "two species checkouts", Render: func() string {
			return renderSpecies(speciesRows(conflictWorkspace()))
		}},
		{Name: "species-empty", Group: "species", Description: "no species found", Render: func() string {
			return renderSpecies(nil)
		}},

		{Name: "status-running", Group: "status", Description: "all stacks healthy", Render: func() string {
			return renderStatus(statusRunning())
		}},
		{Name: "status-partial", Group: "status", Description: "unhealthy, unreachable and not created", Render: func() string {
			return renderStatus(statusPartial())
		}},
		{Name: "status-docker-down", Group: "status", Description: "Docker daemon unavailable", Render: func() string {
			return renderStatus(statusDockerDown())
		}},
	}
}
