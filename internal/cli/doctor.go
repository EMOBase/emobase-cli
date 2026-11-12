package cli

import (
	"fmt"

	"github.com/EMOBase/emobase-cli/internal/checks"
	"github.com/spf13/cobra"
)

func (a *App) doctorCmd() *cobra.Command {
	var fix, withLive bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check the cross-repository configuration and optionally fix it",
		Long: `doctor compares every stack's .env with the authoritative emobase-infra
values, checks uniqueness of ports, URLs and databases, and validates Keycloak
realm/issuer references.

  --fix        apply safe fixes (fill missing values, normalise URLs)
  --fix --yes  also apply choose-class fixes (copy emobase-infra's value)
  --live       also inspect running containers, MySQL, Keycloak and HTTP

Destructive changes (a Keycloak issuer or realm origin after the first import)
are never applied automatically.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			rep := runChecks(ws, withLive, cmd)
			out := cmd.OutOrStdout()

			var applied []checks.Change
			if fix {
				applied, err = checks.Apply(ws, rep, a.Yes)
				if err != nil {
					return err
				}
				if len(applied) > 0 {
					if reloaded, rerr := a.Workspace(); rerr == nil {
						ws = reloaded
						rep = runChecks(ws, withLive, cmd)
					}
				}
			}

			if a.JSON {
				if err := a.writeJSON(out, ws, rep, applied); err != nil {
					return err
				}
				if rep.HasErrors() {
					return ErrFindings
				}
				return nil
			}

			fmt.Fprint(out, renderDoctor(rep, applied, fix, a.Yes))
			if rep.HasErrors() {
				return ErrFindings
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "apply safe fixes to .env files (.env.bak is kept)")
	cmd.Flags().BoolVar(&withLive, "live", false, "also inspect running containers, MySQL, Keycloak and HTTP endpoints")
	return cmd
}

func hasChoose(rep *checks.Report) bool {
	for _, f := range rep.Findings {
		if f.Fix == checks.FixChoose && f.Severity == checks.Error {
			return true
		}
	}
	return false
}
