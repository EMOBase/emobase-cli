package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/EMOBase/emobase-cli/internal/workspace"
	"github.com/spf13/cobra"
)

func (a *App) envCmd() *cobra.Command {
	env := &cobra.Command{
		Use:   "env",
		Short: "Inspect the environment files shared by every stack",
	}
	env.AddCommand(a.matrixCmd())
	return env
}

type jsonMatrix struct {
	Workspace string     `json:"workspace"`
	Headers   []string   `json:"headers"`
	Rows      [][]string `json:"rows"`
}

func writeMatrixJSON(out io.Writer, ws *workspace.Workspace, headers []string, rows [][]string) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(jsonMatrix{Workspace: ws.Root, Headers: headers, Rows: rows})
}

func (a *App) matrixCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "matrix",
		Aliases: []string{"show"},
		Short:   "Show the shared-key matrix across infra, portal and species",
		Long: `matrix prints one row per shared key and one column per stack. A value of —
means the key is absent. Secrets are masked; the authority column names the
stack whose value wins after "emobase doctor --fix".`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			headers, rows := matrixRows(ws)
			out := cmd.OutOrStdout()
			if a.JSON {
				return writeMatrixJSON(out, ws, headers, rows)
			}
			fmt.Fprint(out, renderMatrix(headers, rows))
			return nil
		},
	}
}
