package envfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadPreservesCommentsAndQuotes(t *testing.T) {
	path := writeTemp(t, "# header\n\nA=1\nexport B=\"two words\"\nC='single # not a comment'\n# tail\n")
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := f.Get("A"); got != "1" {
		t.Fatalf("A = %q", got)
	}
	if got, _ := f.Get("B"); got != "two words" {
		t.Fatalf("B = %q", got)
	}
	if got, _ := f.Get("C"); got != "single # not a comment" {
		t.Fatalf("C = %q", got)
	}
	if got := f.Render(); got != "# header\n\nA=1\nexport B=\"two words\"\nC='single # not a comment'\n# tail\n" {
		t.Fatalf("Render changed the file:\n%s", got)
	}
	if f.Dirty() {
		t.Fatal("loaded file should not be dirty")
	}
}

func TestGetReturnsLastDefinition(t *testing.T) {
	f, err := Load(writeTemp(t, "A=first\nA=second\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := f.Get("A"); got != "second" {
		t.Fatalf("Get = %q, want second", got)
	}
}

func TestSetUpdatesInPlace(t *testing.T) {
	f, err := Load(writeTemp(t, "# keep me\nA=old\nB=two\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !f.Set("A", "new") {
		t.Fatal("Set reported no change")
	}
	if got := f.Render(); got != "# keep me\nA=new\nB=two\n" {
		t.Fatalf("Render =\n%s", got)
	}
	if f.Set("A", "new") {
		t.Fatal("setting the same value should be a no-op")
	}
}

func TestSetAppendsBeforeTrailingNewline(t *testing.T) {
	f, err := Load(writeTemp(t, "A=1\n"))
	if err != nil {
		t.Fatal(err)
	}
	f.Set("B", "2")
	if got := f.Render(); got != "A=1\nB=2\n" {
		t.Fatalf("Render = %q", got)
	}
}

func TestSetAppendsToFileWithoutTrailingNewline(t *testing.T) {
	f, err := Load(writeTemp(t, "A=1"))
	if err != nil {
		t.Fatal(err)
	}
	f.Set("B", "2")
	if got := f.Render(); got != "A=1\nB=2" {
		t.Fatalf("Render = %q", got)
	}
}

func TestSetQuotesValuesThatNeedIt(t *testing.T) {
	f, err := Load(writeTemp(t, "A=1\n"))
	if err != nil {
		t.Fatal(err)
	}
	f.Set("B", "two words")
	f.Set("C", `has"quote`)
	if got := f.Render(); got != "A=1\nB=\"two words\"\nC=\"has\\\"quote\"\n" {
		t.Fatalf("Render = %q", got)
	}
}

func TestLoadOrEmptyCreatesEditableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	f, err := LoadOrEmpty(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Dirty() {
		t.Fatal("empty file should not be dirty")
	}
	f.Set("A", "1")
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "A=1\n" {
		t.Fatalf("saved %q", data)
	}
}

func TestSaveKeepsBackupAndMode(t *testing.T) {
	path := writeTemp(t, "A=old\n")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Set("A", "new")
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(bak) != "A=old\n" {
		t.Fatalf("backup = %q", bak)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := st.Mode().Perm(); mode != 0o640 {
		t.Fatalf("mode = %o", mode)
	}
	if f.Dirty() {
		t.Fatal("file should be clean after Save")
	}
}

func TestCRLFIsNormalised(t *testing.T) {
	f, err := Load(writeTemp(t, "A=1\r\nB=2\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := f.Get("B"); got != "2" {
		t.Fatalf("B = %q", got)
	}
	if strings.Contains(f.Render(), "\r") {
		t.Fatal("Render kept CRLF")
	}
}

func TestKeysAreUniqueInOrder(t *testing.T) {
	f, err := Load(writeTemp(t, "B=1\nA=2\nB=3\n# c\nC=4\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(f.Keys(), ",")
	if got != "B,A,C" {
		t.Fatalf("Keys = %q", got)
	}
}
