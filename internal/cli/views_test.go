package cli

import (
	"strings"
	"testing"

	"github.com/EMOBase/emobase-cli/internal/checks"
)

func TestDashboardDoesNotOfferFixWithoutFindings(t *testing.T) {
	ws := healthyWorkspace()
	out := renderDashboard(ws, previewReport(ws))
	if strings.Contains(out, "doctor --fix") {
		t.Errorf("clean dashboard offers doctor --fix:\n%s", out)
	}
	if !strings.Contains(out, "Next: emobase status") {
		t.Errorf("clean dashboard has no next step:\n%s", out)
	}
}

func TestDashboardOffersFixForApplicableFindings(t *testing.T) {
	ws := findingsWorkspace()
	out := renderDashboard(ws, previewReport(ws))
	if !strings.Contains(out, "Next: emobase doctor --fix") {
		t.Errorf("dashboard does not offer safe fixes:\n%s", out)
	}
}

func TestNextDashboardStepForChooseOnly(t *testing.T) {
	rep := &checks.Report{Findings: []checks.Finding{
		checks.Action("test.choose", "workspace", checks.Error, checks.FixChoose, "mismatch", "",
			func() ([]checks.Change, error) { return nil, nil }),
	}}
	if got := nextDashboardStep(rep); !strings.Contains(got, "doctor --fix --yes") {
		t.Fatalf("next step = %q", got)
	}
}

func TestStatusDetailValuesAlign(t *testing.T) {
	out := renderStatus(statusReport{
		Workspace: "/srv/emobase",
		Docker:    true,
		Network:   "emobase-infra",
		NetworkOK: true,
		Databases: []string{"directus_tcas"},
		Realms:    map[string]bool{"emobase-tcas": true},
		Portal:    "HTTP 200",
	})
	values := map[string]string{
		"network":   "emobase-infra",
		"databases": "[directus_tcas]",
		"realm":     "emobase-tcas",
		"portal":    "HTTP 200",
	}
	col := -1
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		value, ok := values[fields[0]]
		if !ok {
			continue
		}
		idx := strings.Index(line, value)
		if idx < 0 {
			t.Fatalf("%q not found in %q", value, line)
		}
		if col == -1 {
			col = idx
			continue
		}
		if idx != col {
			t.Errorf("%q starts at column %d, want %d:\n%s", fields[0], idx, col, out)
		}
	}
	if col == -1 {
		t.Fatalf("no detail lines in output:\n%s", out)
	}
}
