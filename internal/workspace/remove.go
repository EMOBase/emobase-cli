package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SafeRemoveDir deletes a checkout directory only when it is strictly inside
// the workspace root. It refuses the root itself, paths outside the root and
// symlinks that resolve outside the root. A missing directory is not an error.
func SafeRemoveDir(root, dir string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve workspace root: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return fmt.Errorf("resolve workspace root %s: %w", rootAbs, err)
	}
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", dir, err)
	}
	if !within(rootAbs, dirAbs) {
		return fmt.Errorf("refusing to remove %s: it is not inside the workspace %s", dirAbs, rootAbs)
	}

	info, err := os.Lstat(dirAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect %s: %w", dirAbs, err)
	}
	// Remove a symlink itself rather than following it. The lexical check
	// above already proved the link lives inside the workspace.
	if info.Mode()&os.ModeSymlink != 0 {
		return os.Remove(dirAbs)
	}
	if !info.IsDir() {
		return fmt.Errorf("refusing to remove %s: it is not a directory", dirAbs)
	}

	dirReal, err := filepath.EvalSymlinks(dirAbs)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", dirAbs, err)
	}
	if dirReal == rootReal {
		return fmt.Errorf("refusing to remove %s: it is the workspace root", dirAbs)
	}
	if !within(rootReal, dirReal) {
		return fmt.Errorf("refusing to remove %s: it resolves outside the workspace %s", dirAbs, rootReal)
	}
	return os.RemoveAll(dirReal)
}

func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
