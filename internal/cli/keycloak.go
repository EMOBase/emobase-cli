package cli

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/EMOBase/emobase-cli/internal/scaffold"
	"github.com/EMOBase/emobase-cli/internal/ui"
	"github.com/EMOBase/emobase-cli/internal/workspace"
	"github.com/spf13/cobra"
)

// keycloakCmd groups Keycloak administration. Keycloak is hosted by the
// portal, so these commands only run where the portal checkout is present.
func (a *App) keycloakCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keycloak",
		Short: "Manage the portal Keycloak",
		Long: `Manage the Keycloak instance hosted by emobase-portal-deploy.

Realm creation and origin changes require the portal's Keycloak container and
admin credentials, so they are only available on the portal host. A species
server can consume the credentials but cannot manage Keycloak.`,
	}
	cmd.AddCommand(a.keycloakRealmCmd())
	return cmd
}

func (a *App) keycloakRealmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "realm",
		Short: "Manage Keycloak realms",
	}
	cmd.AddCommand(a.keycloakRealmCreateCmd(), a.keycloakRealmAddOriginCmd())
	return cmd
}

type realmCreateOpts struct {
	species, realm, origin, clientID, clientSecret, cocoUser, cocoPass string
	rotateSecret, printEnv, dryRun                                     bool
}

func (a *App) keycloakRealmCreateCmd() *cobra.Command {
	o := &realmCreateOpts{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a realm and a client for a site",
		Long: `Create a Keycloak realm, a confidential client and a seed user on the
portal host, then print the block a species .env needs.

The client secret and seed password are generated; they are never prompted.
Use --client-secret or --coco-password only when another system must supply
them. If the realm already exists, use 'emobase keycloak realm add-origin'
or --rotate-secret.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runRealmCreate(cmd, o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.species, "species", "", "species code used for default realm, origin and client names")
	f.StringVar(&o.realm, "realm", "", "realm name (default: emobase-<species>)")
	f.StringVar(&o.origin, "origin", "", "browser origin, for example https://tcas.example.org")
	f.StringVar(&o.clientID, "client-id", "", "client ID (default: <realm>-client)")
	f.StringVar(&o.clientSecret, "client-secret", "", "client secret (generated when omitted)")
	f.StringVar(&o.cocoUser, "coco-username", "", "seed user name (default coco)")
	f.StringVar(&o.cocoPass, "coco-password", "", "seed password (generated when omitted)")
	f.BoolVar(&o.rotateSecret, "rotate-secret", false, "allow an existing realm and replace its client secret")
	f.BoolVar(&o.printEnv, "print-env", false, "print only the .env block")
	f.BoolVar(&o.dryRun, "dry-run", false, "print the commands without changing Keycloak")
	return cmd
}

func (a *App) runRealmCreate(cmd *cobra.Command, o *realmCreateOpts) error {
	ws, err := a.Workspace()
	if err != nil {
		return err
	}
	if ws.Portal == nil || ws.Portal.MissingEnv {
		return fmt.Errorf("no portal checkout found; create the realm on the portal host")
	}
	ctx := cmd.Context()
	if !o.dryRun && !keycloakRunning(ctx, ws) {
		return portalNotRunningError(cmd, ws)
	}
	publicURL, _ := ws.Portal.Get("KEYCLOAK_PUBLIC_URL")
	if strings.TrimSpace(publicURL) == "" {
		return fmt.Errorf("%s has no KEYCLOAK_PUBLIC_URL", ws.Portal.EnvPath)
	}

	realmDefault, originDefault := "", ""
	if o.species != "" {
		slug := speciesSlug(o.species)
		if slug == "" {
			return fmt.Errorf("invalid species code %q", o.species)
		}
		realmDefault = "emobase-" + slug
		if sp := ws.FindSpecies(o.species); sp != nil {
			originDefault, _ = sp.Get("PUBLIC_SERVER")
		}
		if originDefault == "" {
			originDefault = speciesOrigin(publicURL, slug)
		}
	}

	if o.realm == "" {
		if a.Yes {
			o.realm = realmDefault
		} else if realmDefault != "" {
			if err := prompt(cmd, "Realm", realmDefault, &o.realm); err != nil {
				return err
			}
		} else {
			value, err := promptRequired(cmd, "Realm", "emobase-tcas")
			if err != nil {
				return err
			}
			o.realm = value
		}
	}
	if o.realm == "" {
		return fmt.Errorf("--realm is required when --species is not given")
	}
	if !validKeycloakName(o.realm) {
		return fmt.Errorf("invalid realm name %q", o.realm)
	}
	if o.origin == "" {
		if a.Yes {
			o.origin = originDefault
		} else if originDefault != "" {
			if err := prompt(cmd, "Species origin", originDefault, &o.origin); err != nil {
				return err
			}
		} else {
			value, err := promptRequired(cmd, "Species origin", "https://tcas.example.org")
			if err != nil {
				return err
			}
			o.origin = value
		}
	}
	if o.origin == "" {
		return fmt.Errorf("--origin is required when it cannot be derived from --species")
	}
	origin, err := normalizeOrigin(o.origin)
	if err != nil {
		return fmt.Errorf("species origin: %w", err)
	}
	if o.clientID == "" {
		clientDefault := o.realm + "-client"
		if a.Yes {
			o.clientID = clientDefault
		} else if err := prompt(cmd, "Client ID", clientDefault, &o.clientID); err != nil {
			return err
		}
	}
	if !validKeycloakName(o.clientID) {
		return fmt.Errorf("invalid client ID %q", o.clientID)
	}
	if o.clientSecret == "" {
		o.clientSecret, err = randomToken(32)
		if err != nil {
			return err
		}
	}
	if o.cocoUser == "" {
		o.cocoUser = "coco"
	}
	if o.cocoPass == "" {
		o.cocoPass, err = randomToken(24)
		if err != nil {
			return err
		}
	}

	args := []string{
		"--realm", o.realm,
		"--origin", origin,
		"--client-id", o.clientID,
		"--client-secret", o.clientSecret,
		"--coco-username", o.cocoUser,
		"--coco-password", o.cocoPass,
		"--quiet",
	}
	if !o.rotateSecret {
		args = append(args, "--create-only")
	}
	r := &scaffold.Runner{DryRun: o.dryRun, Stdout: io.Discard, Stderr: cmd.ErrOrStderr()}
	if err := r.Script(ws.Portal.Dir, "./scripts/create-realm.sh", args...); err != nil {
		return fmt.Errorf("create realm: %w", err)
	}

	block := fmt.Sprintf(
		"KEYCLOAK_PUBLIC_URL=%s\nKEYCLOAK_REALM=%s\nKEYCLOAK_CLIENT_ID=%s\nKEYCLOAK_CLIENT_SECRET=%s\nCOCO_USERNAME=%s\nCOCO_PASSWORD=%s\n",
		strings.TrimRight(publicURL, "/"), o.realm, o.clientID, o.clientSecret, o.cocoUser, o.cocoPass,
	)
	out := cmd.OutOrStdout()
	if o.printEnv {
		fmt.Fprint(out, block)
		return nil
	}
	fmt.Fprintln(out, title("Keycloak realm"))
	if o.dryRun {
		fmt.Fprintf(out, "Would create %s (%s) for %s.\n\n", o.realm, o.clientID, origin)
	} else {
		fmt.Fprintf(out, "Created %s (%s) for %s.\n\n", o.realm, o.clientID, origin)
	}
	fmt.Fprint(out, block)
	return nil
}

type realmOriginOpts struct {
	realm, clientID, origin string
	dryRun                  bool
}

func (a *App) keycloakRealmAddOriginCmd() *cobra.Command {
	o := &realmOriginOpts{}
	cmd := &cobra.Command{
		Use:   "add-origin",
		Short: "Allow another site origin on an existing realm client",
		Long: `Add a browser origin to an existing realm client. The client secret and
seed password are left unchanged.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runRealmAddOrigin(cmd, o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.realm, "realm", "", "existing realm")
	f.StringVar(&o.clientID, "client-id", "", "existing client ID")
	f.StringVar(&o.origin, "origin", "", "browser origin to allow")
	f.BoolVar(&o.dryRun, "dry-run", false, "print the command without changing Keycloak")
	return cmd
}

func (a *App) runRealmAddOrigin(cmd *cobra.Command, o *realmOriginOpts) error {
	ws, err := a.Workspace()
	if err != nil {
		return err
	}
	if ws.Portal == nil || ws.Portal.MissingEnv {
		return fmt.Errorf("no portal checkout found; update the realm on the portal host")
	}
	ctx := cmd.Context()
	if !o.dryRun && !keycloakRunning(ctx, ws) {
		return portalNotRunningError(cmd, ws)
	}
	if o.realm == "" {
		value, err := promptRequired(cmd, "Realm", "emobase-tcas")
		if err != nil {
			return err
		}
		o.realm = value
	}
	if o.clientID == "" {
		clientDefault := o.realm + "-client"
		if a.Yes {
			o.clientID = clientDefault
		} else if err := prompt(cmd, "Client ID", clientDefault, &o.clientID); err != nil {
			return err
		}
	}
	if o.origin == "" {
		if a.Yes {
			return fmt.Errorf("--origin is required")
		}
		value, err := promptRequired(cmd, "Species origin", "https://tcas.example.org")
		if err != nil {
			return err
		}
		o.origin = value
	}
	origin, err := normalizeOrigin(o.origin)
	if err != nil {
		return fmt.Errorf("species origin: %w", err)
	}
	r := &scaffold.Runner{DryRun: o.dryRun, Stdout: io.Discard, Stderr: cmd.ErrOrStderr()}
	if err := r.Script(ws.Portal.Dir, "./scripts/create-realm.sh",
		"--realm", o.realm,
		"--origin", origin,
		"--client-id", o.clientID,
		"--keep-secret",
		"--keep-password",
		"--existing-only",
		"--quiet",
	); err != nil {
		return fmt.Errorf("add origin: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s allows %s\n", ui.OK("OK"), o.clientID, origin)
	return nil
}

// portalNotRunningError explains the actual startup order. The portal cannot
// start before the shared network exists, so telling the operator to run
// `up --only portal` alone is misleading.
func portalNotRunningError(cmd *cobra.Command, ws *workspace.Workspace) error {
	if ws.Infra == nil || ws.Infra.MissingEnv || !infraServiceRunning(cmd.Context(), ws, "mysql") {
		return fmt.Errorf("the shared infrastructure is not running; start it first:\n  emobase up --only infra\nthen:\n  emobase up --only portal")
	}
	return fmt.Errorf("the portal Keycloak is not running; start it with: emobase up --only portal")
}

func validKeycloakName(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

// speciesOrigin derives the conventional <species>.<portal-host> origin.
func speciesOrigin(portalURL, slug string) string {
	u, err := url.Parse(portalURL)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	host := slug + "." + u.Hostname()
	if port := u.Port(); port != "" {
		host += ":" + port
	}
	return scheme + "://" + host
}

func randomToken(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(buf)
	if len(encoded) < length {
		return "", fmt.Errorf("random token is too short")
	}
	return encoded[:length], nil
}
