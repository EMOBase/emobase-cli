package cli

import (
	"strings"
	"testing"

	"github.com/EMOBase/emobase-cli/internal/checks"
)

func TestKeycloakProbeHint(t *testing.T) {
	cases := []struct {
		name  string
		out   string
		cause string
		hint  string
	}{
		{
			name:  "tls",
			out:   "UNABLE_TO_VERIFY_LEAF_SIGNATURE",
			cause: "does not trust the TLS certificate",
			hint:  "KEYCLOAK_INTERNAL_URL=http://host.docker.internal:8181/keycloak",
		},
		{
			name:  "dns",
			out:   "ENOTFOUND",
			cause: "cannot resolve",
			hint:  "host.docker.internal",
		},
		{
			name:  "refused",
			out:   "ECONNREFUSED",
			cause: "cannot connect",
			hint:  "emobase-infra is running",
		},
		{
			name:  "status",
			out:   "status 404",
			cause: "answered status 404",
			hint:  "/realms/ibb",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cause, hint := keycloakProbeHint(tc.out, "ibeetlebase.emobase.test", "ibb")
			if !strings.Contains(cause, tc.cause) {
				t.Fatalf("cause = %q, want %q", cause, tc.cause)
			}
			if !strings.Contains(hint, tc.hint) {
				t.Fatalf("hint = %q, want %q", hint, tc.hint)
			}
		})
	}
}

func TestDoctorSummaryMentionsLiveChecks(t *testing.T) {
	static := renderDoctor(&checks.Report{}, nil, false, false, false)
	if !strings.Contains(static, "configuration is consistent") {
		t.Fatalf("static summary = %q", static)
	}
	live := renderDoctor(&checks.Report{}, nil, false, false, true)
	if !strings.Contains(live, "configuration and live checks passed") {
		t.Fatalf("live summary = %q", live)
	}
}
