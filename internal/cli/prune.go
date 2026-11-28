package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/EMOBase/emobase-cli/internal/checks"
	"github.com/EMOBase/emobase-cli/internal/scaffold"
	"github.com/EMOBase/emobase-cli/internal/state"
	"github.com/EMOBase/emobase-cli/internal/ui"
	"github.com/EMOBase/emobase-cli/internal/workspace"
	"github.com/spf13/cobra"
)

type pruneAction struct {
	key string
	db  string
	es  string
}

func (a *App) pruneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prune",
		Short: "Remove state-owned resources left by deleted stacks",
		Long: `prune removes databases and Elasticsearch index prefixes recorded in
the workspace ownership ledger (.emobase/state.json) for checkouts that are
gone or marked pending cleanup.

Keycloak realms are managed on the portal host and are never deleted here.`,
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
			actions, stale, blocked, err := planPrune(ws, ledger)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(actions) == 0 {
				for _, key := range stale {
					ledger.Delete(key)
				}
				if len(stale) > 0 {
					if err := ledger.Save(ws.Root); err != nil {
						return err
					}
				}
				for _, msg := range blocked {
					fmt.Fprintf(out, "%s %s\n", ui.Severity(checks.Warn), msg)
				}
				fmt.Fprintln(out, "Nothing to prune.")
				return nil
			}

			fmt.Fprintln(out, title("Resources to prune"))
			dbCount, esCount := 0, 0
			for _, act := range actions {
				if act.db != "" {
					dbCount++
					fmt.Fprintf(out, "  database  %-30s owner %s\n", act.db, act.key)
				}
				if act.es != "" {
					esCount++
					fmt.Fprintf(out, "  indices   %-30s owner %s\n", act.es+"-*", act.key)
				}
			}
			for _, msg := range blocked {
				fmt.Fprintf(out, "  %s %s\n", ui.Severity(checks.Warn), msg)
			}
			if dbCount > 0 && !infraServiceRunning(cmd.Context(), ws, "mysql") {
				return fmt.Errorf("infra MySQL is not running; start it first with: emobase up --only infra")
			}
			if esCount > 0 && !infraServiceRunning(cmd.Context(), ws, "elasticsearch") {
				return fmt.Errorf("infra Elasticsearch is not running; start it first with: emobase up --only infra")
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
				if act.es != "" {
					if err := r.DeleteESIndices(ws.Infra.Dir, act.es); err != nil {
						return err
					}
					if entry.Elasticsearch != nil {
						entry.Elasticsearch.Created = false
					}
				}
				if (entry.Database == nil || !entry.Database.Created) && (entry.Realm == nil || !entry.Realm.Created) && (entry.Elasticsearch == nil || !entry.Elasticsearch.Created) {
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

func planPrune(ws *workspace.Workspace, ledger *state.File) ([]pruneAction, []string, []string, error) {
	keys := make([]string, 0, len(ledger.Stacks))
	for key := range ledger.Stacks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var actions []pruneAction
	var stale []string
	var blocked []string
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
		ownedDB := entry.Database != nil && entry.Database.Created
		ownedES := entry.Elasticsearch != nil && entry.Elasticsearch.Created && entry.Elasticsearch.Prefix != ""
		if ownedDB {
			act.db = entry.Database.Name
		}
		if ownedES {
			if esPrefixInUse(ws, ledger, key, entry.Elasticsearch.Prefix) {
				blocked = append(blocked, fmt.Sprintf("index prefix %q is still used by an active checkout", entry.Elasticsearch.Prefix))
			} else {
				act.es = entry.Elasticsearch.Prefix
			}
		}
		if act.db == "" && act.es == "" {
			if ownedDB || ownedES {
				// The entry still owns a resource that could not be pruned;
				// keep it so a later prune can finish the cleanup.
				continue
			}
			stale = append(stale, key)
			continue
		}
		actions = append(actions, act)
	}
	return actions, stale, blocked, nil
}

func esPrefixInUse(ws *workspace.Workspace, ledger *state.File, ownerKey, prefix string) bool {
	active := func(st *workspace.Stack) bool {
		entry := ledger.Stacks[stackKey(st)]
		return entry == nil || entry.Status != "pending_cleanup"
	}
	for _, sp := range ws.Species {
		if !active(sp) {
			continue
		}
		if p := stackESPrefix(sp); p == prefix {
			return true
		}
	}
	for key, entry := range ledger.Stacks {
		if key == ownerKey || entry.Elasticsearch == nil || entry.Status == "pending_cleanup" {
			continue
		}
		if entry.Elasticsearch.Prefix == prefix {
			return true
		}
	}
	return false
}
