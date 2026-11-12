// Package workspace discovers the EMOBase deployment checkouts that live next
// to each other: emobase-infra, emobase-portal-deploy and one emobase-deploy
// clone per species. Discovery is signature-based, so directories can be named
// freely and no central registry file is needed.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/EMOBase/emobase-cli/internal/envfile"
)

// Role is the kind of stack a checkout runs.
type Role string

const (
	RoleInfra   Role = "infra"
	RolePortal  Role = "portal"
	RoleSpecies Role = "species"
)

// Stack is one deployment checkout.
type Stack struct {
	Role       Role
	Dir        string
	Name       string
	EnvPath    string
	Env        *envfile.File
	MissingEnv bool
	// ProjectName is the Compose project name, read from `name:` or from the
	// COMPOSE_PROJECT_NAME variable. Used for collision checks.
	ProjectName string
}

func (s *Stack) String() string {
	if s.Role == RoleSpecies {
		if sp, ok := s.Env.Get("MAIN_SPECIES"); ok && sp != "" {
			return fmt.Sprintf("%s (%s)", s.Name, sp)
		}
	}
	return s.Name
}

// Get reads a value from the stack's .env.
func (s *Stack) Get(key string) (string, bool) {
	if s.Env == nil {
		return "", false
	}
	return s.Env.Get(key)
}

// Has reports whether key is present with a non-empty value.
func (s *Stack) Has(key string) bool {
	if v, ok := s.Get(key); ok {
		return strings.TrimSpace(v) != ""
	}
	return false
}

// Workspace is a discovered set of stacks.
type Workspace struct {
	Root    string
	Infra   *Stack
	Portal  *Stack
	Species []*Stack
}

// All returns every stack in dependency order.
func (w *Workspace) All() []*Stack {
	var out []*Stack
	if w.Infra != nil {
		out = append(out, w.Infra)
	}
	if w.Portal != nil {
		out = append(out, w.Portal)
	}
	out = append(out, w.Species...)
	return out
}

// Deploy is All minus infra; the stacks that join the shared network.
func (w *Workspace) Deploy() []*Stack {
	var out []*Stack
	if w.Portal != nil {
		out = append(out, w.Portal)
	}
	return append(out, w.Species...)
}

var (
	reKeycloak = regexp.MustCompile(`(?m)^\s{2}keycloak:\s*$`)
	reMySQL    = regexp.MustCompile(`(?m)^\s{2}mysql:\s*$`)
	rePortal   = regexp.MustCompile(`(?m)^\s{2}genomicsservice:\s*$`)
	reDirectus = regexp.MustCompile(`(?m)^\s{2}directus:\s*$`)
	reName     = regexp.MustCompile(`(?m)^name:\s*(\S+)\s*$`)
)

// ResolveRoot returns the workspace root. When flag is empty the current
// directory is used, or its parent when the current directory is itself a
// stack checkout.
func ResolveRoot(flag string) (string, error) {
	if flag != "" {
		abs, err := filepath.Abs(flag)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if _, ok := composeFile(cwd); ok {
		return filepath.Dir(cwd), nil
	}
	return cwd, nil
}

func composeFile(dir string) (string, bool) {
	for _, name := range []string{"compose.yml", "compose.yaml", "docker-compose.yml", "docker-compose.yaml"} {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, true
		}
	}
	return "", false
}

func detectRole(dir string) (Role, string, bool, error) {
	compose, ok := composeFile(dir)
	if !ok {
		return "", "", false, nil
	}
	b, err := os.ReadFile(compose)
	if err != nil {
		return "", "", false, err
	}
	s := string(b)
	project := ""
	if m := reName.FindStringSubmatch(s); m != nil {
		project = m[1]
	}
	switch {
	case reKeycloak.MatchString(s) && reMySQL.MatchString(s):
		return RoleInfra, project, true, nil
	case rePortal.MatchString(s):
		return RolePortal, project, true, nil
	case reDirectus.MatchString(s):
		return RoleSpecies, project, true, nil
	default:
		return "", "", false, nil
	}
}

// Discover scans root's immediate children. If root is itself a stack it is
// used as the single stack.
func Discover(root string) (*Workspace, error) {
	w := &Workspace{Root: root}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, ent := range entries {
		if !ent.IsDir() || strings.HasPrefix(ent.Name(), ".") || ent.Name() == "node_modules" {
			continue
		}
		dir := filepath.Join(root, ent.Name())
		role, project, ok, err := detectRole(dir)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		st := &Stack{Role: role, Dir: dir, Name: ent.Name(), EnvPath: filepath.Join(dir, ".env"), ProjectName: project}
		if err := st.load(); err != nil {
			return nil, err
		}
		if err := w.add(st); err != nil {
			return nil, err
		}
	}
	if len(w.All()) == 0 {
		if role, project, ok, err := detectRole(root); err != nil {
			return nil, err
		} else if ok {
			st := &Stack{Role: role, Dir: root, Name: filepath.Base(root), EnvPath: filepath.Join(root, ".env"), ProjectName: project}
			if err := st.load(); err != nil {
				return nil, err
			}
			if err := w.add(st); err != nil {
				return nil, err
			}
		}
	}
	if len(w.All()) == 0 {
		return nil, fmt.Errorf("no EMOBase checkouts found in %s\nClone emobase-infra, emobase-portal-deploy and emobase-deploy side by side, or pass --workspace", root)
	}
	sort.Slice(w.Species, func(i, j int) bool { return w.Species[i].Name < w.Species[j].Name })
	for _, st := range w.Species {
		if st.ProjectName == "" {
			if v, ok := st.Get("COMPOSE_PROJECT_NAME"); ok {
				st.ProjectName = v
			}
		}
		// Compose falls back to the checkout directory name, which is what the
		// uniqueness check must compare against.
		if st.ProjectName == "" {
			st.ProjectName = filepath.Base(st.Dir)
		}
	}
	return w, nil
}

func (s *Stack) load() error {
	f, err := envfile.LoadOrEmpty(s.EnvPath)
	if err != nil {
		return err
	}
	s.Env = f
	_, statErr := os.Stat(s.EnvPath)
	s.MissingEnv = os.IsNotExist(statErr)
	return nil
}

func (w *Workspace) add(st *Stack) error {
	switch st.Role {
	case RoleInfra:
		if w.Infra != nil {
			return fmt.Errorf("multiple infra checkouts: %s and %s", w.Infra.Dir, st.Dir)
		}
		w.Infra = st
	case RolePortal:
		if w.Portal != nil {
			return fmt.Errorf("multiple portal checkouts: %s and %s", w.Portal.Dir, st.Dir)
		}
		w.Portal = st
	case RoleSpecies:
		w.Species = append(w.Species, st)
	}
	return nil
}

// FindSpecies returns the species stack whose directory, species code or
// project name matches ref.
func (w *Workspace) FindSpecies(ref string) *Stack {
	ref = strings.ToLower(ref)
	for _, s := range w.Species {
		if strings.ToLower(s.Name) == ref || strings.EqualFold(s.Dir, ref) || strings.EqualFold(s.ProjectName, ref) {
			return s
		}
		if v, ok := s.Get("MAIN_SPECIES"); ok && strings.ToLower(v) == ref {
			return s
		}
	}
	return nil
}
