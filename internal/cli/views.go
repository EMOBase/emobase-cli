package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/EMOBase/emobase-cli/internal/checks"
	"github.com/EMOBase/emobase-cli/internal/ui"
	"github.com/EMOBase/emobase-cli/internal/workspace"
)

// The render functions below build complete screens from data. Commands write
// their result, and the dev preview renders the same functions with fixtures.

func renderDashboard(ws *workspace.Workspace, rep *checks.Report) string {
	var b strings.Builder
	fmt.Fprintln(&b, title("EMOBase workspace"))
	fmt.Fprintln(&b, muted("  "+ws.Root))
	fmt.Fprintln(&b)
	fmt.Fprint(&b, stackOverview(ws))
	fmt.Fprintln(&b)
	printSummary(&b, rep, false)
	if len(rep.Findings) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, title("Findings"))
		printFindings(&b, rep)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, muted(nextDashboardStep(rep)))
	return b.String()
}

// nextDashboardStep points at the command that can actually make progress.
// Offering `doctor --fix` when no finding carries a fix is misleading, so fall
// back to the status screen once the configuration has nothing left to apply.
func nextDashboardStep(rep *checks.Report) string {
	safe, choose := false, false
	for _, f := range rep.Findings {
		switch {
		case f.CanApply(false):
			safe = true
		case f.CanApply(true):
			choose = true
		}
	}
	switch {
	case safe:
		return "Next: emobase doctor --fix   (safe fixes)   |   emobase status"
	case choose:
		return "Next: emobase doctor --fix --yes   (choose fixes)   |   emobase status"
	default:
		return "Next: emobase status"
	}
}

func renderDoctor(rep *checks.Report, applied []checks.Change, withFix, yes, live bool) string {
	var b strings.Builder
	if withFix {
		fmt.Fprintln(&b, title("Applied fixes"))
		ui.Changes(&b, applied)
		fmt.Fprintln(&b)
	}
	printSummary(&b, rep, live)
	if len(rep.Findings) > 0 {
		fmt.Fprintln(&b)
		printFindings(&b, rep)
	}
	if !yes && hasChoose(rep) {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, muted("Some findings need a decision. Re-run with --fix --yes to accept emobase-infra's value,"))
		fmt.Fprintln(&b, muted("or edit the named .env by hand. Destructive issuer changes are never automatic."))
	}
	return b.String()
}

func renderStatus(sr statusReport) string {
	var b strings.Builder
	fmt.Fprintln(&b, title("Runtime status"))
	fmt.Fprintln(&b, muted("  "+sr.Workspace))
	fmt.Fprintln(&b)
	var rows [][]string
	for _, s := range sr.Stacks {
		health := "ok"
		switch {
		case s.Total == 0:
			health = "not created"
		case s.Detail != "":
			health = s.Detail
		case s.Unhealthy > 0:
			health = "unhealthy"
		case s.Running < s.Total:
			health = "partial"
		}
		rows = append(rows, []string{s.Role, s.Name, fmt.Sprintf("%d/%d", s.Running, s.Total), health, s.URL})
	}
	fmt.Fprint(&b, ui.Table([]string{"role", "checkout", "containers", "health", "url"}, rows))

	if !sr.Docker {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, ui.Severity(checks.Error)+" Docker daemon is not reachable")
		return b.String()
	}
	fmt.Fprintln(&b)
	netState := ui.OK("present")
	if !sr.NetworkOK {
		netState = ui.Severity(checks.Error) + " missing"
	}
	fmt.Fprint(&b, statusLine("network", fmt.Sprintf("%-28s %s", sr.Network, netState)))
	if sr.Databases != nil {
		fmt.Fprint(&b, statusLine("databases", fmt.Sprint(sr.Databases)))
	}
	realms := make([]string, 0, len(sr.Realms))
	for realm := range sr.Realms {
		realms = append(realms, realm)
	}
	sort.Strings(realms)
	for _, realm := range realms {
		state := ui.OK("reachable")
		if !sr.Realms[realm] {
			state = ui.Severity(checks.Error) + " unreachable"
		}
		fmt.Fprint(&b, statusLine("realm", fmt.Sprintf("%-28s %s", realm, state)))
	}
	if sr.Portal != "" {
		fmt.Fprint(&b, statusLine("portal", sr.Portal))
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, muted("Use emobase doctor --live for actionable checks."))
	return b.String()
}

// statusLine keeps the detail labels in one column so their values line up.
func statusLine(label, value string) string {
	return fmt.Sprintf("  %-9s  %s\n", label, value)
}

func speciesRows(ws *workspace.Workspace) []speciesRow {
	rows := make([]speciesRow, 0, len(ws.Species))
	for _, sp := range ws.Species {
		get := func(k string) string {
			if v, ok := sp.Get(k); ok {
				return v
			}
			return ""
		}
		rows = append(rows, speciesRow{
			Name: filepath.Base(sp.Dir), Species: get("MAIN_SPECIES"), Dir: sp.Dir,
			URL: get("PUBLIC_SERVER"), Realm: get("KEYCLOAK_REALM"), Database: get("DIRECTUS_DB_DATABASE"),
			Port: get("HTTP_PORT"), Project: sp.ProjectName, HasEnv: !sp.MissingEnv,
		})
	}
	return rows
}

func renderSpecies(rows []speciesRow) string {
	var b strings.Builder
	if len(rows) == 0 {
		fmt.Fprintln(&b, "No species checkouts found.")
		fmt.Fprintln(&b, muted("Add one with: emobase species add Tcas --public-server https://tcas.example.org --clone"))
		return b.String()
	}
	var table [][]string
	for _, r := range rows {
		table = append(table, []string{r.Species, filepath.Base(r.Dir), r.URL, r.Realm, r.Database, r.Port, r.Project})
	}
	fmt.Fprintln(&b, ui.Table([]string{"species", "checkout", "url", "realm", "database", "port", "project"}, table))
	return b.String()
}
