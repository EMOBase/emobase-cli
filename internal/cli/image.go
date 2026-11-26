package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/EMOBase/emobase-cli/internal/ui"
	"github.com/EMOBase/emobase-cli/internal/workspace"
	"github.com/spf13/cobra"
)

const (
	imageSourceEnv     = "env"
	imageSourceDefault = "default"
	imageSourceUnset   = "unset"

	imageGroupApp     = "app"
	imageGroupService = "service"
)

// imageTarget is one image the CLI knows how to inspect and change. The
// variable belongs to the stack's own .env, so the CLI never stores the value.
// ComposeFile is set for an image defined by an optional overlay: the target is
// only available on stacks whose COMPOSE_FILE includes it, and its fallback is
// read from that file.
type imageTarget struct {
	Name        string
	Variable    string
	Role        workspace.Role
	Group       string
	ComposeFile string
	Aliases     []string
}

var managedImages = []imageTarget{
	{Name: "genomics", Variable: "EMOBASE_GENOMICS_IMAGE", Role: workspace.RolePortal, Group: imageGroupApp,
		Aliases: []string{"emobase-genomics", "genomicsservice"}},
	{Name: "portal-web", Variable: "EMOBASE_PORTAL_WEB_IMAGE", Role: workspace.RolePortal, Group: imageGroupApp,
		Aliases: []string{"portalweb", "emobase-portal-web", "portal"}},
	{Name: "species-web", Variable: "EMOBASE_WEB_IMAGE", Role: workspace.RoleSpecies, Group: imageGroupApp,
		Aliases: []string{"web", "emobase-web", "species"}},

	{Name: "ontologyservice", Variable: "ONTOLOGYSERVICE_IMAGE", Role: workspace.RoleSpecies, Group: imageGroupService,
		Aliases: []string{"ontology", "ontology-service"}},
	{Name: "imageservice", Variable: "IMAGESERVICE_IMAGE", Role: workspace.RoleSpecies, Group: imageGroupService,
		Aliases: []string{"image-service"}},
	{Name: "phenotypeservice", Variable: "PHENOTYPESERVICE_IMAGE", Role: workspace.RoleSpecies, Group: imageGroupService,
		Aliases: []string{"phenotype", "phenotype-service"}},
	{Name: "goannotationservice", Variable: "GOANNOTATIONSERVICE_IMAGE", Role: workspace.RoleSpecies, Group: imageGroupService,
		Aliases: []string{"goannotation", "go-annotation", "go-annotation-service"}},
	{Name: "publicationservice", Variable: "PUBLICATIONSERVICE_IMAGE", Role: workspace.RoleSpecies, Group: imageGroupService,
		Aliases: []string{"publication", "publication-service"}},
	{Name: "oba", Variable: "OBA_IMAGE", Role: workspace.RoleSpecies, Group: imageGroupService,
		ComposeFile: "compose.tcas.yml", Aliases: []string{"oba-image"}},
}

func findImageTarget(name string) (imageTarget, bool) {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, t := range managedImages {
		if lower == t.Name {
			return t, true
		}
		for _, alias := range t.Aliases {
			if lower == alias {
				return t, true
			}
		}
	}
	return imageTarget{}, false
}

func imageTargetNames() string {
	names := make([]string, len(managedImages))
	for i, t := range managedImages {
		names[i] = t.Name
	}
	return strings.Join(names, ", ")
}

func unknownImageTarget(name string) error {
	return fmt.Errorf("unknown image target %q (use %s)", name, imageTargetNames())
}

func (a *App) imageCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:     "image",
		Aliases: []string{"images"},
		Short:   "View and change the images the portal and species stacks run",
		Long: `image shows the effective container image for each portal and species stack.

The value comes from the stack's .env when it is set, and otherwise from the
Compose file's ${VAR:-default} fallback. The default view lists the main app
images plus any supporting service image with an explicit override; --all also
lists supporting images that still use their defaults.

set pins an image in .env; reset removes the override again, returning the stack
to the Compose default. The command edits .env but does not recreate containers.
Run emobase up (or emobase up --only portal|species) to apply a change.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runImageList(cmd, all)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include supporting service images that use their defaults")

	cmd.AddCommand(a.imageListCmd(), a.imageSetCmd(), a.imageResetCmd())
	return cmd
}

func (a *App) imageListCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls", "show", "status"},
		Short:   "Show the effective image for each stack",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runImageList(cmd, all)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include supporting service images that use their defaults")
	return cmd
}

func (a *App) imageSetCmd() *cobra.Command {
	var only string
	cmd := &cobra.Command{
		Use:   "set <target> <image>",
		Short: "Pin an image in a stack's .env",
		Long: `set writes the image variable to the selected stack's .env, preserving the
rest of the file. Species targets apply to every species stack unless --only
names one. Use emobase image --all to see the supporting service targets.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			t, ok := findImageTarget(args[0])
			if !ok {
				return unknownImageTarget(args[0])
			}
			ref := strings.TrimSpace(args[1])
			if err := validateImageRef(ref); err != nil {
				return err
			}
			stacks, err := imageTargetStacks(ws, t, only)
			if err != nil {
				return err
			}
			return a.applyImageSet(cmd, t, stacks, ref)
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "limit to one stack (a species/checkout name for species images)")
	return cmd
}

func (a *App) imageResetCmd() *cobra.Command {
	var only string
	var all bool
	cmd := &cobra.Command{
		Use:   "reset <target>",
		Short: "Remove an image override and return to the Compose default",
		Long: `reset removes the image variable from the selected stack's .env, so Compose
uses the default declared in its own Compose file. Species targets apply to every
species stack unless --only names one.

With --all, reset removes every managed image override in the workspace. Each
affected .env is saved once, preserving comments and keeping one .env.bak copy.`,
		Args: cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := a.Workspace()
			if err != nil {
				return err
			}
			if all {
				if len(args) > 0 {
					return fmt.Errorf("--all and a target cannot be used together")
				}
				if strings.TrimSpace(only) != "" {
					return fmt.Errorf("--all and --only cannot be used together")
				}
				return a.applyImageResetAll(cmd, ws)
			}
			if len(args) == 0 {
				return fmt.Errorf(`choose an image to reset, or use --all

Targets: %s

Examples:
  emobase image reset genomics
  emobase image reset species-web --only Tcas
  emobase image reset --all`, imageTargetNames())
			}
			t, ok := findImageTarget(args[0])
			if !ok {
				return unknownImageTarget(args[0])
			}
			stacks, err := imageTargetStacks(ws, t, only)
			if err != nil {
				return err
			}
			return a.applyImageResetJobs(cmd, t.Name, []imageResetJob{{Target: t, Stacks: stacks}})
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "remove every managed image override")
	cmd.Flags().StringVar(&only, "only", "", "limit to one stack (a species/checkout name for species images)")
	return cmd
}

type imageRow struct {
	Target   string `json:"target"`
	Variable string `json:"variable"`
	Image    string `json:"image"`
	Source   string `json:"source"`
	Default  string `json:"default,omitempty"`
	Role     string `json:"role"`
	Checkout string `json:"checkout"`
	Dir      string `json:"dir,omitempty"`
}

type imageChange struct {
	Target   string `json:"target"`
	Checkout string `json:"checkout"`
	Variable string `json:"variable"`
	Old      string `json:"old"`
	New      string `json:"new"`
	Default  string `json:"default,omitempty"`
	Reset    bool   `json:"reset,omitempty"`
}

func (a *App) runImageList(cmd *cobra.Command, all bool) error {
	ws, err := a.Workspace()
	if err != nil {
		return err
	}
	rows := imageRows(ws, all)
	if a.JSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}
	fmt.Fprint(cmd.OutOrStdout(), renderImages(rows))
	return nil
}

func imageRows(ws *workspace.Workspace, all bool) []imageRow {
	rows := make([]imageRow, 0)
	for _, t := range managedImages {
		stacks := imageStacks(ws, t)
		if !all && t.Group != imageGroupApp {
			stacks = overrideStacks(stacks, t.Variable)
		}
		for _, st := range stacks {
			rows = append(rows, imageRowFor(t, st))
		}
	}
	return rows
}

func overrideStacks(stacks []*workspace.Stack, variable string) []*workspace.Stack {
	var overridden []*workspace.Stack
	for _, st := range stacks {
		if v, ok := st.Get(variable); ok && strings.TrimSpace(v) != "" {
			overridden = append(overridden, st)
		}
	}
	return overridden
}

// imageStacks returns the stacks a target applies to. A target defined by an
// optional Compose overlay is limited to stacks whose COMPOSE_FILE includes it.
func imageStacks(ws *workspace.Workspace, t imageTarget) []*workspace.Stack {
	var stacks []*workspace.Stack
	switch t.Role {
	case workspace.RolePortal:
		if ws.Portal != nil {
			stacks = []*workspace.Stack{ws.Portal}
		}
	case workspace.RoleSpecies:
		stacks = ws.Species
	}
	if t.ComposeFile == "" {
		return stacks
	}
	var matching []*workspace.Stack
	for _, st := range stacks {
		if stackUsesComposeFile(st, t.ComposeFile) {
			matching = append(matching, st)
		}
	}
	return matching
}

func stackUsesComposeFile(st *workspace.Stack, name string) bool {
	raw, ok := st.Get("COMPOSE_FILE")
	if !ok || strings.TrimSpace(raw) == "" {
		return false
	}
	for _, part := range strings.Split(raw, string(os.PathListSeparator)) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == name || filepath.Base(part) == name {
			return true
		}
	}
	return false
}

func imageRowFor(t imageTarget, st *workspace.Stack) imageRow {
	def := composeImageDefault(st, t)
	raw, ok := st.Get(t.Variable)
	value := strings.TrimSpace(raw)

	row := imageRow{
		Target:   t.Name,
		Variable: t.Variable,
		Default:  def,
		Role:     string(st.Role),
		Checkout: st.Name,
		Dir:      st.Dir,
	}
	switch {
	case ok && value != "":
		row.Image = value
		row.Source = imageSourceEnv
	case def != "":
		row.Image = def
		row.Source = imageSourceDefault
	default:
		row.Source = imageSourceUnset
	}
	return row
}

// composeImageDefault reads a ${VAR:-default} fallback from the Compose file
// that defines the image. Compose treats an empty ${VAR:-} value as unset, so
// the same default applies when .env defines the variable without a value.
func composeImageDefault(st *workspace.Stack, t imageTarget) string {
	var path string
	if t.ComposeFile != "" {
		path = filepath.Join(st.Dir, t.ComposeFile)
	} else {
		p, ok := workspace.ComposePath(st.Dir)
		if !ok {
			return ""
		}
		path = p
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`(?m)^\s*image:\s*"?\$\{` + regexp.QuoteMeta(t.Variable) + `:?-([^}]*)\}`)
	m := re.FindSubmatch(data)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(string(m[1]))
}

func renderImages(rows []imageRow) string {
	var b strings.Builder
	if len(rows) == 0 {
		fmt.Fprintln(&b, "No portal or species checkouts found.")
		return b.String()
	}
	fmt.Fprintln(&b, title("Container images"))
	fmt.Fprintln(&b, muted("  Effective image from each stack's .env, or the Compose default."))
	fmt.Fprintln(&b)
	var table [][]string
	for _, r := range rows {
		image := r.Image
		if image == "" {
			image = "-"
		}
		def := "-"
		if r.Source == imageSourceEnv && r.Default != "" && r.Default != r.Image {
			def = r.Default
		}
		table = append(table, []string{r.Target, r.Checkout, r.Variable, image, r.Source, def})
	}
	fmt.Fprint(&b, ui.Table([]string{"target", "checkout", "variable", "image", "source", "default"}, table))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, muted("Pin an image: emobase image set <target> <image>"))
	fmt.Fprintln(&b, muted("Include supporting service images: emobase image --all"))
	fmt.Fprintln(&b, muted("Return to the Compose default: emobase image reset <target>"))
	return b.String()
}

func imageTargetStacks(ws *workspace.Workspace, t imageTarget, only string) ([]*workspace.Stack, error) {
	candidates := imageStacks(ws, t)
	if len(candidates) == 0 {
		switch {
		case t.Role == workspace.RolePortal && ws.Portal == nil:
			return nil, fmt.Errorf("no emobase-portal-deploy checkout found")
		case t.Role == workspace.RoleSpecies && len(ws.Species) == 0:
			return nil, fmt.Errorf("no species checkouts found")
		case t.ComposeFile != "":
			return nil, fmt.Errorf("no species checkout includes %s", t.ComposeFile)
		default:
			return nil, fmt.Errorf("no %s stack found", t.Role)
		}
	}
	if strings.TrimSpace(only) == "" {
		return candidates, nil
	}

	selected, err := selectStacks(ws, only)
	if err != nil {
		return nil, err
	}
	var stacks []*workspace.Stack
	for _, st := range selected {
		if st.Role != t.Role {
			continue
		}
		if t.ComposeFile != "" && !stackUsesComposeFile(st, t.ComposeFile) {
			continue
		}
		stacks = append(stacks, st)
	}
	if len(stacks) == 0 {
		if t.ComposeFile != "" {
			return nil, fmt.Errorf("--only %q does not select a species stack using %s", only, t.ComposeFile)
		}
		role := "portal"
		if t.Role == workspace.RoleSpecies {
			role = "species"
		}
		return nil, fmt.Errorf("--only %q does not select a %s stack", only, role)
	}
	return stacks, nil
}

func validateImageRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("image reference is required")
	}
	if strings.ContainsAny(ref, " \t\r\n") {
		return fmt.Errorf("image reference %q must not contain whitespace", ref)
	}
	if strings.Contains(ref, "${") {
		return fmt.Errorf("image reference must be a literal image, not a variable")
	}
	return nil
}

func (a *App) applyImageSet(cmd *cobra.Command, t imageTarget, stacks []*workspace.Stack, ref string) error {
	var changes []imageChange
	for _, st := range stacks {
		if st.MissingEnv {
			return fmt.Errorf("%s has no .env; run emobase create or ./init.sh first", st.Dir)
		}
		old, _ := st.Get(t.Variable)
		if strings.TrimSpace(old) == ref {
			continue
		}
		if !st.Env.Set(t.Variable, ref) {
			continue
		}
		if err := st.Env.Save(); err != nil {
			return fmt.Errorf("save %s: %w", st.EnvPath, err)
		}
		changes = append(changes, imageChange{
			Target: t.Name, Checkout: st.Name, Variable: t.Variable,
			Old: old, New: ref, Default: composeImageDefault(st, t),
		})
	}
	return a.printImageChanges(cmd, t, changes)
}

type imageResetJob struct {
	Target imageTarget
	Stacks []*workspace.Stack
}

func (a *App) applyImageResetAll(cmd *cobra.Command, ws *workspace.Workspace) error {
	var jobs []imageResetJob
	for _, t := range managedImages {
		if stacks := imageStacks(ws, t); len(stacks) > 0 {
			jobs = append(jobs, imageResetJob{Target: t, Stacks: stacks})
		}
	}
	return a.applyImageResetJobs(cmd, "", jobs)
}

func (a *App) applyImageResetJobs(cmd *cobra.Command, label string, jobs []imageResetJob) error {
	var changes []imageChange
	var order []*workspace.Stack
	dirty := map[*workspace.Stack]bool{}

	for _, job := range jobs {
		for _, st := range job.Stacks {
			if st.MissingEnv {
				continue
			}
			old, ok := st.Get(job.Target.Variable)
			if !ok {
				continue
			}
			if !st.Env.Unset(job.Target.Variable) {
				continue
			}
			if !dirty[st] {
				dirty[st] = true
				order = append(order, st)
			}
			changes = append(changes, imageChange{
				Target: job.Target.Name, Checkout: st.Name, Variable: job.Target.Variable,
				Old: old, Default: composeImageDefault(st, job.Target), Reset: true,
			})
		}
	}

	for _, st := range order {
		if err := st.Env.Save(); err != nil {
			return fmt.Errorf("save %s: %w", st.EnvPath, err)
		}
	}
	return a.printImageChanges(cmd, imageTarget{Name: label}, changes)
}

func (a *App) printImageChanges(cmd *cobra.Command, t imageTarget, changes []imageChange) error {
	if a.JSON {
		if changes == nil {
			changes = []imageChange{}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(changes)
	}

	fmt.Fprint(cmd.OutOrStdout(), renderImageChanges(t, changes))
	return nil
}

func renderImageChanges(t imageTarget, changes []imageChange) string {
	var b strings.Builder
	if len(changes) == 0 {
		if t.Name == "" {
			fmt.Fprintln(&b, "No image changes.")
		} else {
			fmt.Fprintf(&b, "No image changes for %s.\n", t.Name)
		}
		return b.String()
	}
	fmt.Fprintln(&b, title("Image changes"))
	var rows [][]string
	for _, ch := range changes {
		rows = append(rows, []string{
			ch.Target, ch.Checkout, ch.Variable, imageOldDisplay(ch), imageNewDisplay(ch),
		})
	}
	fmt.Fprint(&b, ui.Table([]string{"target", "checkout", "variable", "old", "new"}, rows))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, muted("Restart the affected stacks to apply: "+imageRestartHint(changes)))
	return b.String()
}

func imageOldDisplay(ch imageChange) string {
	if strings.TrimSpace(ch.Old) != "" {
		return ch.Old
	}
	if ch.Default != "" {
		return "(default: " + ch.Default + ")"
	}
	return "(unset)"
}

func imageNewDisplay(ch imageChange) string {
	if !ch.Reset {
		return ch.New
	}
	if ch.Default != "" {
		return "(default: " + ch.Default + ")"
	}
	return "(Compose default)"
}

func imageRestartHint(changes []imageChange) string {
	portal, species := false, false
	checkouts := map[string]bool{}
	for _, ch := range changes {
		t, ok := findImageTarget(ch.Target)
		if ok && t.Role == workspace.RolePortal {
			portal = true
		} else {
			species = true
		}
		checkouts[ch.Checkout] = true
	}
	switch {
	case portal && species:
		return "emobase up"
	case portal:
		return "emobase up --only portal"
	case len(checkouts) == 1:
		return "emobase up --only " + changes[0].Checkout
	default:
		return "emobase up --only species"
	}
}
