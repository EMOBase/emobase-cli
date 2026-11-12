package cli

import (
	"bytes"
	"strings"
	"testing"
)

func runPreview(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := RunPreview(args, &out, &errb)
	return out.String(), errb.String(), code
}

func TestPreviewListShowsEveryCase(t *testing.T) {
	out, errb, code := runPreview(t, "list")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, errb)
	}
	for _, c := range previewCases() {
		if !strings.Contains(out, c.Name) {
			t.Errorf("list is missing %q", c.Name)
		}
	}
}

func TestPreviewRendersEveryCase(t *testing.T) {
	for _, c := range previewCases() {
		t.Run(c.Name, func(t *testing.T) {
			out, errb, code := runPreview(t, c.Name, "--no-color")
			if code != 0 {
				t.Fatalf("code = %d, stderr = %s", code, errb)
			}
			if strings.TrimSpace(out) == "" {
				t.Fatal("empty output")
			}
		})
	}
}

func TestPreviewMasksSecrets(t *testing.T) {
	secrets := []string{"root-secret", "wrong-secret", "new-secret", "infra-secret", "tcas-secret", "dmel-secret"}
	for _, name := range []string{"doctor-fix-applied", "matrix-healthy", "matrix-conflicts"} {
		out, _, code := runPreview(t, name, "--no-color")
		if code != 0 {
			t.Fatalf("%s: code = %d", name, code)
		}
		for _, secret := range secrets {
			if strings.Contains(out, secret) {
				t.Errorf("%s leaks %q", name, secret)
			}
		}
	}
}

func TestPreviewUnknownCase(t *testing.T) {
	_, errb, code := runPreview(t, "no-such-case")
	if code != 2 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(errb, "unknown case") {
		t.Fatalf("stderr = %s", errb)
	}
}
