package cli

import (
	"fmt"

	"github.com/EMOBase/emobase-cli/internal/scaffold"
	"github.com/EMOBase/emobase-cli/internal/ui"
	"github.com/EMOBase/emobase-cli/internal/workspace"
	"github.com/spf13/cobra"
)

func (a *App) bootstrapCmd() *cobra.Command {
	var only string
	var noPull, skipSetup, skipMigrations, skipSmoke, dryRun bool
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bring a scaffolded workspace up in dependency order",
		Long: `bootstrap expects every selected stack to have a .env (use emobase create
first). It then follows the deployment guide:

  1. infra: pull (unless --no-pull) and start, waiting for health
  2. portal: pull, JBrowse2 setup, both migrations, start, smoke test
  3. each species: create the database, register the shared-realm origin,
     initialise Directus and start

Use --dry-run to print the exact commands without running them.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			r := &scaffold.Runner{DryRun: dryRun, Stdout: out, Stderr: cmd.ErrOrStderr()}

			doInfra := only == "" || only == "all" || only == "infra"
			doPortal := only == "" || only == "all" || only == "portal"
			doSpecies := only == "" || only == "all" || only == "species"
			if !doInfra && !doPortal && !doSpecies {
				return fmt.Errorf("unknown --only %q (use infra, portal, species or all)", only)
			}

			var missing []string
			addMissing := func(st *workspace.Stack) {
				if st != nil && st.MissingEnv {
					missing = append(missing, st.Dir)
				}
			}
			if doInfra {
				addMissing(ws.Infra)
			}
			if doPortal {
				addMissing(ws.Portal)
			}
			if doSpecies {
				for _, sp := range ws.Species {
					addMissing(sp)
				}
			}
			if len(missing) > 0 {
				for _, m := range missing {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s has no .env\n", m)
				}
				return fmt.Errorf("scaffold the stacks first, for example: emobase create infra, emobase create portal, emobase create species")
			}

			if doInfra && ws.Infra != nil {
				fmt.Fprintln(out, title("Infrastructure"))
				if !noPull {
					if err := r.Pull(ws.Infra.Dir); err != nil {
						return err
					}
				}
				if err := r.Up(ws.Infra.Dir); err != nil {
					return err
				}
			}
			if doPortal && ws.Portal != nil {
				fmt.Fprintln(out, title("Portal"))
				if !noPull {
					if err := r.Pull(ws.Portal.Dir); err != nil {
						return err
					}
				}
				if !skipSetup {
					if err := r.SetupJBrowse2(ws.Portal.Dir); err != nil {
						return err
					}
				}
				if !skipMigrations {
					if err := r.Migrate(ws.Portal.Dir); err != nil {
						return err
					}
				}
				if err := r.Up(ws.Portal.Dir); err != nil {
					return err
				}
				if !skipSmoke {
					if err := r.SmokeTest(ws.Portal.Dir); err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), muted("smoke test failed: "+err.Error()))
					}
				}
			}
			if doSpecies {
				for _, sp := range ws.Species {
					fmt.Fprintln(out, title("Species "+sp.Name))
					if err := a.setupSpecies(cmd, ws, sp, r); err != nil {
						return err
					}
				}
			}
			fmt.Fprintln(out)
			fmt.Fprintf(out, "%s bootstrap finished\n", ui.OK("OK"))
			return nil
		},
	}
	cmd.Flags().StringVar(&only, "only", "all", "infra, portal, species or all")
	cmd.Flags().BoolVar(&noPull, "no-pull", false, "skip image pulls")
	cmd.Flags().BoolVar(&skipSetup, "skip-setup", false, "skip the one-shot JBrowse2 setup")
	cmd.Flags().BoolVar(&skipMigrations, "skip-migrations", false, "skip portal migrations")
	cmd.Flags().BoolVar(&skipSmoke, "skip-smoke", false, "skip the portal smoke test")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print commands without running them")
	return cmd
}
