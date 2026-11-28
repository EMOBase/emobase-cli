// Package scaffold runs the deployment repositories' own scripts and Compose
// commands. The EMOBase repositories stay independently deployable: this
// package only orchestrates them and prints every command it runs.
package scaffold

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Runner executes commands, or prints them when DryRun is set.
type Runner struct {
	DryRun bool
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Exec runs a command in dir.
func (r *Runner) Exec(dir, name string, args ...string) error {
	return r.exec(dir, nil, name, args...)
}

// exec runs a command in dir with extra environment variables.
func (r *Runner) exec(dir string, extraEnv []string, name string, args ...string) error {
	env := os.Environ()
	if len(extraEnv) > 0 {
		env = mergeEnv(env, extraEnv)
	}
	return r.run(dir, env, name, args...)
}

// run executes a command with the given environment.
func (r *Runner) run(dir string, env []string, name string, args ...string) error {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteArg(name))
	redact := false
	for _, a := range args {
		if redact {
			parts = append(parts, "********")
			redact = false
			continue
		}
		switch a {
		case "--client-secret", "--coco-password", "--keycloak-client-secret", "--password":
			redact = true
		}
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
	cmd.Stdin = r.Stdin
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", line, err)
	}
	return nil
}

// mergeEnv applies overrides to base, replacing an existing key instead of
// creating a duplicate entry.
func mergeEnv(base, overrides []string) []string {
	out := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		skip := false
		for _, override := range overrides {
			if okey, _, _ := strings.Cut(override, "="); okey == key {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, entry)
		}
	}
	return append(out, overrides...)
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

// Compose runs docker compose in dir. Variables defined in dir/.env are
// removed from the inherited environment first: Compose gives an exported
// shell variable precedence over .env, which would initialize MySQL with a
// different root password than the deploy stacks read from the file.
func (r *Runner) Compose(dir string, args ...string) error {
	env := withoutDotEnv(os.Environ(), filepath.Join(dir, ".env"))
	return r.run(dir, env, "docker", append([]string{"compose"}, args...)...)
}

// withoutDotEnv drops entries whose key is defined in the dotenv file.
func withoutDotEnv(base []string, path string) []string {
	keys := dotEnvKeys(path)
	if len(keys) == 0 {
		return base
	}
	out := make([]string, 0, len(base))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if keys[key] {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// dotEnvKeys returns the variable names defined in a dotenv file.
func dotEnvKeys(path string) map[string]bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	keys := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if key = strings.TrimSpace(key); key != "" {
			keys[key] = true
		}
	}
	return keys
}

// Init runs ./init.sh with the given flags. EMOBASE_CLI=1 tells the deploy
// script that a person is running it through the CLI, so it can offer
// CLI-specific next steps without assuming the CLI when run standalone.
func (r *Runner) Init(dir string, args ...string) error {
	return r.exec(dir, []string{"EMOBASE_CLI=1"}, "./init.sh", args...)
}

// Up starts a stack and waits for health checks. --remove-orphans clears
// containers left behind by services that a stack no longer defines, such as
// the per-species Elasticsearch that moved to emobase-infra.
func (r *Runner) Up(dir string) error {
	return r.Compose(dir, "up", "-d", "--wait", "--remove-orphans")
}

// UpService starts one Compose service and waits for it to become healthy.
func (r *Runner) UpService(dir, service string) error {
	return r.Compose(dir, "up", "-d", "--wait", service)
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

// SetupJBrowse2 runs the portal's one-shot JBrowse2 initialiser. -T disables
// Compose's pseudo-TTY allocation: the CLI captures output through pipes, so
// Docker rejects a TTY when stdin is not a terminal.
func (r *Runner) SetupJBrowse2(dir string) error {
	return r.Compose(dir, "--profile", "setup", "run", "-T", "--rm", "setup-jbrowse2-web")
}

// Migrate runs the portal's database and Elasticsearch migrations.
func (r *Runner) Migrate(dir string) error {
	if err := r.Compose(dir, "--profile", "update", "run", "-T", "--rm", "migrate-genomics-db"); err != nil {
		return err
	}
	return r.Compose(dir, "--profile", "update", "run", "-T", "--rm", "migrate-genomics-es")
}

// SetupDirectus runs the species one-shot Directus initialiser.
func (r *Runner) SetupDirectus(dir string) error {
	return r.Compose(dir, "run", "-T", "--rm", "setup-directus")
}

// SmokeTest runs the portal smoke test.
func (r *Runner) SmokeTest(dir string) error {
	return r.Script(dir, "./scripts/smoke-test.sh")
}

// CreateDatabase calls emobase-infra's database helper.
func (r *Runner) CreateDatabase(infraDir, db string) error {
	return r.Script(infraDir, "./scripts/create-database.sh", db)
}

// DropDatabase calls emobase-infra's database helper. The CLI only calls this
// after its own --purge --yes confirmation, so the script prompt is skipped.
func (r *Runner) DropDatabase(infraDir, db string) error {
	return r.Script(infraDir, "./scripts/drop-database.sh", "--yes", db)
}

// DeleteRealm removes a dedicated realm after prune confirmation.
func (r *Runner) DeleteRealm(infraDir, realm string) error {
	return r.Script(infraDir, "./scripts/delete-realm.sh", "--yes", realm)
}

// DeleteESIndices removes every index under one species prefix from the shared
// Elasticsearch. The helper only matches <prefix>-*, so the caller must pass an
// owned prefix.
func (r *Runner) DeleteESIndices(infraDir, prefix string) error {
	return r.Script(infraDir, "./scripts/delete-es-indices.sh", "--yes", prefix)
}

// EnsureRealm creates or updates a realm and adds an origin. It keeps an
// existing seed-user password.
func (r *Runner) EnsureRealm(infraDir, realm, origin, clientID, clientSecret, cocoUser, cocoPassword string) error {
	return r.Script(infraDir, "./scripts/create-realm.sh",
		"--realm", realm,
		"--origin", origin,
		"--client-id", clientID,
		"--client-secret", clientSecret,
		"--coco-username", cocoUser,
		"--coco-password", cocoPassword,
		"--keep-password",
		"--quiet",
	)
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
