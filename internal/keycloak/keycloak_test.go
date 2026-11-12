package keycloak

import "testing"

func TestMatchesOrigin(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		origin   string
		want     bool
	}{
		{"exact", []string{"http://localhost:9090"}, "http://localhost:9090", true},
		{"exact with slash", []string{"http://localhost:9090/"}, "http://localhost:9090", true},
		{"trailing slash on origin", []string{"http://localhost:9090"}, "http://localhost:9090/", true},
		{"wildcard covers subpath", []string{"http://localhost:9090/*"}, "http://localhost:9090", true},
		{"wildcard covers child", []string{"http://localhost:9090/*"}, "http://localhost:9090/app", true},
		{"wildcard does not cover lookalike", []string{"http://localhost:9090/*"}, "http://localhost:9090-other", false},
		{"global wildcard", []string{"*"}, "https://anything.example.org", true},
		{"different port", []string{"http://localhost:9091"}, "http://localhost:9090", false},
		{"different scheme", []string{"https://portal.example.org"}, "http://portal.example.org", false},
		{"empty patterns", nil, "http://localhost:9090", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesOrigin(tt.patterns, tt.origin); got != tt.want {
				t.Fatalf("MatchesOrigin(%v, %q) = %v, want %v", tt.patterns, tt.origin, got, tt.want)
			}
		})
	}
}
