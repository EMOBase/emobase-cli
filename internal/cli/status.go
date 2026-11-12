package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/EMOBase/emobase-cli/internal/checks"
	"github.com/EMOBase/emobase-cli/internal/keycloak"
	"github.com/EMOBase/emobase-cli/internal/live"
	"github.com/EMOBase/emobase-cli/internal/ui"
	"github.com/EMOBase/emobase-cli/internal/workspace"
	"github.com/spf13/cobra"
)

type statusStack struct {
	Role      string `json:"role"`
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	Running   int    `json:"running"`
	Total     int    `json:"total"`
	Unhealthy int    `json:"unhealthy"`
	URL       string `json:"url,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

type statusReport struct {
	Workspace string          `json:"workspace"`
	Docker    bool            `json:"docker"`
	Stacks    []statusStack   `json:"stacks"`
	Network   string          `json:"network,omitempty"`
	NetworkOK bool            `json:"network_ok"`
	Databases []string        `json:"databases,omitempty"`
	Realms    map[string]bool `json:"realms,omitempty"`
	Portal    string          `json:"portal_status,omitempty"`
}

func (a *App) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the live state of containers, databases, realms and URLs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			sr := a.collectStatus(ctx, ws)
			out := cmd.OutOrStdout()
			if a.JSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				if err := enc.Encode(sr); err != nil {
					return err
				}
				if !sr.Docker {
					return ErrFindings
				}
				return nil
			}
			fmt.Fprintln(out, title("Runtime status"))
			fmt.Fprintln(out, muted("  "+ws.Root))
			fmt.Fprintln(out)
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
			fmt.Fprint(out, ui.Table([]string{"role", "checkout", "containers", "health", "url"}, rows))

			if !sr.Docker {
				fmt.Fprintln(out)
				fmt.Fprintln(out, ui.Severity(checks.Error)+" Docker daemon is not reachable")
				return ErrFindings
			}
			fmt.Fprintln(out)
			netState := ui.OK("present")
			if !sr.NetworkOK {
				netState = ui.Severity(checks.Error) + " missing"
			}
			fmt.Fprintf(out, "  network %-28s %s\n", sr.Network, netState)
			if sr.Databases != nil {
				fmt.Fprintf(out, "  databases                    %v\n", sr.Databases)
			}
			for realm, ok := range sr.Realms {
				state := ui.OK("reachable")
				if !ok {
					state = ui.Severity(checks.Error) + " unreachable"
				}
				fmt.Fprintf(out, "  realm %-26s %s\n", realm, state)
			}
			if sr.Portal != "" {
				fmt.Fprintf(out, "  portal                       %s\n", sr.Portal)
			}
			fmt.Fprintln(out)
			fmt.Fprintln(out, muted("Use emobase doctor --live for actionable checks."))
			return nil
		},
	}
}

func (a *App) collectStatus(ctx context.Context, ws *workspace.Workspace) statusReport {
	sr := statusReport{Workspace: ws.Root, Realms: map[string]bool{}}
	if !live.DockerAvailable(ctx) {
		for _, st := range ws.All() {
			sr.Stacks = append(sr.Stacks, statusStack{Role: string(st.Role), Name: st.Name, Dir: st.Dir, Detail: "docker unavailable"})
		}
		return sr
	}
	sr.Docker = true
	for _, st := range ws.All() {
		row := statusStack{Role: string(st.Role), Name: st.Name, Dir: st.Dir}
		row.URL, _ = st.Get("PUBLIC_SERVER")
		if st.Role == workspace.RoleInfra {
			row.URL, _ = st.Get("KEYCLOAK_PUBLIC_URL")
		}
		containers, err := live.ComposePS(ctx, st.Dir)
		if err != nil {
			row.Detail = "not created"
		} else {
			for _, c := range containers {
				row.Total++
				state := c.State
				if state == "" {
					state = c.Status
				}
				if isRunning(state, c.Status) {
					row.Running++
				}
				if c.Health == "unhealthy" {
					row.Unhealthy++
				}
			}
		}
		sr.Stacks = append(sr.Stacks, row)
	}

	if ws.Infra != nil {
		sr.Network, _ = ws.Infra.Get("EMOBASE_INFRA_NETWORK")
		if ok, err := live.NetworkExists(ctx, sr.Network); err == nil {
			sr.NetworkOK = ok
		}
		if dbs, err := live.Databases(ctx, ws.Infra.Dir); err == nil {
			sr.Databases = dbs
		} else {
			sr.Databases = nil
		}
		if base, ok := ws.Infra.Get("KEYCLOAK_PUBLIC_URL"); ok && base != "" {
			kc := keycloak.New(base, "", "")
			realms := map[string]bool{}
			if r, ok := ws.Infra.Get("KEYCLOAK_REALM"); ok && r != "" {
				realms[r] = true
			}
			for _, sp := range ws.Species {
				if r, ok := sp.Get("KEYCLOAK_REALM"); ok && r != "" {
					realms[r] = true
				}
			}
			for r := range realms {
				ok, err := kc.RealmReachable(ctx, r)
				sr.Realms[r] = ok && err == nil
			}
		}
	}
	if ws.Portal != nil {
		if u, ok := ws.Portal.Get("PUBLIC_SERVER"); ok && u != "" {
			if code, err := live.HTTPStatus(ctx, u); err == nil {
				sr.Portal = fmt.Sprintf("HTTP %d", code)
			} else {
				sr.Portal = "unreachable"
			}
		}
	}
	return sr
}

func isRunning(state, status string) bool {
	return state == "running" || len(status) >= 2 && (status[:2] == "Up")
}
