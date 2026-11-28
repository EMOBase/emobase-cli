package cli

import "testing"

func TestSpeciesOrigin(t *testing.T) {
	cases := []struct {
		portal, slug, want string
	}{
		{"https://emobase.uni-goettingen.de", "tcas", "https://tcas.emobase.uni-goettingen.de"},
		{"https://portal.example.org/", "ptep", "https://ptep.portal.example.org"},
		{"http://localhost:9080", "tcas", "http://tcas.localhost:9080"},
	}
	for _, tc := range cases {
		if got := speciesOrigin(tc.portal, tc.slug); got != tc.want {
			t.Errorf("speciesOrigin(%q, %q) = %q, want %q", tc.portal, tc.slug, got, tc.want)
		}
	}
	if got := speciesOrigin("not-a-url", "tcas"); got != "" {
		t.Fatalf("invalid portal URL = %q", got)
	}
}

func TestValidKeycloakName(t *testing.T) {
	for _, ok := range []string{"emobase-tcas", "a.b_c-d", "A1"} {
		if !validKeycloakName(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "with space", "with/slash", "with:colon"} {
		if validKeycloakName(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestRandomToken(t *testing.T) {
	a, err := randomToken(24)
	if err != nil {
		t.Fatal(err)
	}
	b, err := randomToken(24)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 24 || len(b) != 24 {
		t.Fatalf("lengths = %d, %d", len(a), len(b))
	}
	if a == b {
		t.Fatal("two tokens are equal")
	}
}
