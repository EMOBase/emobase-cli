package scaffold

import (
	"bytes"
	"strings"
	"testing"
)

func TestQuoteArg(t *testing.T) {
	tests := map[string]string{
		"plain":     "plain",
		"two words": "'two words'",
		"*":         "'*'",
		"":          "''",
		"a'b":       `'a'\''b'`,
		"/tmp/a b":  "'/tmp/a b'",
	}
	for in, want := range tests {
		if got := quoteArg(in); got != want {
			t.Errorf("quoteArg(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDryRunPrintsWithoutExecuting(t *testing.T) {
	var out, errb bytes.Buffer
	r := &Runner{DryRun: true, Stdout: &out, Stderr: &errb}
	if err := r.Compose("/tmp/does-not-exist", "--profile", "*", "pull"); err != nil {
		t.Fatal(err)
	}
	want := "+ cd /tmp/does-not-exist && docker compose --profile '*' pull\n"
	if out.String() != want {
		t.Fatalf("out = %q", out.String())
	}
	if errb.Len() != 0 {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestOneShotComposeRunsDisableTTY(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Runner) error
		want string
	}{
		{"jbrowse2", func(r *Runner) error { return r.SetupJBrowse2("/tmp/portal") },
			"docker compose --profile setup run -T --rm setup-jbrowse2-web"},
		{"migrate", func(r *Runner) error { return r.Migrate("/tmp/portal") },
			"docker compose --profile update run -T --rm migrate-genomics-db"},
		{"directus", func(r *Runner) error { return r.SetupDirectus("/tmp/species") },
			"docker compose run -T --rm setup-directus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			r := &Runner{DryRun: true, Stdout: &out, Stderr: &out}
			if err := tt.run(r); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tt.want) {
				t.Fatalf("output %q does not contain %q", out.String(), tt.want)
			}
		})
	}
}

func TestCloneRunsFromParent(t *testing.T) {
	var out bytes.Buffer
	r := &Runner{DryRun: true, Stdout: &out}
	if err := r.Clone("/tmp/work/emobase-deploy-tcas", "https://example.org/repo.git", "dev"); err != nil {
		t.Fatal(err)
	}
	line := out.String()
	for _, want := range []string{"cd /tmp/work", "git clone -b dev https://example.org/repo.git /tmp/work/emobase-deploy-tcas"} {
		if !strings.Contains(line, want) {
			t.Fatalf("%q does not contain %q", line, want)
		}
	}
}

func TestDropDatabaseSkipsPrompt(t *testing.T) {
	var out bytes.Buffer
	r := &Runner{DryRun: true, Stdout: &out, Stderr: &out}
	if err := r.DropDatabase("/tmp/infra", "directus_tcas"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "drop-database.sh --yes directus_tcas") {
		t.Fatalf("out = %q", out.String())
	}
}

func TestDryRunRedactsSecrets(t *testing.T) {
	var out bytes.Buffer
	r := &Runner{DryRun: true, Stdout: &out, Stderr: &out}
	if err := r.Exec("/tmp", "cmd",
		"--client-secret", "client-secret-value",
		"--coco-password", "coco-password-value",
		"--origin", "https://example.org"); err != nil {
		t.Fatal(err)
	}
	line := out.String()
	for _, secret := range []string{"client-secret-value", "coco-password-value"} {
		if strings.Contains(line, secret) {
			t.Fatalf("secret %q leaked in %q", secret, line)
		}
	}
	if !strings.Contains(line, "********") {
		t.Fatalf("redaction marker missing: %q", line)
	}
}
