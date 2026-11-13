package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/EMOBase/emobase-cli/internal/checks"
	"github.com/EMOBase/emobase-cli/internal/state"
	"github.com/EMOBase/emobase-cli/internal/workspace"
)

func stackKey(st *workspace.Stack) string {
	switch st.Role {
	case workspace.RoleInfra:
		return "infra"
	case workspace.RolePortal:
		return "portal"
	default:
		return "species:" + st.Name
	}
}

func stackDatabase(st *workspace.Stack) string {
	switch st.Role {
	case workspace.RolePortal:
		v, _ := st.Get("EMOBASE_GENOMICS_MYSQL__DATABASE")
		return v
	case workspace.RoleSpecies:
		v, _ := st.Get("DIRECTUS_DB_DATABASE")
		return v
	default:
		return ""
	}
}

func realmOwner(ws *workspace.Workspace, realm, owner string) string {
	if owner != "" {
		return owner
	}
	if ws.Portal != nil {
		if r, _ := ws.Portal.Get("KEYCLOAK_REALM"); r == realm {
			return "portal"
		}
	}
	for _, sp := range ws.Species {
		r, _ := sp.Get("KEYCLOAK_REALM")
		if r != realm {
			continue
		}
		mode, _ := sp.Get("KEYCLOAK_REALM_MODE")
		if mode == "dedicated" {
			return stackKey(sp)
		}
	}
	return ""
}

// syncStackLedger fills identity and desired-resource fields without changing
// existing created flags unless the desired resource name changed.
func syncStackLedger(root string, ws *workspace.Workspace, st *workspace.Stack, entry *state.Stack) {
	entry.Role = string(st.Role)
	entry.Name = st.Name
	entry.Dir = st.Dir
	if rel, err := filepath.Rel(root, st.Dir); err == nil && !strings.HasPrefix(rel, "..") {
		entry.Dir = rel
	}
	entry.ComposeProject = st.ProjectName
	if entry.Status == "" {
		entry.Status = "scaffolded"
	}

	if db := stackDatabase(st); db != "" {
		if entry.Database == nil || entry.Database.Name != db {
			entry.Database = &state.Database{Name: db}
		}
	} else {
		entry.Database = nil
	}

	realm, _ := st.Get("KEYCLOAK_REALM")
	if realm == "" {
		entry.Realm = nil
		return
	}
	mode, _ := st.Get("KEYCLOAK_REALM_MODE")
	if st.Role == workspace.RolePortal {
		mode = "portal"
	}
	owner := ""
	if entry.Realm != nil && entry.Realm.Name == realm {
		owner = entry.Realm.Owner
	}
	owner = realmOwner(ws, realm, owner)
	if entry.Realm == nil || entry.Realm.Name != realm {
		entry.Realm = &state.Realm{Name: realm}
	}
	entry.Realm.Name = realm
	entry.Realm.Mode = mode
	entry.Realm.Owner = owner
}

func updateStackLedger(ws *workspace.Workspace, st *workspace.Stack, mutate func(*state.Stack)) error {
	f, err := state.Load(ws.Root)
	if err != nil {
		return err
	}
	entry := f.Entry(stackKey(st))
	syncStackLedger(ws.Root, ws, st, entry)
	if mutate != nil {
		mutate(entry)
	}
	return f.Save(ws.Root)
}

func markDatabaseCreated(ws *workspace.Workspace, st *workspace.Stack, created bool) error {
	return updateStackLedger(ws, st, func(e *state.Stack) {
		if e.Database != nil {
			e.Database.Created = created
		}
	})
}

func markRealmCreated(ws *workspace.Workspace, st *workspace.Stack, created bool) error {
	return updateStackLedger(ws, st, func(e *state.Stack) {
		if e.Realm != nil {
			e.Realm.Created = created
		}
	})
}

func markStackStatus(ws *workspace.Workspace, st *workspace.Stack, status string) error {
	return updateStackLedger(ws, st, func(e *state.Stack) { e.Status = status })
}

func stateRecordHint(st *workspace.Stack) string {
	switch st.Role {
	case workspace.RolePortal:
		return "run `emobase sync` to record the portal database and realm"
	case workspace.RoleSpecies:
		return fmt.Sprintf("run `emobase species setup %s` or `emobase sync` to record it", st.Name)
	default:
		return "run `emobase sync` to record it"
	}
}

func addStateFindings(ws *workspace.Workspace, rep *checks.Report) {
	_, err := os.Stat(state.Path(ws.Root))
	if os.IsNotExist(err) {
		rep.Add(checks.Advisory("state.missing", "workspace", checks.Warn,
			"workspace ownership ledger not found",
			"run `emobase sync` to record existing resources, or `emobase bootstrap` for a full bring-up"))
		return
	}
	if err != nil {
		rep.Add(checks.Advisory("state.unreadable", "workspace", checks.Warn,
			"workspace ownership ledger is unreadable", err.Error()))
		return
	}
	ledger, err := state.Load(ws.Root)
	if err != nil {
		rep.Add(checks.Advisory("state.invalid", "workspace", checks.Warn,
			"workspace ownership ledger is invalid", err.Error()))
		return
	}
	active := append([]*workspace.Stack{}, ws.All()...)
	for _, st := range active {
		// Infra owns no database or realm, so it needs no ownership entry.
		if st.Role == workspace.RoleInfra {
			continue
		}
		key := stackKey(st)
		entry := ledger.Stacks[key]
		if entry == nil || entry.Status == "pending_cleanup" {
			rep.Add(checks.Advisory("state.unrecorded", checks.ScopeOf(st), checks.Warn,
				fmt.Sprintf("%s is not recorded as active in .emobase/state.json", st.Name),
				stateRecordHint(st)))
			continue
		}
		if db := stackDatabase(st); db != "" && entry.Database != nil && entry.Database.Name != db {
			rep.Add(checks.Advisory("state.database_mismatch", checks.ScopeOf(st), checks.Warn,
				fmt.Sprintf("state records database %q but .env says %q", entry.Database.Name, db),
				"rerun the stack setup or fix the state ledger manually"))
		}
		realm, _ := st.Get("KEYCLOAK_REALM")
		if realm != "" && entry.Realm != nil && entry.Realm.Name != realm {
			rep.Add(checks.Advisory("state.realm_mismatch", checks.ScopeOf(st), checks.Warn,
				fmt.Sprintf("state records realm %q but .env says %q", entry.Realm.Name, realm),
				"rerun the stack setup or fix the state ledger manually"))
		}
	}
	for key, entry := range ledger.Stacks {
		if entry.Status != "pending_cleanup" {
			continue
		}
		dir := entry.Dir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(ws.Root, dir)
		}
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			rep.Add(checks.Advisory("state.pending_cleanup", key, checks.Warn,
				"resources from a removed checkout are pending cleanup",
				"run `emobase prune`"))
		}
	}
}

func removeStackLedger(ws *workspace.Workspace, st *workspace.Stack) error {
	f, err := state.Load(ws.Root)
	if err != nil {
		return err
	}
	f.Delete(stackKey(st))
	return f.Save(ws.Root)
}
