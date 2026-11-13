package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/EMOBase/emobase-cli/internal/checks"
	"github.com/EMOBase/emobase-cli/internal/spec"
	"github.com/EMOBase/emobase-cli/internal/ui"
	"github.com/EMOBase/emobase-cli/internal/workspace"
	"github.com/spf13/cobra"
)

func isTTY() bool {
	st, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

func setColor(enabled bool) { ui.SetColor(enabled) }

func title(s string) string { return ui.Title(s) }
func muted(s string) string { return ui.Muted(s) }

func runChecks(ws *workspace.Workspace, withLive bool, cmd *cobra.Command) *checks.Report {
	rep := checks.Run(ws)
	if withLive {
		addLiveChecks(cmd.Context(), ws, rep)
	}
	rep.Sort()
	return rep
}

func stackOverview(ws *workspace.Workspace) string {
	var rows [][]string
	add := func(st *workspace.Stack, urlKey, realmKey, dbKey, portKey string) {
		env := func(k string) string {
			if k == "" || st.Env == nil {
				return "-"
			}
			if v, ok := st.Get(k); ok && v != "" {
				return v
			}
			return "-"
		}
		state := "ok"
		if st.MissingEnv {
			state = "no .env"
		}
		rows = append(rows, []string{string(st.Role), st.Name, env(urlKey), env(realmKey), env(dbKey), env(portKey), state})
	}
	if ws.Infra != nil {
		add(ws.Infra, "KEYCLOAK_PUBLIC_URL", "KEYCLOAK_REALM", "", "KEYCLOAK_HOST_PORT")
	}
	if ws.Portal != nil {
		add(ws.Portal, "PUBLIC_SERVER", "KEYCLOAK_REALM", "EMOBASE_GENOMICS_MYSQL__DATABASE", "HTTP_PORT")
	}
	for _, sp := range ws.Species {
		add(sp, "PUBLIC_SERVER", "KEYCLOAK_REALM", "DIRECTUS_DB_DATABASE", "HTTP_PORT")
	}
	return ui.Table([]string{"role", "checkout", "url", "realm", "database", "port", "env"}, rows)
}

func printSummary(out io.Writer, rep *checks.Report) {
	e, w, i := rep.Counts()
	switch {
	case e == 0 && w == 0:
		fmt.Fprintf(out, "%s configuration is consistent\n", ui.OK("OK"))
	default:
		fmt.Fprintf(out, "%d error(s), %d warning(s), %d notice(s)\n", e, w, i)
	}
}

func printFindings(out io.Writer, rep *checks.Report) {
	for _, f := range rep.Findings {
		fix := ""
		if f.Fix != checks.FixNone {
			fix = muted(" [" + f.Fix.String() + "]")
		}
		fmt.Fprintf(out, "  %s %-18s %s%s\n", ui.Severity(f.Severity), f.Scope, f.Message, fix)
		if f.Hint != "" && f.Severity >= checks.Warn {
			fmt.Fprintf(out, "       %s\n", ui.Hint(f.Hint))
		}
	}
}

type jsonFinding struct {
	Check    string `json:"check"`
	Scope    string `json:"scope"`
	Severity string `json:"severity"`
	Fix      string `json:"fix"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
}

type jsonReport struct {
	Workspace string          `json:"workspace"`
	Errors    int             `json:"errors"`
	Warnings  int             `json:"warnings"`
	Notices   int             `json:"notices"`
	Findings  []jsonFinding   `json:"findings"`
	Changes   []checks.Change `json:"changes,omitempty"`
}

func (a *App) writeJSON(out io.Writer, ws *workspace.Workspace, rep *checks.Report, changes []checks.Change) error {
	e, w, i := rep.Counts()
	jr := jsonReport{Workspace: ws.Root, Errors: e, Warnings: w, Notices: i, Changes: changes}
	if jr.Findings == nil {
		jr.Findings = []jsonFinding{}
	}
	for _, f := range rep.Findings {
		jr.Findings = append(jr.Findings, jsonFinding{
			Check: f.Check, Scope: f.Scope, Severity: f.Severity.String(),
			Fix: f.Fix.String(), Message: f.Message, Hint: f.Hint,
		})
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(jr)
}

// matrixRows builds the shared-key matrix for the env command.
func matrixRows(ws *workspace.Workspace) ([]string, [][]string) {
	stacks := ws.All()
	headers := []string{"key", "authority"}
	seenScope := map[string]int{}
	for _, st := range stacks {
		label := checks.ScopeOf(st)
		seenScope[label]++
		if seenScope[label] > 1 {
			label = label + "#" + strconv.Itoa(seenScope[label])
		}
		headers = append(headers, label)
	}
	var keys []struct {
		key  string
		auth spec.Authority
	}
	for _, k := range spec.SharedToInfra {
		keys = append(keys, struct {
			key  string
			auth spec.Authority
		}{k.Key, k.Authority})
	}
	var rows [][]string
	for _, k := range keys {
		row := []string{k.key, string(k.auth)}
		for _, st := range stacks {
			v, ok := st.Get(k.key)
			switch {
			case !ok || v == "":
				row = append(row, "—")
			case spec.IsSecret(k.key):
				row = append(row, spec.Mask(v))
			default:
				row = append(row, v)
			}
		}
		rows = append(rows, row)
	}
	return headers, rows
}
