package cli

import (
	"testing"

	"github.com/EMOBase/emobase-cli/internal/state"
	"github.com/EMOBase/emobase-cli/internal/workspace"
)

func TestStackLedgerTracksResources(t *testing.T) {
	root := matrixFixture(t)
	ws, err := workspace.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := updateStackLedger(ws, ws.Portal, nil); err != nil {
		t.Fatal(err)
	}
	if err := updateStackLedger(ws, ws.Species[0], nil); err != nil {
		t.Fatal(err)
	}
	ledger, err := state.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	portal := ledger.Stacks["portal"]
	if portal == nil || portal.Database == nil || portal.Database.Name != "emobase_genomics" {
		t.Fatalf("portal state = %#v", portal)
	}
	if portal.Realm == nil || portal.Realm.Owner != "portal" || portal.Realm.Created {
		t.Fatalf("portal realm = %#v", portal.Realm)
	}
	if err := markDatabaseCreated(ws, ws.Portal, true); err != nil {
		t.Fatal(err)
	}
	if err := markRealmCreated(ws, ws.Portal, true); err != nil {
		t.Fatal(err)
	}
	ledger, err = state.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	portal = ledger.Stacks["portal"]
	if !portal.Database.Created || !portal.Realm.Created {
		t.Fatalf("created flags = %#v", portal)
	}
}

func TestPlanPruneOnlyUsesLedgerResources(t *testing.T) {
	root := t.TempDir()
	ledger := &state.File{
		Version: state.Version,
		Stacks: map[string]*state.Stack{
			"species:gone": {
				Role:   "species",
				Name:   "gone",
				Dir:    "gone",
				Status: "pending_cleanup",
				Database: &state.Database{
					Name:    "directus_gone",
					Created: true,
				},
				Realm: &state.Realm{
					Name:    "emobase-gone",
					Mode:    "dedicated",
					Owner:   "species:gone",
					Created: true,
				},
			},
		},
	}
	ws := &workspace.Workspace{Root: root}
	actions, stale, _, err := planPrune(ws, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 0 || len(actions) != 1 {
		t.Fatalf("actions = %#v, stale = %#v", actions, stale)
	}
	if actions[0].db != "directus_gone" || actions[0].realm != "emobase-gone" {
		t.Fatalf("action = %#v", actions[0])
	}
}

func TestPlanPruneKeepsSharedRealm(t *testing.T) {
	root := t.TempDir()
	ledger := &state.File{
		Version: state.Version,
		Stacks: map[string]*state.Stack{
			"portal": {
				Role:   "portal",
				Name:   "portal",
				Dir:    "portal",
				Status: "pending_cleanup",
				Realm: &state.Realm{
					Name:    "emobase-portal",
					Mode:    "portal",
					Owner:   "portal",
					Created: true,
				},
			},
			"species:joined": {
				Role:   "species",
				Name:   "joined",
				Dir:    "joined",
				Status: "active",
				Realm: &state.Realm{
					Name:  "emobase-portal",
					Mode:  "joined",
					Owner: "portal",
				},
			},
		},
	}
	actions, _, blocked, err := planPrune(&workspace.Workspace{Root: root}, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) == 0 {
		t.Fatal("expected the shared realm to be blocked")
	}
	for _, act := range actions {
		if act.realm != "" {
			t.Fatalf("shared realm was scheduled for deletion: %#v", act)
		}
	}
}
