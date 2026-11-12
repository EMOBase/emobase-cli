package spec

import "testing"

func TestIsSecret(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"MYSQL_ROOT_PASSWORD", true},
		{"KEYCLOAK_ADMIN_PASSWORD", true},
		{"AUTH_SECRET", true},
		{"KEYCLOAK_CLIENT_SECRET", true},
		{"SSR_OIDC_SESSION_SECRET", true},
		{"NUXT_OIDC_TOKEN_KEY", true},
		{"DIRECTUS_KEY", true},
		{"COCO_PASSWORD", true},
		{"PUBLIC_SERVER", false},
		{"DIRECTUS_KEYCLOAK_ISSUER_URL", false},
		{"INTERNAL_KEYCLOAK_ISSUER", false},
		{"KEYCLOAK_PUBLIC_URL", false},
		{"EMOBASE_INFRA_NETWORK", false},
	}
	for _, tt := range tests {
		if got := IsSecret(tt.key); got != tt.want {
			t.Errorf("IsSecret(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestIsPlaceholder(t *testing.T) {
	for _, v := range []string{"", "  ", "change-me", "CHANGEME"} {
		if !IsPlaceholder(v) {
			t.Errorf("IsPlaceholder(%q) = false", v)
		}
	}
	for _, v := range []string{"secret", "http://localhost"} {
		if IsPlaceholder(v) {
			t.Errorf("IsPlaceholder(%q) = true", v)
		}
	}
}

func TestNormalizeURL(t *testing.T) {
	if got := NormalizeURL(" https://example.org/ "); got != "https://example.org" {
		t.Fatalf("NormalizeURL = %q", got)
	}
}
