package cli

import (
	"context"
	"fmt"
	"io"
	"net/url"
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
	up := map[string]bool{}
	for _, svc := range []string{"mysql", "keycloak", "elasticsearch"} {
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
		running := isRunning(state, c.Status)
		switch {
		case !running:
			rep.Add(checks.Advisory("live.service", "infra", checks.Error,
				fmt.Sprintf("%s container is not running (%s)", svc, state),
				"`emobase up --only infra`"))
		case c.Health == "unhealthy":
			rep.Add(checks.Advisory("live.service", "infra", checks.Error,
				fmt.Sprintf("%s container is unhealthy", svc),
				fmt.Sprintf("`cd %s && docker compose logs --tail=100 %s`", ws.Infra.Dir, svc)))
		}
		up[svc] = running
	}
	mysqlUp, keycloakUp := up["mysql"], up["keycloak"]

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
		probeKeycloakFromApp(ctx, sp, rep)
	}
}

// probeKeycloakFromApp verifies the server-side issuer from inside the app
// container. The host can trust a certificate or resolve a name that the
// container cannot, so a fetch from the host does not prove that the callback
// token exchange will work.
func probeKeycloakFromApp(ctx context.Context, sp *workspace.Stack, rep *checks.Report) {
	if sp.MissingEnv {
		return
	}
	issuer := internalKeycloakIssuer(sp)
	if issuer == "" {
		return
	}
	containers, err := live.ComposePS(ctx, sp.Dir)
	if err != nil {
		return
	}
	appUp := false
	for _, c := range containers {
		state := c.State
		if state == "" {
			state = c.Status
		}
		if c.Service == "fe" && isRunning(state, c.Status) {
			appUp = true
			break
		}
	}
	if !appUp {
		return
	}
	endpoint := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	out, err := live.ProbeURLInService(ctx, sp.Dir, "fe", endpoint)
	if err == nil {
		return
	}
	realm, _ := sp.Get("KEYCLOAK_REALM")
	var reason, hint string
	if strings.TrimSpace(out) == "" {
		reason = fmt.Sprintf("the app container did not answer %s within 12s", issuerHost(issuer))
		hint = "check that emobase-infra is running and KEYCLOAK_INTERNAL_URL points at a reachable host"
	} else {
		reason, hint = keycloakProbeHint(out, issuerHost(issuer), strings.TrimSpace(realm))
	}
	rep.Add(checks.Advisory("live.keycloak_internal", checks.ScopeOf(sp), checks.Error, reason, hint))
}

// internalKeycloakIssuer mirrors Compose: KEYCLOAK_INTERNAL_URL, else
// KEYCLOAK_PUBLIC_URL, plus /realms/<realm>.
func internalKeycloakIssuer(sp *workspace.Stack) string {
	base, _ := sp.Get("KEYCLOAK_INTERNAL_URL")
	base = strings.TrimSpace(base)
	if base == "" {
		base, _ = sp.Get("KEYCLOAK_PUBLIC_URL")
		base = strings.TrimSpace(base)
	}
	realm, _ := sp.Get("KEYCLOAK_REALM")
	realm = strings.TrimSpace(realm)
	if base == "" || realm == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/realms/" + realm
}

func issuerHost(issuer string) string {
	u, err := url.Parse(issuer)
	if err != nil || u.Host == "" {
		return issuer
	}
	return u.Host
}

// keycloakProbeHint turns the node/undici error from the in-container fetch
// into a cause and an actionable hint.
func keycloakProbeHint(out, host, realm string) (string, string) {
	low := strings.ToLower(out)
	fallback := "http://host.docker.internal:8181/keycloak"
	switch {
	case strings.Contains(low, "unable_to_verify_leaf_signature"),
		strings.Contains(low, "self_signed_cert_in_chain"),
		strings.Contains(low, "cert_has_expired"),
		strings.Contains(low, "err_tls_cert_altname_invalid"),
		strings.Contains(low, "certificate"):
		return fmt.Sprintf("the app container does not trust the TLS certificate at %s", host),
			fmt.Sprintf("set KEYCLOAK_INTERNAL_URL=%s for a private CA, or mount the CA into fe and set NODE_EXTRA_CA_CERTS", fallback)
	case strings.Contains(low, "enotfound"):
		return fmt.Sprintf("the app container cannot resolve %s", host),
			fmt.Sprintf("the name may exist only in the host's /etc/hosts; set KEYCLOAK_INTERNAL_URL=%s", fallback)
	case strings.Contains(low, "econnrefused"), strings.Contains(low, "etimedout"), strings.Contains(low, "ehostunreach"):
		return fmt.Sprintf("the app container cannot connect to %s", host),
			"check that emobase-infra is running and KEYCLOAK_INTERNAL_URL points at a reachable host"
	case strings.Contains(low, "status "):
		return "the Keycloak issuer answered " + probeStatusLine(out),
			fmt.Sprintf("KEYCLOAK_INTERNAL_URL must be a base URL and the realm /realms/%s must exist", realm)
	default:
		line := probeFirstLine(out)
		if line == "" {
			line = "no output"
		}
		return "could not verify INTERNAL_KEYCLOAK_ISSUER from the app container: " + line,
			"`docker compose exec fe node -e 'fetch(process.env.INTERNAL_KEYCLOAK_ISSUER)'`"
	}
}

func probeStatusLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(strings.ToLower(line), "status ") {
			return strings.TrimSpace(line)
		}
	}
	return "an error status"
}

func probeFirstLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
