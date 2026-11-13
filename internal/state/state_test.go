package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveAndDelete(t *testing.T) {
	root := t.TempDir()
	f, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != Version || f.Stacks == nil {
		t.Fatalf("state = %#v", f)
	}
	e := f.Entry("species:tcas")
	e.Role = "species"
	e.Database = &Database{Name: "directus_tcas", Created: true}
	if err := f.Save(root); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stacks["species:tcas"].Database.Name != "directus_tcas" {
		t.Fatalf("state = %#v", got)
	}
	info, err := os.Stat(Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
	got.Delete("species:tcas")
	if err := got.Save(root); err != nil {
		t.Fatal(err)
	}
	got, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Stacks["species:tcas"]; ok {
		t.Fatal("entry was not deleted")
	}
}

func TestInvalidStateIsReported(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(Path(root)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(root), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Fatal("expected invalid state error")
	}
}
