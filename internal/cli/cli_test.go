package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EMOBase/emobase-cli/internal/workspace"
)

const (
	testInfraCompose   = "services:\n  keycloak:\n    image: kc\n  mysql:\n    image: mysql\n"
	testPortalCompose  = "services:\n  genomicsservice:\n    image: g\n"
	testSpeciesCompose = "services:\n  directus:\n    image: d\n"
)

func mkCLIStack(t *testing.T, root, name, compose, env string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	if env != "" {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func matrixFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mkCLIStack(t, root, "emobase-infra", testInfraCompose, strings.Join([]string{
		"KEYCLOAK_REALM=ibb",
		"KEYCLOAK_CLIENT_ID=ibb-client",
		"KEYCLOAK_CLIENT_SECRET=super-secret",
		"MYSQL_ROOT_PASSWORD=root-secret",
		"KEYCLOAK_PUBLIC_URL=http://localhost:8081/keycloak",
		"KEYCLOAK_INTERNAL_URL=http://keycloak:8080/keycloak",
		"EMOBASE_INFRA_NETWORK=emobase-infra",
		"",
	}, "\n"))
	mkCLIStack(t, root, "emobase-portal-deploy", testPortalCompose, strings.Join([]string{
		"PUBLIC_SERVER=http://localhost:9080",
		"EMOBASE_GENOMICS_MYSQL__DATABASE=emobase_genomics",
		"KEYCLOAK_REALM=ibb",
		"KEYCLOAK_CLIENT_ID=ibb-client",
		"KEYCLOAK_CLIENT_SECRET=super-secret",
		"MYSQL_ROOT_PASSWORD=root-secret",
		"KEYCLOAK_PUBLIC_URL=http://localhost:8081/keycloak",
		"KEYCLOAK_INTERNAL_URL=http://keycloak:8080/keycloak",
		"EMOBASE_INFRA_NETWORK=emobase-infra",
		"",
	}, "\n"))
	mkCLIStack(t, root, "emobase-deploy-tcas", testSpeciesCompose, strings.Join([]string{
		"MAIN_SPECIES=Tcas",
		"PUBLIC_SERVER=http://localhost:9090",
		"KEYCLOAK_REALM=ibb_tcas",
		"KEYCLOAK_CLIENT_ID=ibb-client",
		"KEYCLOAK_CLIENT_SECRET=dedicated-secret",
		"MYSQL_ROOT_PASSWORD=root-secret",
		"KEYCLOAK_PUBLIC_URL=http://localhost:8081/keycloak",
		"KEYCLOAK_INTERNAL_URL=http://keycloak:8080/keycloak",
		"EMOBASE_INFRA_NETWORK=emobase-infra",
		"",
	}, "\n"))
	return root
}

func TestMatrixRowsMaskSecrets(t *testing.T) {
	root := matrixFixture(t)
	ws, err := workspace.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	headers, rows := matrixRows(ws)
	if len(headers) != 5 {
		t.Fatalf("headers = %v", headers)
	}
	if headers[3] != "portal" || headers[4] != "species:emobase-deploy-tcas" {
		t.Fatalf("headers = %v", headers)
	}
	var secretRow []string
	for _, row := range rows {
		if row[0] == "MYSQL_ROOT_PASSWORD" {
			secretRow = row
		}
	}
	if secretRow == nil {
		t.Fatal("missing MYSQL_ROOT_PASSWORD row")
	}
	for _, cell := range secretRow[2:] {
		if strings.Contains(cell, "root-secret") {
			t.Fatalf("secret leaked: %v", secretRow)
		}
	}
}

func TestWriteMatrixJSON(t *testing.T) {
	root := matrixFixture(t)
	ws, err := workspace.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	headers, rows := matrixRows(ws)
	var buf bytes.Buffer
	if err := writeMatrixJSON(&buf, ws, headers, rows); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Workspace string     `json:"workspace"`
		Headers   []string   `json:"headers"`
		Rows      [][]string `json:"rows"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Workspace != root || len(got.Headers) != len(headers) || len(got.Rows) != len(rows) {
		t.Fatalf("unexpected JSON: %+v", got)
	}
}

func TestDoctorJSONReportsErrors(t *testing.T) {
	root := t.TempDir()
	mkCLIStack(t, root, "emobase-infra", testInfraCompose, "KEYCLOAK_REALM=ibb\n")

	cmd := New()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"--workspace", root, "--json", "doctor"})
	err := cmd.Execute()
	if err != ErrFindings {
		t.Fatalf("err = %v, stderr = %s", err, errb.String())
	}
	var rep struct {
		Errors   int `json:"errors"`
		Findings []struct {
			Check    string `json:"check"`
			Severity string `json:"severity"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if rep.Errors == 0 || len(rep.Findings) == 0 {
		t.Fatalf("report = %+v", rep)
	}
}

func TestSpeciesListJSON(t *testing.T) {
	root := matrixFixture(t)
	cmd := New()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"--workspace", root, "--json", "species", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("err = %v, stderr = %s", err, errb.String())
	}
	var rows []struct {
		Species string `json:"species"`
		URL     string `json:"url"`
	}
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if len(rows) != 1 || rows[0].Species != "Tcas" || rows[0].URL != "http://localhost:9090" {
		t.Fatalf("rows = %+v", rows)
	}
}
