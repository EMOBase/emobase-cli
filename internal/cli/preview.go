package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/EMOBase/emobase-cli/internal/ui"
)

// previewCase is one renderable UI state. The dev preview binary lists these
// and renders them without touching a workspace or Docker.
type previewCase struct {
	Name        string
	Group       string
	Description string
	Render      func() string
}

const previewUsage = `Usage:
  preview list                 list every case
  preview <case>               render one case
  preview --all                render every case
  preview --no-color           disable ANSI colours

Flags may appear before or after the case name.
`

// RunPreview implements the dev-only preview command. It is not linked into
// the emobase binary; see cmd/preview.
func RunPreview(args []string, stdout, stderr io.Writer) int {
	noColor, all := false, false
	name := ""

	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			fmt.Fprint(stdout, previewUsage)
			return 0
		case "--no-color":
			noColor = true
		case "--all":
			all = true
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(stderr, "preview: unknown flag %q\n\n", arg)
				fmt.Fprint(stderr, previewUsage)
				return 2
			}
			if name != "" {
				fmt.Fprintln(stderr, "preview: render one case at a time (use --all for every case)")
				return 2
			}
			name = arg
		}
	}

	ui.SetColor(!noColor)
	cases := previewCases()

	if all {
		for i, c := range cases {
			if i > 0 {
				fmt.Fprintln(stdout)
			}
			fmt.Fprintf(stdout, "%s  %s\n\n", title(c.Group+"/"+c.Name), muted(c.Description))
			fmt.Fprint(stdout, c.Render())
		}
		return 0
	}

	if name == "" || name == "list" || name == "ls" {
		printPreviewCases(stdout, cases)
		return 0
	}

	for _, c := range cases {
		if c.Name == name {
			fmt.Fprint(stdout, c.Render())
			return 0
		}
	}

	fmt.Fprintf(stderr, "preview: unknown case %q\n\n", name)
	printPreviewCases(stderr, cases)
	return 2
}

func printPreviewCases(out io.Writer, cases []previewCase) {
	group := ""
	for _, c := range cases {
		if c.Group != group {
			if group != "" {
				fmt.Fprintln(out)
			}
			fmt.Fprintln(out, c.Group)
			group = c.Group
		}
		fmt.Fprintf(out, "  %-24s %s\n", c.Name, c.Description)
	}
}
