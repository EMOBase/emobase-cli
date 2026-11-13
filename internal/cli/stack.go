package cli

import (
	"fmt"

	"github.com/EMOBase/emobase-cli/internal/scaffold"
	"github.com/EMOBase/emobase-cli/internal/workspace"
	"github.com/spf13/cobra"
)

func selectStacks(ws *workspace.Workspace, only string) ([]*workspace.Stack, error) {
	switch only {
	case "", "all":
		return ws.All(), nil
	case "infra":
		if ws.Infra == nil {
			return nil, fmt.Errorf("no emobase-infra checkout found")
		}
		return []*workspace.Stack{ws.Infra}, nil
	case "portal":
		if ws.Portal == nil {
			return nil, fmt.Errorf("no emobase-portal-deploy checkout found")
		}
		return []*workspace.Stack{ws.Portal}, nil
	case "species":
		if len(ws.Species) == 0 {
			return nil, fmt.Errorf("no species checkouts found")
		}
		return ws.Species, nil
	default:
		return nil, fmt.Errorf("unknown --only %q (use infra, portal, species or all)", only)
	}
}

func (a *App) upCmd() *cobra.Command {
	var only string
	var pull, dryRun bool
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start stacks in dependency order (infra → portal → species)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			stacks, err := selectStacks(ws, only)
			if err != nil {
				return err
			}
			r := &scaffold.Runner{DryRun: dryRun, Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr()}
			for _, st := range stacks {
				if st.MissingEnv {
					return fmt.Errorf("%s has no .env; run emobase create or ./init.sh first", st.Dir)
				}
				fmt.Fprintln(cmd.OutOrStdout(), title("Start "+st.Name))
				if pull {
					if err := r.Pull(st.Dir); err != nil {
						return err
					}
				}
				if err := ensureStackRealm(cmd.Context(), r, ws, st); err != nil {
					return err
				}
				if st.Role == workspace.RolePortal {
					if err := ensurePortalDatabase(r, ws, st); err != nil {
						return err
					}
				}
				if err := r.Up(st.Dir); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&only, "only", "all", "infra, portal, species or all")
	cmd.Flags().BoolVar(&pull, "pull", false, "pull images before starting")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print commands without running them")
	return cmd
}

func (a *App) downCmd() *cobra.Command {
	var only string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop stacks in reverse order (species → portal → infra)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			stacks, err := selectStacks(ws, only)
			if err != nil {
				return err
			}
			// Stop dependants first.
			for i, j := 0, len(stacks)-1; i < j; i, j = i+1, j-1 {
				stacks[i], stacks[j] = stacks[j], stacks[i]
			}
			r := &scaffold.Runner{DryRun: dryRun, Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr()}
			for _, st := range stacks {
				fmt.Fprintln(cmd.OutOrStdout(), title("Stop "+st.Name))
				if err := r.Down(st.Dir); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&only, "only", "all", "infra, portal, species or all")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print commands without running them")
	return cmd
}

func (a *App) updateCmd() *cobra.Command {
	var only string
	var noGit, skipMigrations, skipSmoke, dryRun bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "git pull, pull images, migrate and restart in the documented order",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			stacks, err := selectStacks(ws, only)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			r := &scaffold.Runner{DryRun: dryRun, Stdout: out, Stderr: cmd.ErrOrStderr()}
			for _, st := range stacks {
				fmt.Fprintln(out, title("Update "+st.Name))
				if !noGit {
					if err := r.GitPull(st.Dir); err != nil {
						return err
					}
				}
				if err := r.Pull(st.Dir); err != nil {
					return err
				}
				if st.Role == workspace.RolePortal && !skipMigrations {
					if err := r.Migrate(st.Dir); err != nil {
						return err
					}
				}
				if err := r.Up(st.Dir); err != nil {
					return err
				}
				if st.Role == workspace.RolePortal && !skipSmoke {
					if err := r.SmokeTest(st.Dir); err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), muted("smoke test failed: "+err.Error()))
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&only, "only", "all", "infra, portal, species or all")
	cmd.Flags().BoolVar(&noGit, "no-git", false, "skip git pull")
	cmd.Flags().BoolVar(&skipMigrations, "skip-migrations", false, "skip portal migrations")
	cmd.Flags().BoolVar(&skipSmoke, "skip-smoke", false, "skip the portal smoke test")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print commands without running them")
	return cmd
}
