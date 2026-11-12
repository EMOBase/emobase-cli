package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/EMOBase/emobase-cli/internal/checks"
	"github.com/EMOBase/emobase-cli/internal/envfile"
	"github.com/EMOBase/emobase-cli/internal/scaffold"
	"github.com/EMOBase/emobase-cli/internal/ui"
	"github.com/EMOBase/emobase-cli/internal/workspace"
	"github.com/spf13/cobra"
)

type domainOpts struct {
	auth    string
	portal  string
	species []string
}

type domainValues struct {
	auth    string
	portal  string
	species map[*workspace.Stack]string
}

type domainChange struct {
	Stack *workspace.Stack
	Scope string
	Key   string
	Old   string
	New   string
}

type domainOrigin struct {
	Stack  *workspace.Stack
	Scope  string
	Realm  string
	Origin string
	Shared bool
}

type domainPlan struct {
	Changes       []domainChange
	Origins       []domainOrigin
	Restarts      []*workspace.Stack
	Warnings      []string
	AuthChanged   bool
	PortalChanged bool
}

type domainChangeJSON struct {
	Scope string `json:"scope"`
	Key   string `json:"key"`
	Old   string `json:"old"`
	New   string `json:"new"`
}

type domainOriginJSON struct {
	Scope string `json:"scope"`
	Realm string `json:"realm"`
	URL   string `json:"url"`
}

type domainPlanJSON struct {
	AuthChanged bool               `json:"auth_changed"`
	Changes     []domainChangeJSON `json:"changes"`
	Origins     []domainOriginJSON `json:"origins"`
	Restarts    []string           `json:"restarts"`
	Warnings    []string           `json:"warnings"`
}

type domainSpeciesStatus struct {
	Name  string `json:"name"`
	Code  string `json:"code"`
	URL   string `json:"url"`
	Realm string `json:"realm"`
}

type domainStatusJSON struct {
	Auth    string                `json:"auth_url"`
	Portal  string                `json:"portal_url,omitempty"`
	Species []domainSpeciesStatus `json:"species"`
}

// domainCmd is both an interactive editor and a command group. Running
// "emobase domain" asks for the values; "plan", "apply" and "status" provide
// the scriptable forms.
func (a *App) domainCmd() *cobra.Command {
	o := &domainOpts{}
	cmd := &cobra.Command{
		Use:   "domain",
		Short: "Review and update public URLs and the Keycloak issuer",
		Long: `domain shows the current browser-facing URLs, lets you change them, and
updates every affected .env file plus the Keycloak realm origins.

Running it without a subcommand asks one question at a time, with the current
value as the default. plan prints the same changes without applying them;
apply is the non-interactive form for automation.

The command does not configure DNS, TLS or the reverse proxy. Set those up
first, then point the URLs here. If the Keycloak URL changes, existing login
sessions become invalid.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDomainInteractive(cmd, o)
		},
	}

	f := cmd.PersistentFlags()
	f.StringVar(&o.auth, "auth", "", "Keycloak public URL, e.g. https://auth.example.org/keycloak")
	f.StringVar(&o.portal, "portal", "", "portal public URL")
	f.StringArrayVar(&o.species, "species", nil, "species URL as REF=URL (repeatable)")

	cmd.AddCommand(a.domainPlanCmd(o), a.domainApplyCmd(o), a.domainStatusCmd(o))
	return cmd
}

func (a *App) domainPlanCmd(o *domainOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "Print the domain changes without applying them",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			vals, err := resolveDomainValues(ws, o)
			if err != nil {
				return err
			}
			plan, err := buildDomainPlan(ws, vals)
			if err != nil {
				return err
			}
			return a.printDomainPlan(cmd, plan)
		},
	}
}

func (a *App) domainApplyCmd(o *domainOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "apply",
		Short: "Apply the domain changes and restart the affected stacks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			vals, err := resolveDomainValues(ws, o)
			if err != nil {
				return err
			}
			plan, err := buildDomainPlan(ws, vals)
			if err != nil {
				return err
			}
			if len(plan.Changes) == 0 {
				return a.printDomainPlan(cmd, plan)
			}
			if !a.JSON {
				a.printDomainPlanText(cmd, plan)
			}
			if !a.Yes {
				if !canPrompt(cmd.InOrStdin()) {
					return fmt.Errorf("refusing to apply domain changes without --yes (stdin is not a terminal)")
				}
				ok, err := promptConfirm(cmd, "Apply these changes and restart the affected stacks?", false)
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
					return nil
				}
			}
			return a.applyDomainPlan(cmd, ws, plan)
		},
	}
}

func (a *App) domainStatusCmd(o *domainOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the configured browser-facing URLs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			return a.printDomainStatus(cmd, ws)
		},
	}
}

func (a *App) runDomainInteractive(cmd *cobra.Command, o *domainOpts) error {
	ws, err := a.Workspace()
	if err != nil {
		return err
	}
	if a.JSON {
		return a.printDomainStatus(cmd, ws)
	}
	vals, err := resolveDomainValues(ws, o)
	if err != nil {
		return err
	}
	if !a.Yes {
		if !canPrompt(cmd.InOrStdin()) {
			return fmt.Errorf("stdin is not a terminal; use emobase domain plan or emobase domain apply")
		}
		if vals.auth, err = promptEdit(cmd, "Keycloak public URL", vals.auth); err != nil {
			return err
		}
		if ws.Portal != nil {
			if vals.portal, err = promptEdit(cmd, "Portal public URL", vals.portal); err != nil {
				return err
			}
		}
		for _, sp := range ws.Species {
			label := "Species " + domainSpeciesLabel(sp) + " URL"
			if vals.species[sp], err = promptEdit(cmd, label, vals.species[sp]); err != nil {
				return err
			}
		}
	}
	plan, err := buildDomainPlan(ws, vals)
	if err != nil {
		return err
	}
	if len(plan.Changes) == 0 {
		return a.printDomainPlan(cmd, plan)
	}
	a.printDomainPlanText(cmd, plan)
	if !a.Yes {
		ok, err := promptConfirm(cmd, "Apply these changes and restart the affected stacks?", false)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
			return nil
		}
	}
	return a.applyDomainPlan(cmd, ws, plan)
}

func resolveDomainValues(ws *workspace.Workspace, o *domainOpts) (*domainValues, error) {
	if ws.Infra == nil {
		return nil, fmt.Errorf("no emobase-infra checkout found")
	}
	vals := &domainValues{species: map[*workspace.Stack]string{}}
	vals.auth, _ = ws.Infra.Get("KEYCLOAK_PUBLIC_URL")
	if o.auth != "" {
		vals.auth = o.auth
	}
	if ws.Portal != nil {
		vals.portal, _ = ws.Portal.Get("PUBLIC_SERVER")
		if o.portal != "" {
			vals.portal = o.portal
		}
	} else if o.portal != "" {
		return nil, fmt.Errorf("--portal was given, but no emobase-portal-deploy checkout was found")
	}
	for _, sp := range ws.Species {
		u, _ := sp.Get("PUBLIC_SERVER")
		vals.species[sp] = u
	}
	for _, spec := range o.species {
		ref, raw, ok := strings.Cut(spec, "=")
		ref, raw = strings.TrimSpace(ref), strings.TrimSpace(raw)
		if !ok || ref == "" || raw == "" {
			return nil, fmt.Errorf("--species must be REF=URL, got %q", spec)
		}
		sp := ws.FindSpecies(ref)
		if sp == nil {
			return nil, fmt.Errorf("no species checkout matches %q", ref)
		}
		if _, ok := vals.species[sp]; !ok {
			return nil, fmt.Errorf("species %q is not in this workspace", ref)
		}
		vals.species[sp] = raw
	}
	return vals, nil
}

func buildDomainPlan(ws *workspace.Workspace, vals *domainValues) (*domainPlan, error) {
	if ws.Infra == nil {
		return nil, fmt.Errorf("no emobase-infra checkout found")
	}
	plan := &domainPlan{}
	infraRealm, _ := ws.Infra.Get("KEYCLOAK_REALM")
	if infraRealm == "" {
		infraRealm = "ibb"
	}
	hostPort, _ := ws.Infra.Get("KEYCLOAK_HOST_PORT")
	if hostPort == "" {
		hostPort = "8081"
	}

	authOldRaw, _ := ws.Infra.Get("KEYCLOAK_PUBLIC_URL")
	authOld, err := normalizeOptionalAuthURL(authOldRaw)
	if err != nil {
		return nil, fmt.Errorf("current infra KEYCLOAK_PUBLIC_URL: %w", err)
	}
	authNew, err := normalizeAuthURL(vals.auth)
	if err != nil {
		return nil, fmt.Errorf("Keycloak URL: %w", err)
	}
	plan.AuthChanged = authOld != authNew
	if plan.AuthChanged {
		plan.Changes = append(plan.Changes, domainChange{
			Stack: ws.Infra, Scope: domainScope(ws.Infra),
			Key: "KEYCLOAK_PUBLIC_URL", Old: authOldRaw, New: authNew,
		})
	}

	originsSeen := map[string]string{}
	var portalNew string
	if ws.Portal != nil {
		portalOldRaw, _ := ws.Portal.Get("PUBLIC_SERVER")
		portalOld, err := normalizeOptionalOrigin(portalOldRaw)
		if err != nil {
			return nil, fmt.Errorf("current portal PUBLIC_SERVER: %w", err)
		}
		portalNew, err = normalizeOrigin(vals.portal)
		if err != nil {
			return nil, fmt.Errorf("portal URL: %w", err)
		}
		if prev, ok := originsSeen[portalNew]; ok {
			return nil, fmt.Errorf("portal and %s use the same origin %s", prev, portalNew)
		}
		originsSeen[portalNew] = "portal"

		plan.PortalChanged = portalOld != portalNew
		if plan.AuthChanged {
			plan.Changes = append(plan.Changes, domainChange{
				Stack: ws.Portal, Scope: domainScope(ws.Portal),
				Key: "KEYCLOAK_PUBLIC_URL", Old: mustGet(ws.Portal, "KEYCLOAK_PUBLIC_URL"), New: authNew,
			})
		}
		if plan.PortalChanged {
			plan.Changes = append(plan.Changes, domainChange{
				Stack: ws.Portal, Scope: domainScope(ws.Portal),
				Key: "PUBLIC_SERVER", Old: portalOldRaw, New: portalNew,
			})
			plan.Origins = append(plan.Origins, domainOrigin{
				Stack: ws.Portal, Scope: domainScope(ws.Portal),
				Realm: infraRealm, Origin: portalNew, Shared: true,
			})
		}
	}

	for _, sp := range ws.Species {
		speciesOldRaw, _ := sp.Get("PUBLIC_SERVER")
		speciesOld, err := normalizeOptionalOrigin(speciesOldRaw)
		if err != nil {
			return nil, fmt.Errorf("current %s PUBLIC_SERVER: %w", domainSpeciesLabel(sp), err)
		}
		speciesNew, err := normalizeOrigin(vals.species[sp])
		if err != nil {
			return nil, fmt.Errorf("%s URL: %w", domainSpeciesLabel(sp), err)
		}
		if prev, ok := originsSeen[speciesNew]; ok {
			return nil, fmt.Errorf("%s and %s use the same origin %s", prev, domainSpeciesLabel(sp), speciesNew)
		}
		originsSeen[speciesNew] = domainSpeciesLabel(sp)

		realm, _ := sp.Get("KEYCLOAK_REALM")
		if realm == "" {
			return nil, fmt.Errorf("%s has no KEYCLOAK_REALM", domainSpeciesLabel(sp))
		}

		if plan.AuthChanged {
			plan.Changes = append(plan.Changes, domainChange{
				Stack: sp, Scope: domainScope(sp),
				Key: "KEYCLOAK_PUBLIC_URL", Old: mustGet(sp, "KEYCLOAK_PUBLIC_URL"), New: authNew,
			})
			issuer := derivedInternalIssuer(authNew, hostPort, realm)
			plan.Changes = append(plan.Changes, domainChange{
				Stack: sp, Scope: domainScope(sp),
				Key: "INTERNAL_KEYCLOAK_ISSUER", Old: mustGet(sp, "INTERNAL_KEYCLOAK_ISSUER"), New: issuer,
			})
		}
		if plan.PortalChanged {
			plan.Changes = append(plan.Changes, domainChange{
				Stack: sp, Scope: domainScope(sp),
				Key: "PORTAL_SERVER", Old: mustGet(sp, "PORTAL_SERVER"), New: portalNew,
			})
			if internal, ok := sp.Get("PORTAL_INTERNAL_URL"); ok {
				if normalized, err := normalizeOptionalOrigin(internal); err == nil {
					if oldPortal, err := normalizeOptionalOrigin(mustGet(ws.Portal, "PUBLIC_SERVER")); err == nil && oldPortal != "" && normalized == oldPortal {
						plan.Changes = append(plan.Changes, domainChange{
							Stack: sp, Scope: domainScope(sp),
							Key: "PORTAL_INTERNAL_URL", Old: internal, New: portalNew,
						})
					}
				}
			}
		}
		if speciesOld != speciesNew {
			plan.Changes = append(plan.Changes, domainChange{
				Stack: sp, Scope: domainScope(sp),
				Key: "PUBLIC_SERVER", Old: speciesOldRaw, New: speciesNew,
			})
			shared := realm == infraRealm
			plan.Origins = append(plan.Origins, domainOrigin{
				Stack: sp, Scope: domainScope(sp),
				Realm: realm, Origin: speciesNew, Shared: shared,
			})
		}
	}

	for _, ch := range plan.Changes {
		if ch.Old != ch.New {
			plan.addRestart(ch.Stack)
		}
	}
	if plan.AuthChanged {
		plan.Warnings = append(plan.Warnings, "The Keycloak issuer changes; every user must sign in again.")
	}
	if plan.PortalChanged {
		plan.Warnings = append(plan.Warnings, "Every species PORTAL_SERVER is updated; update other integrations that use the old portal URL separately.")
	}
	if len(plan.Origins) > 0 {
		plan.Warnings = append(plan.Warnings, "Old realm origins are left in place; remove them in the Keycloak admin console if this is a permanent cutover.")
	}
	for _, raw := range []string{authNew, vals.portal} {
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			continue
		}
		if u.Scheme == "http" && !isLoopbackHost(u.Hostname()) {
			plan.Warnings = append(plan.Warnings, "At least one URL uses http; use https for a deployment reachable from other machines.")
			break
		}
	}
	for _, sp := range ws.Species {
		if u, err := url.Parse(vals.species[sp]); err == nil && isLoopbackHost(u.Hostname()) {
			plan.Warnings = append(plan.Warnings, "At least one URL uses localhost; it is only reachable from the same machine.")
			break
		}
	}
	return plan, nil
}

func (p *domainPlan) addRestart(st *workspace.Stack) {
	for _, existing := range p.Restarts {
		if existing == st {
			return
		}
	}
	p.Restarts = append(p.Restarts, st)
	sort.SliceStable(p.Restarts, func(i, j int) bool {
		return domainRoleRank(p.Restarts[i]) < domainRoleRank(p.Restarts[j])
	})
}

func domainRoleRank(st *workspace.Stack) int {
	switch st.Role {
	case workspace.RoleInfra:
		return 0
	case workspace.RolePortal:
		return 1
	default:
		return 2
	}
}

func domainScope(st *workspace.Stack) string {
	switch st.Role {
	case workspace.RoleInfra:
		return "infra"
	case workspace.RolePortal:
		return "portal"
	default:
		return "species:" + st.Name
	}
}

func domainSpeciesLabel(st *workspace.Stack) string {
	if code, ok := st.Get("MAIN_SPECIES"); ok && code != "" {
		return code
	}
	return st.Name
}

func mustGet(st *workspace.Stack, key string) string {
	v, _ := st.Get(key)
	return v
}

func (a *App) printDomainPlan(cmd *cobra.Command, plan *domainPlan) error {
	if a.JSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(domainPlanToJSON(plan))
	}
	a.printDomainPlanText(cmd, plan)
	return nil
}

func (a *App) printDomainPlanText(cmd *cobra.Command, plan *domainPlan) {
	out := cmd.OutOrStdout()
	if len(plan.Changes) == 0 {
		fmt.Fprintln(out, "No domain changes.")
		return
	}
	fmt.Fprintln(out, title("Domain changes"))
	var rows [][]string
	for _, ch := range plan.Changes {
		rows = append(rows, []string{ch.Scope, ch.Key, ch.Old, ch.New})
	}
	fmt.Fprint(out, ui.Table([]string{"scope", "key", "old", "new"}, rows))
	if len(plan.Origins) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, title("Realm origins to add"))
		for _, o := range plan.Origins {
			fmt.Fprintf(out, "  %-22s %-18s %s\n", o.Scope, "realm:"+o.Realm, o.Origin)
		}
	}
	if len(plan.Restarts) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, title("Stacks to restart"))
		for _, st := range plan.Restarts {
			fmt.Fprintf(out, "  %s\n", st.Name)
		}
	}
	if len(plan.Warnings) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, title("Warnings"))
		for _, w := range plan.Warnings {
			fmt.Fprintf(out, "  %s %s\n", ui.Severity(checks.Warn), w)
		}
	}
}

func domainPlanToJSON(plan *domainPlan) domainPlanJSON {
	j := domainPlanJSON{AuthChanged: plan.AuthChanged, Warnings: plan.Warnings}
	for _, ch := range plan.Changes {
		j.Changes = append(j.Changes, domainChangeJSON{Scope: ch.Scope, Key: ch.Key, Old: ch.Old, New: ch.New})
	}
	for _, o := range plan.Origins {
		j.Origins = append(j.Origins, domainOriginJSON{Scope: o.Scope, Realm: o.Realm, URL: o.Origin})
	}
	for _, st := range plan.Restarts {
		j.Restarts = append(j.Restarts, st.Name)
	}
	if j.Changes == nil {
		j.Changes = []domainChangeJSON{}
	}
	if j.Origins == nil {
		j.Origins = []domainOriginJSON{}
	}
	if j.Restarts == nil {
		j.Restarts = []string{}
	}
	if j.Warnings == nil {
		j.Warnings = []string{}
	}
	return j
}

func (a *App) printDomainStatus(cmd *cobra.Command, ws *workspace.Workspace) error {
	if ws.Infra == nil {
		return fmt.Errorf("no emobase-infra checkout found")
	}
	if a.JSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(domainStatusFor(ws))
	}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, title("Configured domains"))
	var rows [][]string
	auth, _ := ws.Infra.Get("KEYCLOAK_PUBLIC_URL")
	realm, _ := ws.Infra.Get("KEYCLOAK_REALM")
	if realm == "" {
		realm = "ibb"
	}
	rows = append(rows, []string{"infra", "emobase-infra", auth, realm})
	if ws.Portal != nil {
		url, _ := ws.Portal.Get("PUBLIC_SERVER")
		realm, _ := ws.Portal.Get("KEYCLOAK_REALM")
		rows = append(rows, []string{"portal", ws.Portal.Name, url, realm})
	}
	for _, sp := range ws.Species {
		url, _ := sp.Get("PUBLIC_SERVER")
		realm, _ := sp.Get("KEYCLOAK_REALM")
		rows = append(rows, []string{"species", sp.Name, url, realm})
	}
	fmt.Fprint(out, ui.Table([]string{"role", "checkout", "url", "realm"}, rows))
	return nil
}

func domainStatusFor(ws *workspace.Workspace) domainStatusJSON {
	st := domainStatusJSON{}
	st.Auth, _ = ws.Infra.Get("KEYCLOAK_PUBLIC_URL")
	if ws.Portal != nil {
		st.Portal, _ = ws.Portal.Get("PUBLIC_SERVER")
	}
	for _, sp := range ws.Species {
		code, _ := sp.Get("MAIN_SPECIES")
		realm, _ := sp.Get("KEYCLOAK_REALM")
		url, _ := sp.Get("PUBLIC_SERVER")
		st.Species = append(st.Species, domainSpeciesStatus{
			Name: sp.Name, Code: code, URL: url, Realm: realm,
		})
	}
	if st.Species == nil {
		st.Species = []domainSpeciesStatus{}
	}
	return st
}

func (a *App) applyDomainPlan(cmd *cobra.Command, ws *workspace.Workspace, plan *domainPlan) error {
	ctx := cmd.Context()
	if !keycloakRunning(ctx, ws) {
		return fmt.Errorf("Keycloak is not running; start infra first with: emobase up --only infra")
	}

	out := cmd.OutOrStdout()
	if a.JSON {
		out = io.Discard
	}
	files := map[string]*envfile.File{}
	stacks := map[string]*workspace.Stack{}
	for _, ch := range plan.Changes {
		if _, ok := files[ch.Stack.EnvPath]; !ok {
			f, err := envfile.Load(ch.Stack.EnvPath)
			if err != nil {
				return fmt.Errorf("load %s: %w", ch.Stack.EnvPath, err)
			}
			files[ch.Stack.EnvPath] = f
			stacks[ch.Stack.EnvPath] = ch.Stack
		}
		files[ch.Stack.EnvPath].Set(ch.Key, ch.New)
	}

	order := make([]*workspace.Stack, 0, len(stacks))
	for _, st := range stacks {
		order = append(order, st)
	}
	sort.SliceStable(order, func(i, j int) bool { return domainRoleRank(order[i]) < domainRoleRank(order[j]) })
	for _, st := range order {
		f := files[st.EnvPath]
		if !f.Dirty() {
			continue
		}
		if err := f.Save(); err != nil {
			return fmt.Errorf("save %s: %w", st.EnvPath, err)
		}
		fmt.Fprintf(out, "  updated %s\n", st.EnvPath)
	}

	r := &scaffold.Runner{Stdout: out, Stderr: cmd.ErrOrStderr()}
	if plan.AuthChanged {
		fmt.Fprintln(out, title("Restart Keycloak"))
		if err := r.Up(ws.Infra.Dir); err != nil {
			return err
		}
	}
	for _, o := range plan.Origins {
		if o.Shared {
			if err := r.EnsureSharedRealm(ws.Infra.Dir, o.Origin); err != nil {
				return err
			}
			continue
		}
		clientID := mustGet(o.Stack, "KEYCLOAK_CLIENT_ID")
		clientSecret := mustGet(o.Stack, "KEYCLOAK_CLIENT_SECRET")
		cocoUser := mustGet(o.Stack, "COCO_USERNAME")
		cocoPass := mustGet(o.Stack, "COCO_PASSWORD")
		if clientID == "" || clientSecret == "" || cocoPass == "" {
			return fmt.Errorf("%s is missing its dedicated-realm client ID, secret or seed password", o.Stack.Name)
		}
		if err := r.EnsureDedicatedRealm(
			ws.Infra.Dir, o.Realm, o.Origin,
			clientID, clientSecret, cocoUser, cocoPass,
		); err != nil {
			return err
		}
	}
	for _, st := range plan.Restarts {
		if st.Role == workspace.RoleInfra {
			continue
		}
		fmt.Fprintln(out, title("Restart "+st.Name))
		if err := r.Up(st.Dir); err != nil {
			return err
		}
	}

	if plan.AuthChanged {
		realm, _ := ws.Infra.Get("KEYCLOAK_REALM")
		if realm == "" {
			realm = "ibb"
		}
		auth, _ := ws.Infra.Get("KEYCLOAK_PUBLIC_URL")
		auth = domainPlanValue(plan, ws.Infra, "KEYCLOAK_PUBLIC_URL", auth)
		if err := verifyIssuer(auth, realm); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s %s\n", ui.Severity(checks.Warn), err)
		}
	}

	if a.JSON {
		return a.printDomainPlan(cmd, plan)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s domain updated\n", ui.OK("OK"))
	return nil
}

func domainPlanValue(plan *domainPlan, st *workspace.Stack, key, fallback string) string {
	for _, ch := range plan.Changes {
		if ch.Stack == st && ch.Key == key {
			return ch.New
		}
	}
	return fallback
}

func verifyIssuer(authURL, realm string) error {
	u := strings.TrimRight(authURL, "/") + "/realms/" + realm + "/.well-known/openid-configuration"
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return fmt.Errorf("could not verify the Keycloak issuer at %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("could not verify the Keycloak issuer at %s: HTTP %d", u, resp.StatusCode)
	}
	var body struct {
		Issuer string `json:"issuer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("could not read the Keycloak discovery document: %w", err)
	}
	want := strings.TrimRight(authURL, "/") + "/realms/" + realm
	if body.Issuer != want {
		return fmt.Errorf("Keycloak reports issuer %q, expected %q", body.Issuer, want)
	}
	return nil
}

func normalizeAuthURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("value is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("must start with http:// or https://")
	}
	if u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("must be a base URL without credentials, query or fragment")
	}
	path := strings.TrimRight(u.Path, "/")
	if path == "" {
		path = "/keycloak"
	}
	if path != "/keycloak" {
		return "", fmt.Errorf("must end with /keycloak (for example https://auth.example.org/keycloak)")
	}
	u.Path = path
	u.RawPath = ""
	return u.String(), nil
}

func normalizeOptionalAuthURL(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	return normalizeAuthURL(raw)
}

func normalizeOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("value is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("must start with http:// or https://")
	}
	if u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("must be an origin without credentials, query or fragment")
	}
	if path := strings.TrimRight(u.Path, "/"); path != "" && path != "/" {
		return "", fmt.Errorf("must not include a path")
	}
	u.Path = ""
	u.RawPath = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func normalizeOptionalOrigin(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	return normalizeOrigin(raw)
}

func derivedInternalIssuer(authURL, hostPort, realm string) string {
	u, err := url.Parse(authURL)
	if err == nil && isLoopbackHost(u.Hostname()) {
		port := hostPort
		if port == "" {
			port = u.Port()
		}
		if port == "" {
			port = "8081"
		}
		return fmt.Sprintf("http://host.docker.internal:%s/keycloak/realms/%s", port, realm)
	}
	return strings.TrimRight(authURL, "/") + "/realms/" + realm
}

func isLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
