package manage

import (
	"fmt"
	"os"
	"path/filepath"
)

// RemoveInstalledHelpers removes only the installer's dedicated namespace.
// Operator-supplied executable paths elsewhere are never cleanup targets.
func RemoveInstalledHelpers() error {
	return removeInstalledHelperTree("/usr/local/lib/fullpack")
}

func removeInstalledHelperTree(base string) error {
	root := filepath.Join(base, "helpers")
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	// Reject redirected ancestors before recursive cleanup. Children that are
	// symlinks are removed as links by os.RemoveAll, never followed. The default
	// /usr/local/lib ancestry is root-owned; base/root must not be writable by
	// other users. A customized/redirection path needs explicit manual cleanup.
	for path := root; ; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("helper cleanup refuses a redirected or non-directory path: %s", path)
		}
		if (path == base || path == root) && info.Mode().Perm()&0022 != 0 {
			return fmt.Errorf("helper cleanup refuses a directory writable by other users: %s", path)
		}
		if filepath.Dir(path) == path {
			break
		}
	}
	return os.RemoveAll(root)
}
