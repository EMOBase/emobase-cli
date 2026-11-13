// Package checks validates the cross-repository environment contract and
// offers safe, idempotent fixes.
package checks

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/EMOBase/emobase-cli/internal/spec"
	"github.com/EMOBase/emobase-cli/internal/workspace"
)

// Severity is the impact of a finding.
type Severity int

const (
	OK Severity = iota
	Info
	Warn
	Error
)

func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warn:
		return "warn"
	case Info:
		return "info"
	default:
		return "ok"
	}
}

// Label is the short, coloured tag used in tables.
func (s Severity) Label() string {
	switch s {
	case Error:
		return "ERROR"
	case Warn:
		return "WARN"
	case Info:
		return "INFO"
	default:
		return "OK"
	}
}

// FixKind explains what it takes to resolve a finding.
type FixKind int

const (
	FixNone FixKind = iota
	FixSafe
	FixChoose
	FixDestructive
)

func (f FixKind) String() string {
	switch f {
	case FixSafe:
		return "safe"
	case FixChoose:
		return "choose"
	case FixDestructive:
		return "destructive"
	default:
		return "-"
	}
}

// Change is one environment-file edit.
type Change struct {
	Scope  string `json:"scope"`
	Key    string `json:"key"`
	Old    string `json:"old"`
	New    string `json:"new"`
	Secret bool   `json:"-"`
}

// MarshalJSON masks secret values so machine-readable output never leaks them.
func (c Change) MarshalJSON() ([]byte, error) {
	old, new := c.Old, c.New
	if c.Secret {
		old, new = "********", "********"
	}
	return json.Marshal(struct {
		Scope string `json:"scope"`
		Key   string `json:"key"`
		Old   string `json:"old"`
		New   string `json:"new"`
	}{Scope: c.Scope, Key: c.Key, Old: old, New: new})
}

// Finding is one failed or advisory check.
type Finding struct {
	Check    string   `json:"check"`
	Scope    string   `json:"scope"`
	Severity Severity `json:"-"`
	Fix      FixKind  `json:"-"`
	Message  string   `json:"message"`
	Hint     string   `json:"hint,omitempty"`

	apply func() ([]Change, error)
}

// CanApply reports whether `doctor --fix` can edit files for this finding.
func (f Finding) CanApply(includeChoose bool) bool {
	if f.apply == nil {
		return false
	}
	return f.Fix == FixSafe || (includeChoose && f.Fix == FixChoose)
}

// Advisory builds a finding without a fix action.
func Advisory(check, scope string, sev Severity, msg, hint string) Finding {
	return Finding{Check: check, Scope: scope, Severity: sev, Fix: FixNone, Message: msg, Hint: hint}
}

// Action builds a finding with a fix action. Fix should be FixSafe or
// FixChoose; destructive findings must not carry an action.
func Action(check, scope string, sev Severity, fix FixKind, msg, hint string, apply func() ([]Change, error)) Finding {
	return Finding{Check: check, Scope: scope, Severity: sev, Fix: fix, Message: msg, Hint: hint, apply: apply}
}

// Add appends a finding to the report (used by live checks in other packages).
func (r *Report) Add(f Finding) { r.add(f) }

// Apply runs the fix closure.
func (f Finding) Apply() ([]Change, error) {
	if f.apply == nil {
		return nil, nil
	}
	return f.apply()
}

// Report is the result of a full check run.
type Report struct {
	Root     string    `json:"workspace"`
	Findings []Finding `json:"findings"`
}

func (r *Report) add(f Finding) { r.Findings = append(r.Findings, f) }

func (r *Report) Counts() (errors, warns, infos int) {
	for _, f := range r.Findings {
		switch f.Severity {
		case Error:
			errors++
		case Warn:
			warns++
		case Info:
			infos++
		}
	}
	return
}

// HasErrors reports whether any finding is an error.
func (r *Report) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Severity == Error {
			return true
		}
	}
	return false
}

// Sort orders findings by severity (errors first), then scope and check.
func (r *Report) Sort() {
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if a.Severity != b.Severity {
			return a.Severity > b.Severity
		}
		if a.Scope != b.Scope {
			return a.Scope < b.Scope
		}
		return a.Check < b.Check
	})
}

// ScopeOf returns the stable display name for a stack.
func ScopeOf(s *workspace.Stack) string { return scopeOf(s) }

func scopeOf(s *workspace.Stack) string {
	if s == nil {
		return "workspace"
	}
	switch s.Role {
	case workspace.RoleInfra:
		return "infra"
	case workspace.RolePortal:
		return "portal"
	default:
		return "species:" + s.Name
	}
}

// Run evaluates every static check.
func Run(w *workspace.Workspace) *Report {
	r := &Report{Root: w.Root}

	if w.Infra == nil {
		r.add(Finding{
			Check:    "workspace.infra",
			Scope:    "workspace",
			Severity: Error,
			Fix:      FixNone,
			Message:  "no emobase-infra checkout found",
			Hint:     "clone emobase-infra next to this workspace: `git clone https://github.com/EMOBase/emobase-infra.git`",
		})
	}
	if w.Portal == nil && len(w.Species) > 0 {
		r.add(Finding{
			Check:    "workspace.portal",
			Scope:    "workspace",
			Severity: Warn,
			Fix:      FixNone,
			Message:  "no local emobase-portal-deploy checkout; species must use a remote portal",
			Hint:     "clone emobase-portal-deploy here, or set PORTAL_SERVER in each species .env",
		})
	}

	// A stack without .env cannot be checked further; report it once.
	for _, st := range w.All() {
		if st.MissingEnv {
			r.add(Finding{
				Check:    "env.missing",
				Scope:    scopeOf(st),
				Severity: Error,
				Fix:      FixNone,
				Message:  fmt.Sprintf("%s has no .env", st.Dir),
				Hint:     fmt.Sprintf("`cd %s && ./init.sh`  (or `emobase create`)", st.Dir),
			})
		}
	}

	if w.Infra != nil && !w.Infra.MissingEnv {
		checkRequired(r, w.Infra, infraRequired())
	}
	if w.Portal != nil && !w.Portal.MissingEnv {
		checkRequired(r, w.Portal, spec.RequiredPortal)
		checkShared(r, w, w.Portal)
	}
	for _, sp := range w.Species {
		if sp.MissingEnv {
			continue
		}
		checkRequired(r, sp, spec.RequiredSpecies)
		checkSpeciesProjectName(r, sp)
		checkShared(r, w, sp)
		checkSpeciesRealm(r, sp)
		checkIssuerKeys(r, sp)
	}
	checkRealmGroups(r, w)
	checkSpeciesPortalURL(r, w)
	checkTrailingSlashes(r, w)
	checkUniqueness(r, w)
	checkLoopbackPorts(r, w)
	checkInfraLoopbackPort(r, w)
	checkInternalHosts(r, w)
	checkPlaceholders(r, w)
	return r
}

func infraRequired() []string {
	out := append([]string{}, spec.RequiredInfra...)
	for _, k := range spec.SharedToInfra {
		out = append(out, k.Key)
	}
	return out
}

// checkSpeciesProjectName requires an explicit Compose project name so two
// clones do not silently share container names when they are both checked out
// as emobase-deploy. The safe fix derives one from the species code.
func checkSpeciesProjectName(r *Report, st *workspace.Stack) {
	if st.Has("COMPOSE_PROJECT_NAME") {
		return
	}
	want := st.ProjectName
	if code, ok := st.Get("MAIN_SPECIES"); ok && code != "" {
		want = strings.ToLower(strings.TrimSpace(code))
	}
	if want == "" {
		want = strings.ToLower(st.Name)
	}
	r.add(Finding{
		Check:    "required.COMPOSE_PROJECT_NAME",
		Scope:    scopeOf(st),
		Severity: Error,
		Fix:      FixSafe,
		Message:  "COMPOSE_PROJECT_NAME is empty; Compose would fall back to the directory name",
		Hint:     fmt.Sprintf("`emobase doctor --fix` sets it to %q", want),
		apply:    setter(st, "COMPOSE_PROJECT_NAME", want, false),
	})
}

func checkRequired(r *Report, st *workspace.Stack, keys []string) {
	for _, key := range keys {
		v, ok := st.Get(key)
		if ok && strings.TrimSpace(v) != "" {
			continue
		}
		r.add(Finding{
			Check:    "required." + key,
			Scope:    scopeOf(st),
			Severity: Error,
			Fix:      FixNone,
			Message:  key + " is missing",
			Hint:     fmt.Sprintf("run `./init.sh` in %s (or `emobase create`) to generate it", st.Dir),
		})
	}
}

// checkShared compares the global shared keys with emobase-infra. Realm and
// client credentials are per realm group and are checked separately.
func checkShared(r *Report, w *workspace.Workspace, st *workspace.Stack) {
	if w.Infra == nil || w.Infra.Env == nil {
		return
	}
	for _, sk := range spec.SharedToInfra {
		compareShared(r, w.Infra, st, sk)
	}
}

func compareShared(r *Report, infra, st *workspace.Stack, sk spec.SharedKey) {
	want, ok := infra.Get(sk.Key)
	if !ok || spec.IsPlaceholder(want) {
		return // infra's own required check reports it
	}
	got, ok := st.Get(sk.Key)
	display := func(v string) string {
		if sk.Secret {
			return spec.Mask(v)
		}
		return v
	}
	if !ok || spec.IsPlaceholder(got) {
		r.add(Finding{
			Check:    "shared." + sk.Key,
			Scope:    scopeOf(st),
			Severity: Error,
			Fix:      FixSafe,
			Message:  fmt.Sprintf("%s is missing or a placeholder; it must match %s (%s)", sk.Key, spec.AuthorityInfra, display(want)),
			Hint:     fmt.Sprintf("`emobase doctor --fix`  (copies %s from emobase-infra)", sk.Key),
			apply:    setter(st, sk.Key, want, sk.Secret),
		})
		return
	}
	if equalValue(sk.Key, got, want) {
		return
	}
	r.add(Finding{
		Check:    "shared." + sk.Key,
		Scope:    scopeOf(st),
		Severity: Error,
		Fix:      FixChoose,
		Message:  fmt.Sprintf("%s does not match emobase-infra (%s != %s)", sk.Key, display(got), display(want)),
		Hint:     fmt.Sprintf("`emobase doctor --fix --yes` copies emobase-infra's value; verify manually first"),
		apply:    setter(st, sk.Key, want, sk.Secret),
	})
}

func checkSpeciesRealm(r *Report, st *workspace.Stack) {
	realm, _ := st.Get("KEYCLOAK_REALM")
	clientID, _ := st.Get("KEYCLOAK_CLIENT_ID")
	secret, _ := st.Get("KEYCLOAK_CLIENT_SECRET")
	if strings.TrimSpace(realm) == "" {
		r.add(Finding{
			Check:    "realm.missing",
			Scope:    scopeOf(st),
			Severity: Error,
			Fix:      FixNone,
			Message:  "KEYCLOAK_REALM is missing; choose an existing realm or create a new one",
			Hint:     "`emobase create species`   (or `--keycloak-realm join:REALM` / `new:REALM`)",
		})
		return
	}
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(secret) == "" {
		r.add(Finding{
			Check:    "realm.client",
			Scope:    scopeOf(st),
			Severity: Error,
			Fix:      FixNone,
			Message:  fmt.Sprintf("realm %s needs KEYCLOAK_CLIENT_ID and KEYCLOAK_CLIENT_SECRET", realm),
			Hint:     fmt.Sprintf("`cd %s` and re-run the realm setup", st.Dir),
		})
	}
}

// checkRealmGroups ensures every stack that uses the same realm also uses the
// same client ID and secret.
func checkRealmGroups(r *Report, w *workspace.Workspace) {
	type group struct {
		stack  *workspace.Stack
		client string
		secret string
	}
	seen := map[string]group{}
	add := func(st *workspace.Stack) {
		if st == nil || st.MissingEnv {
			return
		}
		realm, _ := st.Get("KEYCLOAK_REALM")
		if strings.TrimSpace(realm) == "" {
			return
		}
		client, _ := st.Get("KEYCLOAK_CLIENT_ID")
		secret, _ := st.Get("KEYCLOAK_CLIENT_SECRET")
		first, ok := seen[realm]
		if !ok {
			seen[realm] = group{stack: st, client: client, secret: secret}
			return
		}
		if client != "" && first.client != "" && client != first.client {
			r.add(Finding{
				Check:    "realm.client_mismatch",
				Scope:    scopeOf(st),
				Severity: Error,
				Fix:      FixNone,
				Message:  fmt.Sprintf("realm %s uses client %q here but %q in %s", realm, client, first.client, first.stack.Name),
				Hint:     "all checkouts that share a realm must use the same client ID and secret",
			})
		}
		if secret != "" && first.secret != "" && secret != first.secret {
			r.add(Finding{
				Check:    "realm.secret_mismatch",
				Scope:    scopeOf(st),
				Severity: Error,
				Fix:      FixNone,
				Message:  fmt.Sprintf("realm %s has a different client secret from %s", realm, first.stack.Name),
				Hint:     "copy the realm owner's KEYCLOAK_CLIENT_SECRET into this checkout",
			})
		}
	}
	add(w.Portal)
	for _, sp := range w.Species {
		add(sp)
	}
}

func checkIssuerKeys(r *Report, st *workspace.Stack) {
	realm, _ := st.Get("KEYCLOAK_REALM")
	if realm == "" {
		return
	}
	for _, key := range []string{"INTERNAL_KEYCLOAK_ISSUER", "DIRECTUS_KEYCLOAK_ISSUER_URL"} {
		v, ok := st.Get(key)
		if !ok || v == "" {
			continue
		}
		if !strings.HasSuffix(spec.NormalizeURL(v), "/realms/"+realm) {
			r.add(Finding{
				Check:    "issuer." + key,
				Scope:    scopeOf(st),
				Severity: Warn,
				Fix:      FixNone,
				Message:  fmt.Sprintf("%s does not end with /realms/%s", key, realm),
				Hint:     "server-side sign-in will fail; align it with KEYCLOAK_REALM",
			})
		}
	}
}

func checkSpeciesPortalURL(r *Report, w *workspace.Workspace) {
	var portalURL string
	if w.Portal != nil && !w.Portal.MissingEnv {
		portalURL, _ = w.Portal.Get("PUBLIC_SERVER")
	}
	if portalURL == "" {
		for _, sp := range w.Species {
			if sp.MissingEnv || sp.Has("PORTAL_SERVER") {
				continue
			}
			r.add(Finding{
				Check:    "species.portal_server",
				Scope:    scopeOf(sp),
				Severity: Error,
				Fix:      FixNone,
				Message:  "PORTAL_SERVER is missing and no local portal PUBLIC_SERVER is available",
				Hint:     "pass the portal URL to `./init.sh` or `emobase create species --portal-server`",
			})
		}
		return
	}
	for _, sp := range w.Species {
		if sp.MissingEnv {
			continue
		}
		got, ok := sp.Get("PORTAL_SERVER")
		if ok && spec.NormalizeURL(got) == spec.NormalizeURL(portalURL) {
			continue
		}
		if !ok || spec.IsPlaceholder(got) {
			r.add(Finding{
				Check:    "species.portal_server",
				Scope:    scopeOf(sp),
				Severity: Error,
				Fix:      FixSafe,
				Message:  "PORTAL_SERVER is missing; the species site needs the portal URL",
				Hint:     "`emobase doctor --fix` copies the portal PUBLIC_SERVER",
				apply:    setter(sp, "PORTAL_SERVER", portalURL, false),
			})
			continue
		}
		r.add(Finding{
			Check:    "species.portal_server",
			Scope:    scopeOf(sp),
			Severity: Error,
			Fix:      FixChoose,
			Message:  fmt.Sprintf("PORTAL_SERVER (%s) differs from the local portal (%s)", got, portalURL),
			Hint:     "`emobase doctor --fix --yes` points this species at the local portal",
			apply:    setter(sp, "PORTAL_SERVER", portalURL, false),
		})
	}
}

func checkTrailingSlashes(r *Report, w *workspace.Workspace) {
	for _, st := range w.All() {
		if st.Env == nil {
			continue
		}
		for key := range spec.URLKeys {
			v, ok := st.Get(key)
			if !ok || !strings.HasSuffix(v, "/") {
				continue
			}
			trimmed := strings.TrimRight(v, "/")
			r.add(Finding{
				Check:    "url.trailing_slash." + key,
				Scope:    scopeOf(st),
				Severity: Warn,
				Fix:      FixSafe,
				Message:  key + " has a trailing slash",
				Hint:     "Keycloak treats a trailing slash as a different issuer",
				apply:    setter(st, key, trimmed, false),
			})
		}
	}
}

func checkUniqueness(r *Report, w *workspace.Workspace) {
	type entry struct {
		scope string
		value string
	}
	check := func(checkID, key string, entries []entry, severity Severity) {
		seen := map[string][]string{}
		for _, e := range entries {
			if strings.TrimSpace(e.value) == "" {
				continue
			}
			seen[e.value] = append(seen[e.value], e.scope)
		}
		for value, scopes := range seen {
			if len(scopes) < 2 {
				continue
			}
			r.add(Finding{
				Check:    checkID,
				Scope:    "workspace",
				Severity: severity,
				Fix:      FixChoose,
				Message:  fmt.Sprintf("%s %q is used by %s", key, value, strings.Join(scopes, ", ")),
				Hint:     "give each stack a unique value (and matching PUBLIC_SERVER/HTTP_PORT)",
			})
		}
	}

	var ports, dbs, servers, projects []entry
	for _, st := range w.All() {
		if st.MissingEnv {
			continue
		}
		if v, ok := st.Get("HTTP_PORT"); ok {
			ports = append(ports, entry{scopeOf(st), v})
		}
		if v, ok := st.Get("PUBLIC_SERVER"); ok {
			servers = append(servers, entry{scopeOf(st), spec.NormalizeURL(v)})
		}
		if st.Role == workspace.RoleSpecies {
			if v, ok := st.Get("DIRECTUS_DB_DATABASE"); ok {
				dbs = append(dbs, entry{scopeOf(st), strings.ToLower(v)})
			}
			if st.ProjectName != "" {
				projects = append(projects, entry{scopeOf(st), st.ProjectName})
			}
		}
	}
	check("unique.http_port", "HTTP_PORT", ports, Error)
	check("unique.directus_db", "DIRECTUS_DB_DATABASE", dbs, Error)
	check("unique.public_server", "PUBLIC_SERVER", servers, Error)
	check("unique.compose_project", "Compose project", projects, Error)
}

func checkLoopbackPorts(r *Report, w *workspace.Workspace) {
	for _, st := range w.All() {
		if st.MissingEnv {
			continue
		}
		publicServer, ok := st.Get("PUBLIC_SERVER")
		httpPort, ok2 := st.Get("HTTP_PORT")
		if !ok || !ok2 {
			continue
		}
		u, err := url.Parse(publicServer)
		if err != nil {
			continue
		}
		host := u.Hostname()
		if host != "localhost" && host != "127.0.0.1" {
			continue
		}
		port := u.Port()
		if port == "" {
			if u.Scheme == "http" {
				port = "80"
			} else {
				port = "443"
			}
		}
		if port != httpPort {
			r.add(Finding{
				Check:    "local.port_mismatch",
				Scope:    scopeOf(st),
				Severity: Warn,
				Fix:      FixChoose,
				Message:  fmt.Sprintf("PUBLIC_SERVER uses port %s but HTTP_PORT is %s", port, httpPort),
				Hint:     "for a local setup these must match; behind a TLS proxy a mismatch is expected",
			})
		}
	}
}

// checkInfraLoopbackPort verifies that a localhost Keycloak URL uses the
// published host port. Behind a TLS proxy the public port legitimately differs,
// so this only applies to loopback URLs.
func checkInfraLoopbackPort(r *Report, w *workspace.Workspace) {
	if w.Infra == nil || w.Infra.MissingEnv {
		return
	}
	public, ok := w.Infra.Get("KEYCLOAK_PUBLIC_URL")
	hostPort, ok2 := w.Infra.Get("KEYCLOAK_HOST_PORT")
	if !ok || !ok2 {
		return
	}
	u, err := url.Parse(public)
	if err != nil {
		return
	}
	if host := u.Hostname(); host != "localhost" && host != "127.0.0.1" {
		return
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	if port != hostPort {
		r.add(Finding{
			Check:    "local.keycloak_port",
			Scope:    "infra",
			Severity: Warn,
			Fix:      FixNone,
			Message:  fmt.Sprintf("KEYCLOAK_PUBLIC_URL uses port %s but KEYCLOAK_HOST_PORT is %s", port, hostPort),
			Hint:     "for a local setup these must match; behind a TLS proxy a mismatch is expected",
		})
	}
}

// checkInternalHosts warns when a container-facing URL points at loopback.
// Inside a container, localhost is the container itself; the shared stack is
// reached through a Compose service name or host.docker.internal.
func checkInternalHosts(r *Report, w *workspace.Workspace) {
	keys := []string{
		"KEYCLOAK_INTERNAL_URL",
		"PORTAL_INTERNAL_URL",
		"INTERNAL_KEYCLOAK_ISSUER",
		"DIRECTUS_KEYCLOAK_ISSUER_URL",
		"INTERNAL_DIRECTUS_URL",
		"INTERNAL_API_GENOMICS_SERVICE",
	}
	for _, st := range w.All() {
		if st.MissingEnv {
			continue
		}
		for _, key := range keys {
			v, ok := st.Get(key)
			if !ok || v == "" {
				continue
			}
			u, err := url.Parse(v)
			if err != nil || u.Host == "" {
				continue
			}
			host := u.Hostname()
			if host != "localhost" && host != "127.0.0.1" && host != "::1" {
				continue
			}
			r.add(Finding{
				Check:    "internal.loopback." + key,
				Scope:    scopeOf(st),
				Severity: Warn,
				Fix:      FixNone,
				Message:  fmt.Sprintf("%s uses loopback (%s), but containers cannot reach the Docker host that way", key, v),
				Hint:     "use a Compose service name (for example http://keycloak:8080/keycloak) or host.docker.internal",
			})
		}
	}
}

func checkPlaceholders(r *Report, w *workspace.Workspace) {
	for _, st := range w.All() {
		if st.Env == nil {
			continue
		}
		for _, key := range st.Env.Keys() {
			v, _ := st.Get(key)
			if strings.Contains(strings.ToLower(v), "change-me") {
				r.add(Finding{
					Check:    "placeholder." + key,
					Scope:    scopeOf(st),
					Severity: Warn,
					Fix:      FixNone,
					Message:  key + " still has an example value",
					Hint:     fmt.Sprintf("regenerate it with `./init.sh` in %s", st.Dir),
				})
			}
		}
	}
}

func setter(st *workspace.Stack, key, value string, secret bool) func() ([]Change, error) {
	return func() ([]Change, error) {
		old, _ := st.Get(key)
		if !st.Env.Set(key, value) {
			return nil, nil
		}
		return []Change{{Scope: scopeOf(st), Key: key, Old: old, New: value, Secret: secret}}, nil
	}
}

func equalValue(key, a, b string) bool {
	if spec.URLKeys[key] {
		return spec.NormalizeURL(a) == spec.NormalizeURL(b)
	}
	return a == b
}

// Apply edits the stacks' .env files for every applicable finding and saves
// them. Choose-class fixes are applied only when includeChoose is true.
func Apply(w *workspace.Workspace, r *Report, includeChoose bool) ([]Change, error) {
	var changes []Change
	for _, f := range r.Findings {
		if !f.CanApply(includeChoose) {
			continue
		}
		cs, err := f.Apply()
		if err != nil {
			return changes, err
		}
		changes = append(changes, cs...)
	}
	for _, st := range w.All() {
		if st.Env != nil && st.Env.Dirty() {
			if err := st.Env.Save(); err != nil {
				return changes, fmt.Errorf("save %s: %w", st.EnvPath, err)
			}
		}
	}
	return changes, nil
}

// MustAtoi is a small helper used by callers that need a numeric port.
func MustAtoi(s string, fallback int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}
