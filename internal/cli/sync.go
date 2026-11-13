package cli

import (
	"fmt"

	"github.com/EMOBase/emobase-cli/internal/scaffold"
	"github.com/EMOBase/emobase-cli/internal/ui"
	"github.com/EMOBase/emobase-cli/internal/workspace"
	"github.com/spf13/cobra"
)

func (a *App) syncCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Create missing databases and realms for active stacks",
		Long: `sync reconciles active .env files with MySQL and Keycloak. It creates
missing portal/species databases and realm origins, and records them in the
workspace ownership ledger. It does not pull images, run migrations or start
containers; use bootstrap or up for that.

sync never deletes resources. Use prune for state-owned cleanup.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			if ws.Infra == nil {
				return fmt.Errorf("no emobase-infra checkout found")
			}
			if !dryRun && !infraReady(cmd.Context(), ws) {
				return fmt.Errorf("infra is not running; start it first with: emobase up --only infra")
			}
			r := &scaffold.Runner{DryRun: dryRun, Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr()}
			out := cmd.OutOrStdout()
			for _, st := range ws.All() {
				if st.Role == workspace.RoleInfra {
					continue
				}
				fmt.Fprintln(out, title("Sync "+st.Name))
				if db := stackDatabase(st); db != "" {
					if err := r.CreateDatabase(ws.Infra.Dir, db); err != nil {
						return err
					}
					if !dryRun {
						if err := markDatabaseCreated(ws, st, true); err != nil {
							return err
						}
					}
				}
				if err := ensureStackRealm(cmd.Context(), r, ws, st); err != nil {
					return err
				}
				if !dryRun {
					if err := markRealmCreated(ws, st, true); err != nil {
						return err
					}
				}
			}
			fmt.Fprintln(out)
			fmt.Fprintf(out, "%s sync finished\n", ui.OK("OK"))
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print commands without running them")
	return cmd
}
