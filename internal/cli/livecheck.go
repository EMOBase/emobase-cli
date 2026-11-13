package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/EMOBase/emobase-cli/internal/checks"
	"github.com/EMOBase/emobase-cli/internal/keycloak"
	"github.com/EMOBase/emobase-cli/internal/live"
	"github.com/EMOBase/emobase-cli/internal/scaffold"
	"github.com/EMOBase/emobase-cli/internal/workspace"
)

// addLiveChecks appends runtime findings to a static report. It degrades to a
// single warning when Docker is unavailable.
func addLiveChecks(ctx context.Context, ws *workspace.Workspace, rep *checks.Report) {
	if !live.DockerAvailable(ctx) {
		rep.Add(checks.Advisory("live.docker", "workspace", checks.Warn,
			"Docker daemon is not reachable; runtime checks skipped",
			"start Docker and re-run `emobase doctor --live`"))
		return
	}
	if ws.Infra == nil {
		return
	}

	containers, err := live.ComposePS(ctx, ws.Infra.Dir)
	if err != nil {
		rep.Add(checks.Advisory("live.infra", "infra", checks.Error,
			"could not query emobase-infra containers", err.Error()))
		return
	}
	byService := map[string]live.Container{}
	for _, c := range containers {
		byService[c.Service] = c
	}
	mysqlUp, keycloakUp := false, false
	for _, svc := range []string{"mysql", "keycloak"} {
		c, ok := byService[svc]
		if !ok {
			rep.Add(checks.Advisory("live.service", "infra", checks.Error,
				fmt.Sprintf("%s container has not been created", svc),
				"`emobase up --only infra`"))
			continue
		}
		state := c.State
		if state == "" {
			state = c.Status
		}
		up := isRunning(state, c.Status)
		switch {
		case !up:
			rep.Add(checks.Advisory("live.service", "infra", checks.Error,
				fmt.Sprintf("%s container is not running (%s)", svc, state),
				"`emobase up --only infra`"))
		case c.Health == "unhealthy":
			rep.Add(checks.Advisory("live.service", "infra", checks.Error,
				fmt.Sprintf("%s container is unhealthy", svc),
				fmt.Sprintf("`cd %s && docker compose logs --tail=100 %s`", ws.Infra.Dir, svc)))
		}
		if up {
			if svc == "mysql" {
				mysqlUp = true
			} else {
				keycloakUp = true
			}
		}
	}

	runner := &scaffold.Runner{Stdout: io.Discard, Stderr: io.Discard}

	netName, _ := ws.Infra.Get("EMOBASE_INFRA_NETWORK")
	if netName != "" {
		if ok, err := live.NetworkExists(ctx, netName); err != nil || !ok {
			rep.Add(checks.Advisory("live.network", "infra", checks.Error,
				fmt.Sprintf("shared Docker network %q is missing", netName),
				"`emobase up --only infra` (Compose creates it)"))
		}
	}

	if mysqlUp {
		dbs, err := live.Databases(ctx, ws.Infra.Dir)
		if err != nil {
			rep.Add(checks.Advisory("live.database", "infra", checks.Warn,
				"could not list MySQL databases", err.Error()))
		} else {
			have := map[string]bool{}
			for _, d := range dbs {
				have[strings.ToLower(d)] = true
			}
			need := map[string]string{"keycloak": "infra"}
			if ws.Portal != nil {
				db := "emobase_genomics"
				if v, ok := ws.Portal.Get("EMOBASE_GENOMICS_MYSQL__DATABASE"); ok && v != "" {
					db = v
				}
				need[strings.ToLower(db)] = "portal"
			}
			for _, sp := range ws.Species {
				if db, ok := sp.Get("DIRECTUS_DB_DATABASE"); ok && db != "" {
					need[strings.ToLower(db)] = checks.ScopeOf(sp)
				}
			}
			for db, scope := range need {
				if have[db] {
					continue
				}
				dbCopy, scopeCopy := db, scope
				rep.Add(checks.Action("live.database", scope, checks.Error, checks.FixSafe,
					fmt.Sprintf("database %q does not exist", db),
					"`emobase doctor --fix --live` creates it with `emobase-infra/scripts/create-database.sh`",
					func() ([]checks.Change, error) {
						if err := runner.CreateDatabase(ws.Infra.Dir, dbCopy); err != nil {
							return nil, err
						}
						return []checks.Change{{Scope: scopeCopy, Key: "database:" + dbCopy, Old: "missing", New: "created"}}, nil
					}))
			}
		}
	}

	if keycloakUp {
		base, _ := ws.Infra.Get("KEYCLOAK_PUBLIC_URL")
		adminUser, _ := ws.Infra.Get("KEYCLOAK_ADMIN")
		adminPass, _ := ws.Infra.Get("KEYCLOAK_ADMIN_PASSWORD")
		if base != "" {
			kc := keycloak.New(base, adminUser, adminPass)
			var order []string
			groups := map[string][]*workspace.Stack{}
			add := func(st *workspace.Stack) {
				if st == nil || st.MissingEnv {
					return
				}
				realm, _, _, _, _, ok := stackRealm(st)
				if !ok {
					return
				}
				if _, exists := groups[realm]; !exists {
					order = append(order, realm)
				}
				groups[realm] = append(groups[realm], st)
			}
			add(ws.Portal)
			for _, sp := range ws.Species {
				add(sp)
			}
			for _, realm := range order {
				members := groups[realm]
				scope := checks.ScopeOf(members[0])
				if ok, err := kc.RealmReachable(ctx, realm); err != nil || !ok {
					rep.Add(checks.Advisory("live.realm", scope, checks.Error,
						fmt.Sprintf("realm %q is not reachable at KEYCLOAK_PUBLIC_URL", realm),
						"check KEYCLOAK_PUBLIC_URL (it must be the browser URL) and emobase-infra logs"))
					continue
				}
				if adminUser == "" || adminPass == "" {
					continue
				}
				_, clientID, _, _, _, _ := stackRealm(members[0])
				crep, err := kc.ClientByID(ctx, realm, clientID)
				switch {
				case err != nil:
					rep.Add(checks.Advisory("live.client", scope, checks.Warn,
						fmt.Sprintf("could not inspect client %q in realm %q", clientID, realm), err.Error()))
					continue
				case crep == nil:
					rep.Add(checks.Advisory("live.client", scope, checks.Error,
						fmt.Sprintf("client %q does not exist in realm %q", clientID, realm),
						"re-run the portal/species realm setup or `emobase bootstrap`"))
					continue
				}
				for _, st := range members {
					origin, _ := st.Get("PUBLIC_SERVER")
					if origin == "" || keycloak.MatchesOrigin(crep.RedirectURIs, origin) {
						continue
					}
					originCopy, scopeCopy := origin, checks.ScopeOf(st)
					rep.Add(checks.Action("live.redirect_uri", scopeCopy, checks.Error, checks.FixSafe,
						fmt.Sprintf("origin %s is not registered on client %q in realm %q", origin, clientID, realm),
						"`emobase doctor --fix --live` registers it with the realm helper",
						func() ([]checks.Change, error) {
							if err := ensureStackRealm(ctx, runner, ws, st); err != nil {
								return nil, err
							}
							return []checks.Change{{Scope: scopeCopy, Key: "redirectUri", Old: "(unregistered)", New: originCopy}}, nil
						}))
				}
			}
		}
	}

	if ws.Portal != nil {
		if u, ok := ws.Portal.Get("PUBLIC_SERVER"); ok && u != "" {
			if code, err := live.HTTPStatus(ctx, u); err != nil {
				rep.Add(checks.Advisory("live.portal", "portal", checks.Warn,
					"portal is not reachable at "+u, err.Error()))
			} else if code >= 500 {
				rep.Add(checks.Advisory("live.portal", "portal", checks.Error,
					fmt.Sprintf("portal returned HTTP %d", code),
					"`emobase status`; `docker compose logs --tail=100 fe genomicsservice`"))
			}
		}
	}
	for _, sp := range ws.Species {
		if u, ok := sp.Get("PUBLIC_SERVER"); ok && u != "" {
			if _, err := live.HTTPStatus(ctx, u); err != nil {
				rep.Add(checks.Advisory("live.site", checks.ScopeOf(sp), checks.Warn,
					"species site is not reachable at "+u, err.Error()))
			}
		}
	}
}
