// Package scaffold runs the deployment repositories' own scripts and Compose
// commands. The EMOBase repositories stay independently deployable: this
// package only orchestrates them and prints every command it runs.
package scaffold

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Runner executes commands, or prints them when DryRun is set.
type Runner struct {
	DryRun bool
	Stdout io.Writer
	Stderr io.Writer
}

// Exec runs a command in dir.
func (r *Runner) Exec(dir, name string, args ...string) error {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteArg(name))
	for _, a := range args {
		parts = append(parts, quoteArg(a))
	}
	line := fmt.Sprintf("cd %s && %s", quoteArg(dir), strings.Join(parts, " "))
	if r.DryRun {
		fmt.Fprintln(r.Stdout, "+ "+line)
		return nil
	}
	fmt.Fprintln(r.Stderr, "+ "+line)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", line, err)
	}
	return nil
}

// quoteArg renders an argument for the printed command line. It never affects
// what is executed.
func quoteArg(s string) string {
	if s == "" {
		return "''"
	}
	if strings.ContainsAny(s, " \t\n'\"*$&|;<>()[]{}?!#~`\\") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}

// Script runs an executable relative to dir (for example ./init.sh).
func (r *Runner) Script(dir, script string, args ...string) error {
	return r.Exec(dir, script, args...)
}

// Compose runs docker compose in dir.
func (r *Runner) Compose(dir string, args ...string) error {
	return r.Exec(dir, "docker", append([]string{"compose"}, args...)...)
}

// Init runs ./init.sh with the given flags.
func (r *Runner) Init(dir string, args ...string) error {
	return r.Script(dir, "./init.sh", args...)
}

// Up starts a stack and waits for health checks.
func (r *Runner) Up(dir string) error {
	return r.Compose(dir, "up", "-d", "--wait")
}

// Pull pulls every Compose profile in dir.
func (r *Runner) Pull(dir string) error {
	return r.Compose(dir, "--profile", "*", "pull")
}

// Down stops a stack without deleting volumes.
func (r *Runner) Down(dir string) error {
	return r.Compose(dir, "--profile", "*", "down")
}

// Destroy stops a stack and deletes its volumes.
func (r *Runner) Destroy(dir string) error {
	return r.Compose(dir, "--profile", "*", "down", "-v")
}

// SetupJBrowse2 runs the portal's one-shot JBrowse2 initialiser.
func (r *Runner) SetupJBrowse2(dir string) error {
	return r.Compose(dir, "--profile", "setup", "run", "--rm", "setup-jbrowse2-web")
}

// Migrate runs the portal's database and Elasticsearch migrations.
func (r *Runner) Migrate(dir string) error {
	if err := r.Compose(dir, "--profile", "update", "run", "--rm", "migrate-genomics-db"); err != nil {
		return err
	}
	return r.Compose(dir, "--profile", "update", "run", "--rm", "migrate-genomics-es")
}

// SetupDirectus runs the species one-shot Directus initialiser.
func (r *Runner) SetupDirectus(dir string) error {
	return r.Compose(dir, "run", "--rm", "setup-directus")
}

// SmokeTest runs the portal smoke test.
func (r *Runner) SmokeTest(dir string) error {
	return r.Script(dir, "./scripts/smoke-test.sh")
}

// CreateDatabase calls emobase-infra's database helper.
func (r *Runner) CreateDatabase(infraDir, db string) error {
	return r.Script(infraDir, "./scripts/create-database.sh", db)
}

// DropDatabase calls emobase-infra's database helper.
func (r *Runner) DropDatabase(infraDir, db string) error {
	return r.Script(infraDir, "./scripts/drop-database.sh", db)
}

// AllowOrigin registers a shared-realm origin with Keycloak.
func (r *Runner) AllowOrigin(infraDir, origin string) error {
	return r.Script(infraDir, "./scripts/allow-origin.sh", origin)
}

// Clone clones a repository into dir when it is missing.
func (r *Runner) Clone(dir, url, branch string) error {
	args := []string{"clone"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	args = append(args, url, dir)
	// Clone runs from dir's parent so the final argument is the new directory.
	return r.Exec(filepath.Dir(dir), "git", args...)
}

// GitPull updates a checkout.
func (r *Runner) GitPull(dir string) error {
	return r.Exec(dir, "git", "pull")
}
