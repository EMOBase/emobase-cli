package cli

import (
	"strings"
	"testing"

	"github.com/EMOBase/emobase-cli/internal/checks"
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
	if portal.Elasticsearch != nil {
		t.Fatalf("portal must not own an Elasticsearch prefix: %#v", portal.Elasticsearch)
	}
	if portal.Realm == nil || portal.Realm.Owner != "portal" || portal.Realm.Created {
		t.Fatalf("portal realm = %#v", portal.Realm)
	}
	species := ledger.Stacks["species:emobase-deploy-tcas"]
	if species == nil || species.Elasticsearch == nil || species.Elasticsearch.Prefix != "tcas" || species.Elasticsearch.Created {
		t.Fatalf("species state = %#v", species)
	}
	if err := markDatabaseCreated(ws, ws.Portal, true); err != nil {
		t.Fatal(err)
	}
	if err := markRealmCreated(ws, ws.Portal, true); err != nil {
		t.Fatal(err)
	}
	if err := markESCreated(ws, ws.Species[0], true); err != nil {
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
	species = ledger.Stacks["species:emobase-deploy-tcas"]
	if !species.Elasticsearch.Created {
		t.Fatalf("es created flag = %#v", species.Elasticsearch)
	}
}

func TestStateFindingsHintMatchesRole(t *testing.T) {
	root := matrixFixture(t)
	ws, err := workspace.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := (&state.File{}).Save(root); err != nil {
		t.Fatal(err)
	}
	var rep checks.Report
	addStateFindings(ws, &rep)

	portalSeen, speciesSeen := false, false
	for _, f := range rep.Findings {
		if f.Scope == "infra" {
			t.Fatalf("infra needs no ownership entry, got finding: %#v", f)
		}
		if f.Scope == "portal" {
			portalSeen = true
			if strings.Contains(f.Hint, "species setup") {
				t.Fatalf("portal hint suggests species setup: %q", f.Hint)
			}
			if !strings.Contains(f.Hint, "emobase sync") {
				t.Fatalf("portal hint does not suggest sync: %q", f.Hint)
			}
		}
		if strings.HasPrefix(f.Scope, "species:") {
			speciesSeen = true
			if !strings.Contains(f.Hint, "species setup") {
				t.Fatalf("species hint does not suggest species setup: %q", f.Hint)
			}
		}
	}
	if !portalSeen || !speciesSeen {
		t.Fatalf("missing findings: portal=%v species=%v", portalSeen, speciesSeen)
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
				Elasticsearch: &state.Elasticsearch{
					Prefix:  "gone",
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
	if actions[0].db != "directus_gone" || actions[0].realm != "emobase-gone" || actions[0].es != "gone" {
		t.Fatalf("action = %#v", actions[0])
	}
}

func TestPlanPruneBlocksSharedESPrefix(t *testing.T) {
	root := t.TempDir()
	ledger := &state.File{
		Version: state.Version,
		Stacks: map[string]*state.Stack{
			"species:gone": {
				Role:   "species",
				Name:   "gone",
				Dir:    "gone",
				Status: "pending_cleanup",
				Elasticsearch: &state.Elasticsearch{
					Prefix:  "shared",
					Created: true,
				},
			},
			"species:live": {
				Role:          "species",
				Name:          "live",
				Dir:           "live",
				Status:        "active",
				Elasticsearch: &state.Elasticsearch{Prefix: "shared"},
			},
		},
	}
	actions, _, blocked, err := planPrune(&workspace.Workspace{Root: root}, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) == 0 {
		t.Fatal("expected the shared prefix to be blocked")
	}
	for _, act := range actions {
		if act.es != "" {
			t.Fatalf("shared prefix was scheduled for deletion: %#v", act)
		}
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
