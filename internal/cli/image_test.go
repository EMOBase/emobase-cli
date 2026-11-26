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
	imagePortalCompose = `services:
  genomicsservice:
    image: ${EMOBASE_GENOMICS_IMAGE:-ghcr.io/emobase/emobase-genomics:main}
  fe:
    image: ${EMOBASE_PORTAL_WEB_IMAGE:-ghcr.io/emobase/emobase-portal-web:main}
`
	imageSpeciesCompose = `services:
  directus:
    image: directus/directus:11
  fe:
    image: ${EMOBASE_WEB_IMAGE:-ghcr.io/emobase/emobase-web:dev}
  ontologyservice:
    image: ${ONTOLOGYSERVICE_IMAGE:-ghcr.io/emobase/ibb-ontologyservice:v1.6.2}
  imageservice:
    image: ${IMAGESERVICE_IMAGE:-ghcr.io/emobase/ibb-imageservice:v1.6.2}
  phenotypeservice:
    image: ${PHENOTYPESERVICE_IMAGE:-ghcr.io/emobase/ibb-phenotypeservice:v1.6.1}
  goannotationservice:
    image: ${GOANNOTATIONSERVICE_IMAGE:-ghcr.io/emobase/ibb-goannotationservice:v1.6.1}
  publicationservice:
    image: ${PUBLICATIONSERVICE_IMAGE:-ghcr.io/emobase/ibb-publicationservice:v1.6.1}
`
	imageTcasOverlay = `services:
  oba:
    image: ${OBA_IMAGE:-openjdk:7}
`
)

func imageFixture(t *testing.T) (string, *workspace.Workspace) {
	t.Helper()
	root := t.TempDir()
	mkCLIStack(t, root, "emobase-portal-deploy", imagePortalCompose,
		"# portal\nEMOBASE_GENOMICS_IMAGE=emobase-genomics:local\nEMOBASE_PORTAL_WEB_IMAGE=\n")
	mkCLIStack(t, root, "emobase-deploy-tcas", imageSpeciesCompose,
		"MAIN_SPECIES=Tcas\nCOMPOSE_FILE=compose.yml:compose.tcas.yml\n# tcas\nEMOBASE_WEB_IMAGE=ghcr.io/emobase/emobase-web:test\n")
	if err := os.WriteFile(filepath.Join(root, "emobase-deploy-tcas", "compose.tcas.yml"), []byte(imageTcasOverlay), 0o600); err != nil {
		t.Fatal(err)
	}
	mkCLIStack(t, root, "emobase-deploy-dmel", imageSpeciesCompose, "MAIN_SPECIES=Dmel\n")
	ws, err := workspace.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, ws
}

func findImageRow(rows []imageRow, checkout, variable string) *imageRow {
	for i := range rows {
		if rows[i].Checkout == checkout && rows[i].Variable == variable {
			return &rows[i]
		}
	}
	return nil
}

func runImageCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := New()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

func TestImageRowsUseEnvOrDefault(t *testing.T) {
	_, ws := imageFixture(t)
	rows := imageRows(ws, false)
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4: %#v", len(rows), rows)
	}

	genomics := findImageRow(rows, "emobase-portal-deploy", "EMOBASE_GENOMICS_IMAGE")
	if genomics == nil || genomics.Image != "emobase-genomics:local" || genomics.Source != imageSourceEnv {
		t.Fatalf("genomics row = %#v", genomics)
	}
	if genomics.Default != "ghcr.io/emobase/emobase-genomics:main" {
		t.Fatalf("genomics default = %q", genomics.Default)
	}

	portalWeb := findImageRow(rows, "emobase-portal-deploy", "EMOBASE_PORTAL_WEB_IMAGE")
	if portalWeb == nil || portalWeb.Image != "ghcr.io/emobase/emobase-portal-web:main" || portalWeb.Source != imageSourceDefault {
		t.Fatalf("portal-web row = %#v", portalWeb)
	}

	dmel := findImageRow(rows, "emobase-deploy-dmel", "EMOBASE_WEB_IMAGE")
	if dmel == nil || dmel.Image != "ghcr.io/emobase/emobase-web:dev" || dmel.Source != imageSourceDefault {
		t.Fatalf("dmel row = %#v", dmel)
	}
	tcas := findImageRow(rows, "emobase-deploy-tcas", "EMOBASE_WEB_IMAGE")
	if tcas == nil || tcas.Image != "ghcr.io/emobase/emobase-web:test" || tcas.Source != imageSourceEnv {
		t.Fatalf("tcas row = %#v", tcas)
	}
}

func TestImageListJSON(t *testing.T) {
	root, _ := imageFixture(t)
	out, errb, err := runImageCLI(t, "--workspace", root, "--json", "image", "list")
	if err != nil {
		t.Fatalf("err = %v, stderr = %s", err, errb)
	}
	var rows []imageRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(rows))
	}
	if row := findImageRow(rows, "emobase-portal-deploy", "EMOBASE_GENOMICS_IMAGE"); row == nil || row.Image != "emobase-genomics:local" {
		t.Fatalf("genomics row = %#v", row)
	}
}

func TestImageListAllJSONIncludesServices(t *testing.T) {
	root, _ := imageFixture(t)
	out, errb, err := runImageCLI(t, "--workspace", root, "--json", "image", "list", "--all")
	if err != nil {
		t.Fatalf("err = %v, stderr = %s", err, errb)
	}
	var rows []imageRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(rows) != 15 {
		t.Fatalf("rows = %d, want 15", len(rows))
	}
	if row := findImageRow(rows, "emobase-deploy-tcas", "ONTOLOGYSERVICE_IMAGE"); row == nil || row.Image != "ghcr.io/emobase/ibb-ontologyservice:v1.6.2" {
		t.Fatalf("ontology row = %#v", row)
	}
	if row := findImageRow(rows, "emobase-deploy-tcas", "OBA_IMAGE"); row == nil || row.Image != "openjdk:7" {
		t.Fatalf("oba row = %#v", row)
	}
}

func TestImageListTextShowsDefaults(t *testing.T) {
	root, _ := imageFixture(t)
	out, errb, err := runImageCLI(t, "--workspace", root, "image")
	if err != nil {
		t.Fatalf("err = %v, stderr = %s", err, errb)
	}
	for _, want := range []string{
		"Container images",
		"emobase-genomics:local",
		"ghcr.io/emobase/emobase-portal-web:main",
		"emobase image set",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestImageSetOnlyUpdatesSelectedSpecies(t *testing.T) {
	root, _ := imageFixture(t)
	_, errb, err := runImageCLI(t, "--workspace", root, "image", "set", "web", "emobase-web:local", "--only", "Tcas")
	if err != nil {
		t.Fatalf("err = %v, stderr = %s", err, errb)
	}

	tcas, err := os.ReadFile(filepath.Join(root, "emobase-deploy-tcas", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tcas), "EMOBASE_WEB_IMAGE=emobase-web:local\n") {
		t.Fatalf("tcas .env =\n%s", tcas)
	}
	dmel, err := os.ReadFile(filepath.Join(root, "emobase-deploy-dmel", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dmel), "EMOBASE_WEB_IMAGE") {
		t.Fatalf("dmel .env should be unchanged:\n%s", dmel)
	}
}

func TestImageSetPreservesCommentsAndCreatesBackup(t *testing.T) {
	root, _ := imageFixture(t)
	_, errb, err := runImageCLI(t, "--workspace", root, "image", "set", "genomics", "emobase-genomics:test")
	if err != nil {
		t.Fatalf("err = %v, stderr = %s", err, errb)
	}

	path := filepath.Join(root, "emobase-portal-deploy", ".env")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "# portal\n") {
		t.Fatalf(".env lost its comment:\n%s", data)
	}
	if !strings.Contains(string(data), "EMOBASE_GENOMICS_IMAGE=emobase-genomics:test\n") {
		t.Fatalf(".env =\n%s", data)
	}
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bak), "EMOBASE_GENOMICS_IMAGE=emobase-genomics:local\n") {
		t.Fatalf("backup =\n%s", bak)
	}
}

func TestImageResetRemovesOverride(t *testing.T) {
	root, _ := imageFixture(t)
	out, errb, err := runImageCLI(t, "--workspace", root, "image", "reset", "genomics")
	if err != nil {
		t.Fatalf("err = %v, stderr = %s", err, errb)
	}
	if !strings.Contains(out, "(default: ghcr.io/emobase/emobase-genomics:main)") {
		t.Errorf("output does not show the default:\n%s", out)
	}

	path := filepath.Join(root, "emobase-portal-deploy", ".env")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "EMOBASE_GENOMICS_IMAGE") {
		t.Fatalf("override was not removed:\n%s", data)
	}
	if !strings.Contains(string(data), "# portal\n") || !strings.Contains(string(data), "EMOBASE_PORTAL_WEB_IMAGE=\n") {
		t.Fatalf(".env lost other content:\n%s", data)
	}

	out, _, err = runImageCLI(t, "--workspace", root, "image", "reset", "genomics")
	if err != nil {
		t.Fatalf("second reset: %v", err)
	}
	if !strings.Contains(out, "No image changes for genomics.") {
		t.Fatalf("second reset output:\n%s", out)
	}
}

func TestImageAllIncludesServiceImagesAndTcasOverlay(t *testing.T) {
	_, ws := imageFixture(t)
	rows := imageRows(ws, true)

	ontology := findImageRow(rows, "emobase-deploy-tcas", "ONTOLOGYSERVICE_IMAGE")
	if ontology == nil || ontology.Image != "ghcr.io/emobase/ibb-ontologyservice:v1.6.2" || ontology.Source != imageSourceDefault {
		t.Fatalf("ontology row = %#v", ontology)
	}
	if row := findImageRow(rows, "emobase-deploy-dmel", "ONTOLOGYSERVICE_IMAGE"); row == nil {
		t.Fatal("service images should be listed for every species")
	}
	oba := findImageRow(rows, "emobase-deploy-tcas", "OBA_IMAGE")
	if oba == nil || oba.Image != "openjdk:7" || oba.Source != imageSourceDefault {
		t.Fatalf("oba row = %#v", oba)
	}
	if row := findImageRow(rows, "emobase-deploy-dmel", "OBA_IMAGE"); row != nil {
		t.Fatalf("dmel has no Tcas overlay but an OBA row: %#v", row)
	}
}

func TestImageDefaultListHidesUnchangedServicesButShowsOverrides(t *testing.T) {
	root, _ := imageFixture(t)
	out, _, err := runImageCLI(t, "--workspace", root, "image")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "ONTOLOGYSERVICE_IMAGE") {
		t.Fatalf("default list should hide unchanged service images:\n%s", out)
	}

	_, errb, err := runImageCLI(t, "--workspace", root, "image", "set", "ontology", "ghcr.io/emobase/ibb-ontologyservice:test", "--only", "Tcas")
	if err != nil {
		t.Fatalf("err = %v, stderr = %s", err, errb)
	}
	out, _, err = runImageCLI(t, "--workspace", root, "image")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ONTOLOGYSERVICE_IMAGE") || !strings.Contains(out, "ghcr.io/emobase/ibb-ontologyservice:test") {
		t.Fatalf("default list should show an explicit service override:\n%s", out)
	}
}

func TestImageSetServiceTargetOnlyUpdatesSelectedSpecies(t *testing.T) {
	root, _ := imageFixture(t)
	_, errb, err := runImageCLI(t, "--workspace", root, "image", "set", "ontologyservice", "ghcr.io/emobase/ibb-ontologyservice:test", "--only", "Tcas")
	if err != nil {
		t.Fatalf("err = %v, stderr = %s", err, errb)
	}
	tcas, err := os.ReadFile(filepath.Join(root, "emobase-deploy-tcas", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tcas), "ONTOLOGYSERVICE_IMAGE=ghcr.io/emobase/ibb-ontologyservice:test\n") {
		t.Fatalf("tcas .env =\n%s", tcas)
	}
	dmel, err := os.ReadFile(filepath.Join(root, "emobase-deploy-dmel", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dmel), "ONTOLOGYSERVICE_IMAGE") {
		t.Fatalf("dmel .env should be unchanged:\n%s", dmel)
	}
}

func TestImageResetServiceTarget(t *testing.T) {
	root, _ := imageFixture(t)
	if _, _, err := runImageCLI(t, "--workspace", root, "image", "set", "ontology", "ghcr.io/emobase/ibb-ontologyservice:test", "--only", "Tcas"); err != nil {
		t.Fatal(err)
	}
	out, errb, err := runImageCLI(t, "--workspace", root, "image", "reset", "ontology", "--only", "Tcas")
	if err != nil {
		t.Fatalf("err = %v, stderr = %s", err, errb)
	}
	if !strings.Contains(out, "(default: ghcr.io/emobase/ibb-ontologyservice:v1.6.2)") {
		t.Fatalf("output does not show the service default:\n%s", out)
	}
	tcas, err := os.ReadFile(filepath.Join(root, "emobase-deploy-tcas", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(tcas), "ONTOLOGYSERVICE_IMAGE") {
		t.Fatalf("override was not removed:\n%s", tcas)
	}
}

func TestImageOBARequiresTcasOverlay(t *testing.T) {
	root, _ := imageFixture(t)
	_, errb, err := runImageCLI(t, "--workspace", root, "image", "set", "oba", "azul/zulu-openjdk:7", "--only", "Tcas")
	if err != nil {
		t.Fatalf("err = %v, stderr = %s", err, errb)
	}
	tcas, err := os.ReadFile(filepath.Join(root, "emobase-deploy-tcas", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tcas), "OBA_IMAGE=azul/zulu-openjdk:7\n") {
		t.Fatalf("tcas .env =\n%s", tcas)
	}

	_, _, err = runImageCLI(t, "--workspace", root, "image", "set", "oba", "azul/zulu-openjdk:7", "--only", "Dmel")
	if err == nil || !strings.Contains(err.Error(), "using compose.tcas.yml") {
		t.Fatalf("err = %v", err)
	}
}

func TestImageSetRejectsWhitespace(t *testing.T) {
	root, _ := imageFixture(t)
	_, _, err := runImageCLI(t, "--workspace", root, "image", "set", "genomics", "two words")
	if err == nil || !strings.Contains(err.Error(), "whitespace") {
		t.Fatalf("err = %v", err)
	}
}

func TestImageSetRejectsMismatchedOnly(t *testing.T) {
	root, _ := imageFixture(t)
	_, _, err := runImageCLI(t, "--workspace", root, "image", "set", "web", "emobase-web:local", "--only", "portal")
	if err == nil || !strings.Contains(err.Error(), "does not select a species stack") {
		t.Fatalf("err = %v", err)
	}
}

func TestUnknownImageTarget(t *testing.T) {
	root, _ := imageFixture(t)
	_, _, err := runImageCLI(t, "--workspace", root, "image", "set", "database", "x")
	if err == nil || !strings.Contains(err.Error(), "unknown image target") {
		t.Fatalf("err = %v", err)
	}
}
