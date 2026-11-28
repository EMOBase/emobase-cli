package cli

import (
	"path/filepath"

	"github.com/EMOBase/emobase-cli/internal/checks"
	"github.com/EMOBase/emobase-cli/internal/envfile"
	"github.com/EMOBase/emobase-cli/internal/workspace"
)

const previewRoot = "/srv/emobase"

const infraEnv = `MYSQL_ROOT_PASSWORD=root-secret
KEYCLOAK_PUBLIC_URL=https://auth.example.org/keycloak
KEYCLOAK_INTERNAL_URL=http://keycloak:8080/keycloak
KEYCLOAK_HOST_PORT=8181
KEYCLOAK_ADMIN=admin
KEYCLOAK_ADMIN_PASSWORD=admin-secret
EMOBASE_INFRA_NETWORK=emobase-infra
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
KEYCLOAK_REALM=emobase-portal
KEYCLOAK_CLIENT_ID=emobase-portal-client
KEYCLOAK_CLIENT_SECRET=portal-secret
COCO_USERNAME=coco
COCO_PASSWORD=coco-secret
MYSQL_ROOT_PASSWORD=root-secret
KEYCLOAK_PUBLIC_URL=https://auth.example.org/keycloak
KEYCLOAK_INTERNAL_URL=http://keycloak:8080/keycloak
EMOBASE_INFRA_NETWORK=emobase-infra
`

const tcasEnv = `MAIN_SPECIES=Tcas
PUBLIC_SERVER=https://tcas.example.org
PUBLIC_HOST=tcas.example.org
HTTP_PORT=9090
PORTAL_SERVER=https://portal.example.org
PORTAL_INTERNAL_URL=http://portal:3000
DIRECTUS_DB_DATABASE=directus_tcas
EMOBASE_ES_INDEX_PREFIX=tcas
DIRECTUS_SECRET=directus-tcas-secret
DIRECTUS_ADMIN_EMAIL=admin@example.org
DIRECTUS_ADMIN_PASSWORD=directus-admin-secret
COCO_USERNAME=coco
COCO_PASSWORD=coco-secret
EMOBASE_WEB_IMAGE=ghcr.io/emobase/web:latest
SSR_OIDC_SESSION_SECRET=ssr-tcas-secret
INTERNAL_DIRECTUS_URL=http://directus:8055
PUBLIC_DATA_DIR=/srv/emobase/tcas/public
PRIVATE_DATA_DIR=/srv/emobase/tcas/private
IMAGE_DIR=/srv/emobase/tcas/images
UID=1000
GID=1000
KEYCLOAK_REALM=emobase-portal
KEYCLOAK_CLIENT_ID=emobase-portal-client
KEYCLOAK_CLIENT_SECRET=portal-secret
MYSQL_ROOT_PASSWORD=root-secret
KEYCLOAK_PUBLIC_URL=https://auth.example.org/keycloak
KEYCLOAK_INTERNAL_URL=http://host.docker.internal:8181/keycloak
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

	dmel := previewStack(workspace.RoleSpecies, "emobase-deploy-dmel", tcasEnv, "tcas")
	dmel.Env.Set("MAIN_SPECIES", "Dmel")
	dmel.Env.Set("PUBLIC_SERVER", "https://dmel.example.org")
	dmel.Env.Set("KEYCLOAK_REALM", "emobase-dmel")
	dmel.Env.Set("KEYCLOAK_CLIENT_ID", "emobase-dmel-client")
	dmel.Env.Set("KEYCLOAK_CLIENT_SECRET", "dmel-secret")
	dmel.Env.Set("DIRECTUS_DB_DATABASE", "directus_tcas")
	dmel.Env.Set("EMOBASE_ES_INDEX_PREFIX", "dmel")
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

func previewImageRows(overrides, all bool) []imageRow {
	rows := []imageRow{
		{Target: "genomics", Variable: "EMOBASE_GENOMICS_IMAGE", Image: "ghcr.io/emobase/emobase-genomics:dev", Source: imageSourceDefault, Default: "ghcr.io/emobase/emobase-genomics:dev", Role: "portal", Checkout: "emobase-portal-deploy"},
		{Target: "portal-web", Variable: "EMOBASE_PORTAL_WEB_IMAGE", Image: "ghcr.io/emobase/emobase-portal-web:main", Source: imageSourceDefault, Default: "ghcr.io/emobase/emobase-portal-web:main", Role: "portal", Checkout: "emobase-portal-deploy"},
		{Target: "species-web", Variable: "EMOBASE_WEB_IMAGE", Image: "ghcr.io/emobase/emobase-web:dev", Source: imageSourceDefault, Default: "ghcr.io/emobase/emobase-web:dev", Role: "species", Checkout: "emobase-deploy-tcas"},
	}
	if overrides {
		rows[0].Image = "emobase-genomics:local"
		rows[0].Source = imageSourceEnv
		rows[2].Image = "emobase-web:local"
		rows[2].Source = imageSourceEnv
	}
	if !all {
		return rows
	}
	services := []imageRow{
		{Target: "ontologyservice", Variable: "ONTOLOGYSERVICE_IMAGE", Image: "ghcr.io/emobase/ibb-ontologyservice:v1.6.2", Source: imageSourceDefault, Default: "ghcr.io/emobase/ibb-ontologyservice:v1.6.2", Role: "species", Checkout: "emobase-deploy-tcas"},
		{Target: "imageservice", Variable: "IMAGESERVICE_IMAGE", Image: "ghcr.io/emobase/ibb-imageservice:v1.6.2", Source: imageSourceDefault, Default: "ghcr.io/emobase/ibb-imageservice:v1.6.2", Role: "species", Checkout: "emobase-deploy-tcas"},
		{Target: "phenotypeservice", Variable: "PHENOTYPESERVICE_IMAGE", Image: "ghcr.io/emobase/ibb-phenotypeservice:v1.6.1", Source: imageSourceDefault, Default: "ghcr.io/emobase/ibb-phenotypeservice:v1.6.1", Role: "species", Checkout: "emobase-deploy-tcas"},
		{Target: "goannotationservice", Variable: "GOANNOTATIONSERVICE_IMAGE", Image: "ghcr.io/emobase/ibb-goannotationservice:v1.6.1", Source: imageSourceDefault, Default: "ghcr.io/emobase/ibb-goannotationservice:v1.6.1", Role: "species", Checkout: "emobase-deploy-tcas"},
		{Target: "publicationservice", Variable: "PUBLICATIONSERVICE_IMAGE", Image: "ghcr.io/emobase/ibb-publicationservice:v1.6.1", Source: imageSourceDefault, Default: "ghcr.io/emobase/ibb-publicationservice:v1.6.1", Role: "species", Checkout: "emobase-deploy-tcas"},
		{Target: "oba", Variable: "OBA_IMAGE", Image: "jboss/base-jdk:7", Source: imageSourceDefault, Default: "jboss/base-jdk:7", Role: "species", Checkout: "emobase-deploy-tcas"},
	}
	if overrides {
		services[0].Image = "ghcr.io/emobase/ibb-ontologyservice:test"
		services[0].Source = imageSourceEnv
	}
	rows = append(rows, services...)
	// Illustrate the "-" state: an image override with no Compose fallback.
	rows = append(rows, imageRow{
		Target: "custom-service", Variable: "CUSTOM_SERVICE_IMAGE",
		Image: "registry.example/custom-service:local", Source: imageSourceEnv,
		Role: "species", Checkout: "emobase-deploy-tcas",
	})
	return rows
}

func previewCases() []previewCase {
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
			return renderDoctor(previewReport(ws), nil, false, false, false)
		}},
		{Name: "doctor-findings", Group: "doctor", Description: "safe, warning and missing-key findings", Render: func() string {
			ws := findingsWorkspace()
			return renderDoctor(previewReport(ws), nil, false, false, false)
		}},
		{Name: "doctor-conflicts", Group: "doctor", Description: "choose and destructive findings", Render: func() string {
			ws := conflictWorkspace()
			return renderDoctor(previewReport(ws), nil, false, false, false)
		}},
		{Name: "doctor-notices", Group: "doctor", Description: "informational findings only", Render: func() string {
			return renderDoctor(noticesReport(), nil, false, false, false)
		}},
		{Name: "doctor-fix-applied", Group: "doctor", Description: "applied fixes with masked secrets", Render: func() string {
			ws := healthyWorkspace()
			applied := []checks.Change{
				{Scope: "species:emobase-deploy-tcas", Key: "COMPOSE_PROJECT_NAME", Old: "", New: "tcas"},
				{Scope: "species:emobase-deploy-tcas", Key: "PORTAL_SERVER", Old: "", New: "https://portal.example.org"},
				{Scope: "species:emobase-deploy-tcas", Key: "DIRECTUS_SECRET", Old: "", New: "new-secret", Secret: true},
				{Scope: "portal", Key: "MYSQL_ROOT_PASSWORD", Old: "wrong-secret", New: "root-secret", Secret: true},
			}
			return renderDoctor(previewReport(ws), applied, true, true, false)
		}},

		{Name: "credential-list", Group: "credential", Description: "admin login plus two realms sharing one", Render: func() string {
			return renderCredentialList([]credentialRow{
				{Stack: "infra", User: "admin", Source: previewRoot + "/emobase-infra"},
				{Stack: "portal", Realm: "emobase-portal", User: "coco", Seed: "unused", Source: previewRoot + "/emobase-portal-deploy"},
				{Stack: "Lsta", Realm: "emobase-portal", User: "coco", Seed: "unused", Source: previewRoot + "/emobase-deploy-lsta", CopyDiffers: true},
				{Stack: "Ptep", Realm: "emobase-ptep", User: "coco", Seed: "changed", Source: previewRoot + "/emobase-deploy-ptep"},
			})
		}},
		{Name: "credential-infra", Group: "credential", Description: "Keycloak admin login", Render: func() string {
			return renderCredentialAdmin(credentialRow{
				Stack: "infra", User: "admin", Seed: "admin",
				Source: previewRoot + "/emobase-infra", Password: "example-admin-pass",
			}, "")
		}},
		{Name: "credential-detail", Group: "credential", Description: "unused seed prints user and password", Render: func() string {
			return renderCredentialDetail(credentialRow{
				Stack: "Lsta", Realm: "emobase-portal", User: "coco",
				Seed: "unused", Source: previewRoot + "/emobase-deploy-lsta", Password: "example-generated-seed",
			}, "https://auth.example.org/keycloak/admin/")
		}},
		{Name: "credential-detail-changed", Group: "credential", Description: "replaced seed with reset instructions", Render: func() string {
			return renderCredentialDetail(credentialRow{
				Stack: "Ptep", Realm: "emobase-ptep", User: "coco",
				Seed: "changed", Source: previewRoot + "/emobase-deploy-ptep", Password: "example-generated-seed",
			}, "https://auth.example.org/keycloak/admin/")
		}},

		{Name: "species-list", Group: "species", Description: "two species checkouts", Render: func() string {
			return renderSpecies(speciesRows(conflictWorkspace()))
		}},
		{Name: "species-empty", Group: "species", Description: "no species found", Render: func() string {
			return renderSpecies(nil)
		}},

		{Name: "image-mixed", Group: "image", Description: "local overrides plus Compose defaults", Render: func() string {
			return renderImages(previewImageRows(true, false))
		}},
		{Name: "image-defaults", Group: "image", Description: "every stack on its Compose default", Render: func() string {
			return renderImages(previewImageRows(false, false))
		}},
		{Name: "image-all", Group: "image", Description: "supporting services plus a no-fallback row", Render: func() string {
			return renderImages(previewImageRows(true, true))
		}},
		{Name: "image-empty", Group: "image", Description: "no portal or species checkouts", Render: func() string {
			return renderImages(nil)
		}},
		{Name: "image-set", Group: "image", Description: "pin the portal genomics image", Render: func() string {
			t, _ := findImageTarget("genomics")
			return renderImageChanges(t, []imageChange{
				{Target: "genomics", Checkout: "emobase-portal-deploy", Variable: "EMOBASE_GENOMICS_IMAGE",
					New: "emobase-genomics:local", Default: "ghcr.io/emobase/emobase-genomics:dev"},
			})
		}},
		{Name: "image-set-species", Group: "image", Description: "pin one image across several species", Render: func() string {
			t, _ := findImageTarget("species-web")
			return renderImageChanges(t, []imageChange{
				{Target: "species-web", Checkout: "emobase-deploy-dmel", Variable: "EMOBASE_WEB_IMAGE",
					New: "emobase-web:local", Default: "ghcr.io/emobase/emobase-web:dev"},
				{Target: "species-web", Checkout: "emobase-deploy-tcas", Variable: "EMOBASE_WEB_IMAGE",
					New: "emobase-web:local", Default: "ghcr.io/emobase/emobase-web:dev"},
			})
		}},
		{Name: "image-reset", Group: "image", Description: "remove an override and show the Compose default", Render: func() string {
			t, _ := findImageTarget("genomics")
			return renderImageChanges(t, []imageChange{
				{Target: "genomics", Checkout: "emobase-portal-deploy", Variable: "EMOBASE_GENOMICS_IMAGE",
					Old: "emobase-genomics:local", Default: "ghcr.io/emobase/emobase-genomics:dev", Reset: true},
			})
		}},
		{Name: "image-reset-all", Group: "image", Description: "clear portal and species overrides together", Render: func() string {
			return renderImageChanges(imageTarget{}, []imageChange{
				{Target: "genomics", Checkout: "emobase-portal-deploy", Variable: "EMOBASE_GENOMICS_IMAGE",
					Old: "emobase-genomics:local", Default: "ghcr.io/emobase/emobase-genomics:dev", Reset: true},
				{Target: "species-web", Checkout: "emobase-deploy-tcas", Variable: "EMOBASE_WEB_IMAGE",
					Old: "emobase-web:local", Default: "ghcr.io/emobase/emobase-web:dev", Reset: true},
				{Target: "ontologyservice", Checkout: "emobase-deploy-tcas", Variable: "ONTOLOGYSERVICE_IMAGE",
					Old: "ghcr.io/emobase/ibb-ontologyservice:test", Default: "ghcr.io/emobase/ibb-ontologyservice:v1.6.2", Reset: true},
			})
		}},
		{Name: "image-reset-all-skip", Group: "image", Description: "reset all except an image without a fallback", Render: func() string {
			return renderImageChanges(imageTarget{}, []imageChange{
				{Target: "portal-web", Checkout: "emobase-portal-deploy", Variable: "EMOBASE_PORTAL_WEB_IMAGE",
					Old: "ghcr.io/emobase/emobase-portal-web:test", Default: "ghcr.io/emobase/emobase-portal-web:main", Reset: true},
				{Target: "species-web", Checkout: "emobase-deploy-tcas", Variable: "EMOBASE_WEB_IMAGE",
					Old: "emobase-web:local", Default: "ghcr.io/emobase/emobase-web:dev", Reset: true},
			}, "left genomics unchanged in emobase-portal-deploy: EMOBASE_GENOMICS_IMAGE has no Compose fallback")
		}},
		{Name: "image-nochange", Group: "image", Description: "set or reset made no edit", Render: func() string {
			t, _ := findImageTarget("publicationservice")
			return renderImageChanges(t, nil)
		}},
		{Name: "image-reset-no-fallback", Group: "image", Description: "reset refused because Compose has no fallback", Render: func() string {
			t, _ := findImageTarget("genomics")
			return "Error: " + imageNoFallbackError(t.Name, t.Variable, "emobase-portal-deploy").Error() + "\n"
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
