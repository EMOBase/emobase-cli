package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeRemoveDirRemovesNestedDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "emobase-deploy-tcas")
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SafeRemoveDir(root, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("directory still exists: %v", err)
	}
}

func TestSafeRemoveDirRejectsWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	if err := SafeRemoveDir(root, root); err == nil {
		t.Fatal("expected the workspace root to be rejected")
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("workspace root was removed: %v", err)
	}
}

func TestSafeRemoveDirRejectsOutsidePath(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "victim")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SafeRemoveDir(root, outside); err == nil {
		t.Fatal("expected an outside path to be rejected")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside directory was removed: %v", err)
	}
}

func TestSafeRemoveDirRemovesSymlinkOnly(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "victim")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "emobase-deploy-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := SafeRemoveDir(root, link); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("symlink still exists: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("symlink target was removed: %v", err)
	}
}
