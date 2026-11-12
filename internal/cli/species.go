package cli

import (
	"encoding/json"
	"fmt"

	"github.com/EMOBase/emobase-cli/internal/scaffold"
	"github.com/EMOBase/emobase-cli/internal/ui"
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
	var purge bool
	cmd := &cobra.Command{
		Use:   "remove <species|checkout>",
		Short: "Stop a species stack and its data",
		Long: `Without --purge, remove only prints the exact commands. With --purge --yes it
stops the stack and deletes its Compose volumes and Directus database. The
checkout directory is never deleted by this command.`,
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
				fmt.Fprintf(out, "  rm -rf %s\n", sp.Dir)
				fmt.Fprintln(out)
				fmt.Fprintln(out, muted("Re-run with --purge --yes to run the first two commands."))
				return nil
			}
			if !a.Yes {
				return fmt.Errorf("refusing to destroy %s without --yes", sp.Name)
			}
			r := &scaffold.Runner{Stdout: out, Stderr: cmd.ErrOrStderr()}
			if err := r.Destroy(sp.Dir); err != nil {
				return err
			}
			if ws.Infra != nil && db != "" {
				if err := r.DropDatabase(ws.Infra.Dir, db); err != nil {
					return err
				}
			}
			fmt.Fprintf(out, "%s stopped %s; the directory %s was kept\n", ui.OK("OK"), sp.Name, sp.Dir)
			return nil
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "stop the stack, delete its volumes and drop the database")
	return cmd
}
