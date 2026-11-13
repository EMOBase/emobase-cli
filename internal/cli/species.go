package cli

import (
	"encoding/json"
	"fmt"

	"github.com/EMOBase/emobase-cli/internal/scaffold"
	"github.com/EMOBase/emobase-cli/internal/state"
	"github.com/EMOBase/emobase-cli/internal/ui"
	"github.com/EMOBase/emobase-cli/internal/workspace"
	"github.com/spf13/cobra"
)

func (a *App) speciesCmd() *cobra.Command {
	sp := &cobra.Command{
		Use:   "species",
		Short: "List, add, set up and remove species stacks",
	}
	sp.AddCommand(a.speciesListCmd(), a.newSpeciesAddCmd("add"), a.speciesSetupCmd(), a.speciesRemoveCmd())
	return sp
}

type speciesRow struct {
	Name     string `json:"checkout"`
	Species  string `json:"species"`
	Dir      string `json:"dir"`
	URL      string `json:"url"`
	Realm    string `json:"realm"`
	Database string `json:"database"`
	Port     string `json:"http_port"`
	Project  string `json:"compose_project"`
	HasEnv   bool   `json:"has_env"`
}

func (a *App) speciesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List the species checkouts discovered on disk",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			rows := speciesRows(ws)
			if a.JSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			fmt.Fprint(out, renderSpecies(rows))
			return nil
		},
	}
}

func (a *App) speciesSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup <species|checkout>",
		Short: "Create the database, register the realm origin, initialise Directus and start",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			if ws.Infra == nil {
				return fmt.Errorf("no emobase-infra checkout found")
			}
			sp := ws.FindSpecies(args[0])
			if sp == nil {
				return fmt.Errorf("no species checkout matches %q (try emobase species list)", args[0])
			}
			return a.setupSpecies(cmd, ws, sp, &scaffold.Runner{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr()})
		},
	}
}

func (a *App) speciesRemoveCmd() *cobra.Command {
	var purge, keepDir bool
	cmd := &cobra.Command{
		Use:   "remove <species|checkout>",
		Short: "Stop a species stack, delete its data and remove the checkout",
		Long: `Without --purge, remove only prints the exact commands. With --purge --yes it
stops the stack, deletes its Compose volumes, drops the Directus database and
removes the checkout directory. Use --keep-dir to keep the directory.

A dedicated Keycloak realm is cleaned up later by 'emobase prune'.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			sp := ws.FindSpecies(args[0])
			if sp == nil {
				return fmt.Errorf("no species checkout matches %q (try emobase species list)", args[0])
			}
			out := cmd.OutOrStdout()
			db, _ := sp.Get("DIRECTUS_DB_DATABASE")
			if !purge {
				fmt.Fprintf(out, "To remove %s:\n", sp.Name)
				fmt.Fprintf(out, "  cd %s && docker compose --profile '*' down -v\n", sp.Dir)
				if ws.Infra != nil && db != "" {
					fmt.Fprintf(out, "  %s/scripts/drop-database.sh %s\n", ws.Infra.Dir, db)
				}
				if !keepDir {
					fmt.Fprintf(out, "  rm -rf %s\n", sp.Dir)
				}
				fmt.Fprintln(out)
				fmt.Fprintln(out, muted("Re-run with --purge --yes to run these commands."))
				if keepDir {
					fmt.Fprintln(out, muted("The checkout directory will be kept."))
				} else {
					fmt.Fprintln(out, muted("Add --keep-dir to keep the checkout directory."))
				}
				return nil
			}
			if !a.Yes {
				return fmt.Errorf("refusing to destroy %s without --yes", sp.Name)
			}
			ledger, err := state.Load(ws.Root)
			if err != nil {
				return err
			}
			key := stackKey(sp)
			entry := ledger.Stacks[key]
			legacy := entry == nil
			if legacy {
				entry = ledger.Entry(key)
				syncStackLedger(ws.Root, ws, sp, entry)
				// Without a ledger entry we cannot prove the resource was never
				// created. Adopt it so a later prune can clean it up safely.
				if entry.Database != nil {
					entry.Database.Created = true
				}
				if entry.Realm != nil && entry.Realm.Owner == key {
					entry.Realm.Created = true
				}
			}
			if db == "" && entry.Database != nil {
				db = entry.Database.Name
			}
			dbCreated := entry.Database != nil && entry.Database.Created
			realmOwned := entry.Realm != nil && entry.Realm.Created && entry.Realm.Owner == key

			r := &scaffold.Runner{Stdout: out, Stderr: cmd.ErrOrStderr()}
			if err := r.Destroy(sp.Dir); err != nil {
				return err
			}

			pending := false
			if db != "" && dbCreated {
				if ws.Infra == nil || !infraServiceRunning(cmd.Context(), ws, "mysql") {
					pending = true
					fmt.Fprintln(out, muted("Database cleanup deferred: infra MySQL is not running."))
				} else if err := r.DropDatabase(ws.Infra.Dir, db); err != nil {
					pending = true
					fmt.Fprintln(out, muted("Database cleanup deferred: "+err.Error()))
				} else if entry.Database != nil {
					entry.Database.Created = false
				}
			}
			if realmOwned && entry.Realm != nil && entry.Realm.Mode == "dedicated" {
				pending = true
				fmt.Fprintln(out, muted("Dedicated realm cleanup deferred to emobase prune."))
			}

			if !keepDir {
				if err := workspace.SafeRemoveDir(ws.Root, sp.Dir); err != nil {
					return err
				}
			}

			if pending {
				entry.Status = "pending_cleanup"
				if err := ledger.Save(ws.Root); err != nil {
					return err
				}
				if keepDir {
					fmt.Fprintf(out, "%s stopped %s; the directory %s was kept and cleanup is pending. Run: emobase prune\n", ui.OK("OK"), sp.Name, sp.Dir)
				} else {
					fmt.Fprintf(out, "%s removed %s (%s); cleanup is pending. Run: emobase prune\n", ui.OK("OK"), sp.Name, sp.Dir)
				}
				return nil
			}
			ledger.Delete(key)
			if err := ledger.Save(ws.Root); err != nil {
				return err
			}
			if keepDir {
				fmt.Fprintf(out, "%s stopped %s; the directory %s was kept\n", ui.OK("OK"), sp.Name, sp.Dir)
			} else {
				fmt.Fprintf(out, "%s removed %s (%s)\n", ui.OK("OK"), sp.Name, sp.Dir)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "stop the stack, delete its volumes, drop the database and remove the checkout")
	cmd.Flags().BoolVar(&keepDir, "keep-dir", false, "keep the checkout directory after purging")
	return cmd
}
