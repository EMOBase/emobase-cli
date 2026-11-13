package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/EMOBase/emobase-cli/internal/scaffold"
	"github.com/EMOBase/emobase-cli/internal/state"
	"github.com/EMOBase/emobase-cli/internal/ui"
	"github.com/EMOBase/emobase-cli/internal/workspace"
	"github.com/spf13/cobra"
)

type pruneAction struct {
	key   string
	db    string
	realm string
}

func (a *App) pruneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prune",
		Short: "Remove state-owned resources left by deleted stacks",
		Long: `prune removes databases and dedicated realms recorded in the workspace
ownership ledger (.emobase/state.json) for checkouts that are gone or marked
pending cleanup.

It never uses name patterns to guess ownership. Resources absent from the
ledger are left untouched. Shared realms are skipped while any active checkout
still uses them.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			ledger, err := state.Load(ws.Root)
			if err != nil {
				return err
			}
			actions, stale, err := planPrune(ws, ledger)
			if err != nil {
				return err
			}
			if len(actions) == 0 {
				for _, key := range stale {
					ledger.Delete(key)
				}
				if len(stale) > 0 {
					if err := ledger.Save(ws.Root); err != nil {
						return err
					}
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Nothing to prune.")
				return nil
			}

			out := cmd.OutOrStdout()
			fmt.Fprintln(out, title("Resources to prune"))
			dbCount, realmCount := 0, 0
			for _, act := range actions {
				if act.db != "" {
					dbCount++
					fmt.Fprintf(out, "  database  %-30s owner %s\n", act.db, act.key)
				}
				if act.realm != "" {
					realmCount++
					fmt.Fprintf(out, "  realm     %-30s owner %s\n", act.realm, act.key)
				}
			}
			if dbCount > 0 && !infraServiceRunning(cmd.Context(), ws, "mysql") {
				return fmt.Errorf("infra MySQL is not running; start it first with: emobase up --only infra")
			}
			if realmCount > 0 && !keycloakRunning(cmd.Context(), ws) {
				return fmt.Errorf("infra Keycloak is not running; start it first with: emobase up --only infra")
			}
			if !a.Yes {
				if !canPrompt(cmd.InOrStdin()) {
					return fmt.Errorf("refusing to prune without --yes (stdin is not a terminal)")
				}
				ok, err := promptConfirm(cmd, "Delete these resources? This cannot be undone.", false)
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(out, "Cancelled.")
					return nil
				}
			}

			r := &scaffold.Runner{Stdout: out, Stderr: cmd.ErrOrStderr()}
			for _, act := range actions {
				entry := ledger.Stacks[act.key]
				if act.db != "" {
					if err := r.DropDatabase(ws.Infra.Dir, act.db); err != nil {
						return err
					}
					if entry.Database != nil {
						entry.Database.Created = false
					}
				}
				if act.realm != "" {
					if err := r.DeleteRealm(ws.Infra.Dir, act.realm); err != nil {
						return err
					}
					if entry.Realm != nil {
						entry.Realm.Created = false
					}
				}
				if (entry.Database == nil || !entry.Database.Created) && (entry.Realm == nil || !entry.Realm.Created) {
					ledger.Delete(act.key)
				}
				if err := ledger.Save(ws.Root); err != nil {
					return err
				}
			}
			fmt.Fprintf(out, "%s prune finished\n", ui.OK("OK"))
			return nil
		},
	}
}

func planPrune(ws *workspace.Workspace, ledger *state.File) ([]pruneAction, []string, error) {
	keys := make([]string, 0, len(ledger.Stacks))
	for key := range ledger.Stacks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var actions []pruneAction
	var stale []string
	for _, key := range keys {
		entry := ledger.Stacks[key]
		dir := entry.Dir
		if dir != "" && !filepath.IsAbs(dir) {
			dir = filepath.Join(ws.Root, dir)
		}
		exists := false
		if dir != "" {
			if _, err := os.Stat(dir); err == nil {
				exists = true
			}
		}
		if exists && entry.Status != "pending_cleanup" {
			continue
		}
		act := pruneAction{key: key}
		if entry.Database != nil && entry.Database.Created {
			act.db = entry.Database.Name
		}
		if entry.Realm != nil && entry.Realm.Created && entry.Realm.Owner == key && !realmInUse(ws, ledger, key, entry.Realm.Name) {
			act.realm = entry.Realm.Name
		}
		if act.db == "" && act.realm == "" {
			stale = append(stale, key)
			continue
		}
		actions = append(actions, act)
	}
	return actions, stale, nil
}

func realmInUse(ws *workspace.Workspace, ledger *state.File, ownerKey, realm string) bool {
	if ws.Portal != nil {
		if r, _ := ws.Portal.Get("KEYCLOAK_REALM"); r == realm {
			return true
		}
	}
	for _, sp := range ws.Species {
		if r, _ := sp.Get("KEYCLOAK_REALM"); r == realm {
			return true
		}
	}
	for key, entry := range ledger.Stacks {
		if key == ownerKey || entry.Realm == nil {
			continue
		}
		if entry.Realm.Name == realm {
			return true
		}
	}
	return false
}
