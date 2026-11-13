// Package cli contains the emobase command tree.
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/EMOBase/emobase-cli/internal/workspace"
	"github.com/spf13/cobra"
)

// ErrFindings signals that checks failed in a way that should set exit code 1.
var ErrFindings = errors.New("findings")

// version is set at build time with -ldflags "-X .../internal/cli.version=v1.2.3".
var version = "dev"

// App holds global options shared by every command.
type App struct {
	WorkspaceFlag string
	JSON          bool
	NoColor       bool
	Yes           bool
}

// Workspace discovers the deployment checkouts.
func (a *App) Workspace() (*workspace.Workspace, error) {
	root, err := workspace.ResolveRoot(a.WorkspaceFlag)
	if err != nil {
		return nil, err
	}
	return workspace.Discover(root)
}

// New builds the root command.
func New() *cobra.Command {
	a := &App{}
	root := &cobra.Command{
		Use:     "emobase",
		Short:   "Manage EMOBase deployments across emobase-infra, the portal and species stacks",
		Version: version,
		Long: `emobase is the cross-repository control point for EMOBase deployments.

It scaffolds each stack's own .env with that repository's init.sh, validates the
shared configuration between emobase-infra, emobase-portal-deploy and every
emobase-deploy checkout, and runs the stacks in dependency order.

Each repository still owns its secrets and remains deployable on its own.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if a.NoColor || os.Getenv("NO_COLOR") != "" {
				setColor(false)
			} else {
				setColor(isTTY())
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.dashboard(cmd)
		},
	}
	pf := root.PersistentFlags()
	pf.StringVar(&a.WorkspaceFlag, "workspace", "", "directory containing the EMOBase checkouts (default: current directory or its parent)")
	pf.BoolVar(&a.JSON, "json", false, "emit machine-readable JSON")
	pf.BoolVar(&a.NoColor, "no-color", false, "disable ANSI colours")
	pf.BoolVarP(&a.Yes, "yes", "y", false, "answer yes to prompts and accept create defaults; enables choose-class fixes and destructive confirmations")

	root.AddCommand(
		a.doctorCmd(),
		a.statusCmd(),
		a.envCmd(),
		a.domainCmd(),
		a.speciesCmd(),
		a.createCmd(),
		a.upCmd(),
		a.downCmd(),
		a.updateCmd(),
		a.bootstrapCmd(),
		a.syncCmd(),
		a.pruneCmd(),
	)
	return root
}

// Execute runs the CLI and returns a process exit code.
func Execute() int {
	root := New()
	if err := root.Execute(); err != nil {
		if errors.Is(err, ErrFindings) {
			return 1
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}

func (a *App) dashboard(cmd *cobra.Command) error {
	ws, err := a.Workspace()
	if err != nil {
		return err
	}
	rep := runChecks(ws, false, cmd)
	out := cmd.OutOrStdout()
	if a.JSON {
		return a.writeJSON(out, ws, rep, nil)
	}
	fmt.Fprint(out, renderDashboard(ws, rep))
	if rep.HasErrors() {
		return ErrFindings
	}
	return nil
}
