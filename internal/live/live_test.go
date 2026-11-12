package live

import "testing"

func TestParsePSArray(t *testing.T) {
	out := `[{"Service":"mysql","Name":"infra-mysql-1","State":"running","Health":"healthy"},{"Service":"keycloak","State":"exited"}]`
	cs, err := parsePS(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || cs[0].Service != "mysql" || cs[0].Health != "healthy" || cs[1].State != "exited" {
		t.Fatalf("containers = %+v", cs)
	}
}

func TestParsePSLines(t *testing.T) {
	out := "{\"Service\":\"mysql\",\"State\":\"running\"}\n{\"Service\":\"keycloak\",\"State\":\"running\"}\n"
	cs, err := parsePS(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("containers = %+v", cs)
	}
}

func TestParsePSEmpty(t *testing.T) {
	for _, out := range []string{"", "  \n"} {
		cs, err := parsePS(out)
		if err != nil || cs != nil {
			t.Fatalf("parsePS(%q) = %+v, %v", out, cs, err)
		}
	}
}
