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
