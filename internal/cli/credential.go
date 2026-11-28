package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/EMOBase/emobase-cli/internal/keycloak"
	"github.com/EMOBase/emobase-cli/internal/ui"
	"github.com/EMOBase/emobase-cli/internal/workspace"
	"github.com/spf13/cobra"
)

type credentialState string

const (
	stateUnused  credentialState = "unused"
	stateChanged credentialState = "changed"
	stateMissing credentialState = "missing"
	stateUnknown credentialState = "unknown"
)

// credentialRow is the render model for both views and for --json. Password is
// populated only for the single-stack view.
type credentialRow struct {
	Stack       string `json:"stack"`
	Realm       string `json:"realm"`
	User        string `json:"user"`
	Seed        string `json:"seed,omitempty"`
	Source      string `json:"source"`
	CopyDiffers bool   `json:"copies_differ,omitempty"`
	Password    string `json:"password,omitempty"`
}

// seedKey identifies one seed user. Several checkouts can share a realm, and
// the seed state is a property of the realm/user pair, not of a checkout.
type seedKey struct {
	realm string
	user  string
}

func (a *App) credentialCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "credential",
		Short: "Inspect the seed login credential of each realm",
	}
	cmd.AddCommand(a.credentialShowCmd())
	return cmd
}

func (a *App) credentialShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [stack|keycloak]",
		Short: "List seed users; name a stack to reveal its password",
		Long: `Without an argument, show lists every stack's seed user and whether the
password is still unused. The password itself is only printed when a stack is
named, so the fleet view stays safe to paste into a ticket. 'show keycloak'
prints the Keycloak admin credential from emobase-portal-deploy/.env. 'show
infra' is kept as an alias because the admin credential used to live there.

A stack is accepted as its table label (the species code, or "portal"/"infra"),
its checkout name (emobase-deploy-lsta) or its Compose project name.

Each checkout stores its own COCO_PASSWORD copy. When several checkouts share a
realm those copies can drift; the output flags that rather than picking one as
authoritative, because the file cannot tell which copy Keycloak hashed. Verify by
signing in.

The seed password is created as temporary, so Keycloak asks for a new one at
first sign-in. The detail view still shows the stored value for reference;
reset it in the Keycloak admin console if you need a working login.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			if len(args) == 1 && isKeycloakRef(args[0]) {
				return a.credentialKeycloakDetail(cmd, ws)
			}
			if len(args) == 1 && isInfraRef(ws, args[0]) {
				// Compatibility alias: the Keycloak admin used to live in the
				// infra checkout.
				return a.credentialKeycloakDetail(cmd, ws)
			}
			if len(realmStacks(ws)) == 0 {
				return fmt.Errorf("no portal or species checkout has realm credentials")
			}
			if len(args) == 1 {
				return a.credentialDetail(cmd, ws, args[0])
			}
			return a.credentialList(cmd, ws)
		},
	}
}

// realmStacks returns every stack that has realm client credentials, portal
// first, then species in discovery order.
func realmStacks(ws *workspace.Workspace) []*workspace.Stack {
	out := make([]*workspace.Stack, 0, len(ws.Species)+1)
	if ws.Portal != nil {
		if _, _, _, _, _, ok := stackRealm(ws.Portal); ok {
			out = append(out, ws.Portal)
		}
	}
	for _, sp := range ws.Species {
		if _, _, _, _, _, ok := stackRealm(sp); ok {
			out = append(out, sp)
		}
	}
	return out
}

func (a *App) credentialList(cmd *cobra.Command, ws *workspace.Workspace) error {
	rows := a.collectCredentialRows(cmd.Context(), ws)
	if a.JSON {
		return writeCredentialJSON(cmd.OutOrStdout(), ws.Root, rows)
	}
	fmt.Fprint(cmd.OutOrStdout(), renderCredentialList(rows))
	return nil
}

// credentialLabel is the short, typeable name shown in the table and accepted
// by `show`: the role for infra/portal, the species code for a species.
func credentialLabel(st *workspace.Stack) string {
	switch st.Role {
	case workspace.RoleInfra:
		return "infra"
	case workspace.RolePortal:
		return "portal"
	}
	if code, ok := st.Get("MAIN_SPECIES"); ok && strings.TrimSpace(code) != "" {
		return strings.TrimSpace(code)
	}
	return st.Name
}

func (a *App) collectCredentialRows(ctx context.Context, ws *workspace.Workspace) []credentialRow {
	kc := realmAdmin(ws)
	stacks := realmStacks(ws)
	states := map[seedKey]credentialState{}
	rows := make([]credentialRow, 0, len(stacks)+1)

	// The Keycloak admin login is a bootstrap credential too, and it is the one
	// needed to reset a seed password.
	if _, user, _, dir := portalKeycloak(ws); user != "" {
		rows = append(rows, credentialRow{
			Stack:  "keycloak",
			User:   user,
			Source: dir,
		})
	}

	for _, st := range stacks {
		realm, _, _, user, pass, ok := stackRealm(st)
		if !ok {
			continue
		}
		key := seedKey{realm: realm, user: user}
		state, seen := states[key]
		if !seen {
			state, _ = credentialStatus(ctx, kc, realm, user)
			states[key] = state
		}
		rows = append(rows, credentialRow{
			Stack:  credentialLabel(st),
			Realm:  realm,
			User:   user,
			Seed:   string(state),
			Source: st.Dir,
			// Divergent copies only matter while a seed password can still be
			// live; once the first sign-in replaced it, the copies are
			// historical and the warning would be misleading.
			CopyDiffers: seedCanBeLive(state) && copiesDiffer(stacks, realm, user, pass),
		})
	}
	return rows
}

func (a *App) credentialDetail(cmd *cobra.Command, ws *workspace.Workspace, ref string) error {
	st := findStack(ws, ref)
	if st == nil {
		return fmt.Errorf("no stack matches %q (try emobase species list)", ref)
	}
	realm, _, _, user, pass, ok := stackRealm(st)
	if !ok {
		return fmt.Errorf("%s has no realm credentials", st.Name)
	}

	state, _ := credentialStatus(cmd.Context(), realmAdmin(ws), realm, user)
	row := credentialRow{
		Stack:       credentialLabel(st),
		Realm:       realm,
		User:        user,
		Seed:        string(state),
		Source:      st.Dir,
		CopyDiffers: seedCanBeLive(state) && copiesDiffer(realmStacks(ws), realm, user, pass),
		Password:    pass,
	}

	if a.JSON {
		return writeCredentialJSON(cmd.OutOrStdout(), ws.Root, []credentialRow{row})
	}
	fmt.Fprint(cmd.OutOrStdout(), renderCredentialDetail(row, keycloakAdminURL(ws)))
	return nil
}

// seedCanBeLive reports whether a stored copy could still be the seed password
// Keycloak accepts. Only then is it worth warning that copies disagree.
func seedCanBeLive(state credentialState) bool {
	return state == stateUnused || state == stateUnknown
}

// copiesDiffer reports whether another checkout using the same realm and seed
// user stores a different password. It says nothing about which one is live.
func copiesDiffer(stacks []*workspace.Stack, realm, user, pass string) bool {
	for _, other := range stacks {
		r, _, _, u, p, ok := stackRealm(other)
		if !ok || r != realm || u != user {
			continue
		}
		if p != pass {
			return true
		}
	}
	return false
}

// renderCredentialList is the password-free fleet view. It never receives a
// password: collectCredentialRows leaves that field empty by construction.
func renderCredentialList(rows []credentialRow) string {
	var b strings.Builder
	table := make([][]string, 0, len(rows))
	divergent := make([]string, 0)
	seenRealm := map[string]bool{}

	for _, r := range rows {
		table = append(table, []string{
			r.Stack, r.Realm, r.User, r.Seed, filepath.Base(r.Source) + "/.env",
		})
		if r.CopyDiffers && seedCanBeLive(credentialState(r.Seed)) && !seenRealm[r.Realm] {
			seenRealm[r.Realm] = true
			divergent = append(divergent, r.Realm)
		}
	}

	fmt.Fprint(&b, ui.Table([]string{"stack", "realm", "user", "state", "source"}, table))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, muted("Name a stack to print its password: emobase credential show <stack>"))
	for _, realm := range divergent {
		fmt.Fprintln(&b, muted(fmt.Sprintf("note: checkouts sharing realm %s store different COCO_PASSWORD values; only one can be live.", realm)))
	}
	return b.String()
}

// renderCredentialDetail prints the seed login for one stack. The stored copy
// is shown after the first sign-in too, marked "(seed)", so it can be kept for
// reference even though Keycloak has already replaced it.
func renderCredentialDetail(r credentialRow, adminURL string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "user:     %s\n", r.User)
	switch credentialState(r.Seed) {
	case stateUnused:
		fmt.Fprintf(&b, "password: %s\n", r.Password)
	case stateChanged:
		fmt.Fprintf(&b, "password: %s (seed)\n", r.Password)
		fmt.Fprintln(&b, "The seed password was already changed at first sign-in.")
		console := "the Keycloak admin console"
		if adminURL != "" {
			console = adminURL
		}
		fmt.Fprintln(&b, muted(fmt.Sprintf("Reset it at %s → realm %s → Users → %s → Credentials.", console, r.Realm, r.User)))
	case stateMissing:
		fmt.Fprintln(&b, "Keycloak has no such user in this realm, so the stored value cannot work.")
	default:
		fmt.Fprintln(&b, "The seed password could not be verified (Keycloak unreachable or admin credentials missing).")
		fmt.Fprintln(&b, muted("Check the admin login with `emobase credential show keycloak`."))
	}
	return b.String()
}

// renderCredentialAdmin prints the shared Keycloak admin login.
func renderCredentialAdmin(r credentialRow, verifyErr string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "user:     %s\n", r.User)
	fmt.Fprintf(&b, "password: %s\n", r.Password)
	if verifyErr != "" {
		fmt.Fprintln(&b, muted("Keycloak did not accept these credentials: "+verifyErr))
	}
	return b.String()
}

func (a *App) credentialKeycloakDetail(cmd *cobra.Command, ws *workspace.Workspace) error {
	if ws.Portal == nil {
		return fmt.Errorf("no emobase-portal-deploy checkout found; the Keycloak admin credential lives there")
	}
	_, user, pass, dir := portalKeycloak(ws)
	if strings.TrimSpace(user) == "" || pass == "" {
		return fmt.Errorf("%s/.env has no KEYCLOAK_ADMIN / KEYCLOAK_ADMIN_PASSWORD", dir)
	}

	row := credentialRow{
		Stack:    "keycloak",
		User:     user,
		Source:   dir,
		Password: pass,
	}
	if a.JSON {
		return writeCredentialJSON(cmd.OutOrStdout(), ws.Root, []credentialRow{row})
	}

	verifyErr := ""
	if kc := realmAdmin(ws); kc != nil {
		ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
		defer cancel()
		if err := kc.VerifyAdmin(ctx); err != nil {
			verifyErr = err.Error()
		}
	}
	fmt.Fprint(cmd.OutOrStdout(), renderCredentialAdmin(row, verifyErr))
	return nil
}

func isInfraRef(ws *workspace.Workspace, ref string) bool {
	if ws.Infra == nil {
		return false
	}
	ref = strings.TrimSpace(ref)
	return strings.EqualFold(ref, "infra") ||
		strings.EqualFold(ref, ws.Infra.Name) ||
		strings.EqualFold(ref, ws.Infra.Dir)
}

// isKeycloakRef accepts the Keycloak admin credential by name.
func isKeycloakRef(ref string) bool {
	return strings.EqualFold(strings.TrimSpace(ref), "keycloak")
}

// portalKeycloak returns the portal-owned Keycloak credentials. Keycloak moved
// from emobase-infra to emobase-portal-deploy.
func portalKeycloak(ws *workspace.Workspace) (base, admin, pass, dir string) {
	if ws.Portal == nil {
		return "", "", "", ""
	}
	base, _ = ws.Portal.Get("KEYCLOAK_PUBLIC_URL")
	admin, _ = ws.Portal.Get("KEYCLOAK_ADMIN")
	pass, _ = ws.Portal.Get("KEYCLOAK_ADMIN_PASSWORD")
	return strings.TrimSpace(base), strings.TrimSpace(admin), pass, ws.Portal.Dir
}

// keycloakAdminURL points at the admin console of the portal Keycloak, or an
// empty string when the portal checkout does not record it.
func keycloakAdminURL(ws *workspace.Workspace) string {
	base, _, _, _ := portalKeycloak(ws)
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/admin/"
}

func findStack(ws *workspace.Workspace, ref string) *workspace.Stack {
	ref = strings.TrimSpace(ref)
	// Accept the label shown in the fleet table, e.g. species:emobase-deploy-lsta,
	// as well as the checkout name, species code or Compose project name.
	if len(ref) > len("species:") && strings.EqualFold(ref[:len("species:")], "species:") {
		ref = ref[len("species:"):]
	}
	if isInfraRef(ws, ref) {
		return ws.Infra
	}
	if ws.Portal != nil && matchesStackRef(ws.Portal, ref) {
		return ws.Portal
	}
	return ws.FindSpecies(ref)
}

func matchesStackRef(st *workspace.Stack, ref string) bool {
	if strings.EqualFold(st.Name, ref) || strings.EqualFold(st.Dir, ref) {
		return true
	}
	if st.ProjectName != "" && strings.EqualFold(st.ProjectName, ref) {
		return true
	}
	return strings.EqualFold(string(st.Role), ref)
}

// realmAdmin builds an admin client from the portal checkout, which owns the
// Keycloak admin credentials.
func realmAdmin(ws *workspace.Workspace) *keycloak.Client {
	base, admin, pass, _ := portalKeycloak(ws)
	if base == "" || admin == "" || pass == "" {
		return nil
	}
	return keycloak.New(base, admin, pass)
}

// credentialStatus reports whether the stored seed password is still the one
// Keycloak expects. UPDATE_PASSWORD is the required action that a temporary
// password sets, so its presence means no sign-in has replaced it yet. The
// answer is per realm and user, not per checkout.
func credentialStatus(ctx context.Context, kc *keycloak.Client, realm, user string) (credentialState, string) {
	if kc == nil {
		return stateUnknown, "emobase-portal-deploy/.env has no Keycloak admin credentials"
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rep, err := kc.UserByUsername(ctx, realm, user)
	if err != nil {
		return stateUnknown, err.Error()
	}
	if rep == nil {
		return stateMissing, ""
	}
	for _, action := range rep.RequiredActions {
		if strings.EqualFold(action, "UPDATE_PASSWORD") {
			return stateUnused, ""
		}
	}
	return stateChanged, ""
}

func writeCredentialJSON(out io.Writer, root string, rows []credentialRow) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{
		"workspace":   root,
		"credentials": rows,
	})
}
