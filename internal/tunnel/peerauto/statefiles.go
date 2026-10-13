package peerauto

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// MoveStoppedState preserves generation/switch history across a local rename.
// Both locks must be available: never move the state beneath a live controller.
// Config/service lifecycle remains owned by the caller, which can move it back
// before starting either service if another part of a rename fails.
func MoveStoppedState(oldPath, newPath string) error {
	if oldPath == newPath {
		return nil
	}
	f, err := openStateRead(oldPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	f.Close()
	oldLock, err := lockState(oldPath + ".lock")
	if err != nil {
		return err
	}
	defer unlockState(oldLock)
	newLock, err := lockState(newPath + ".lock")
	if err != nil {
		return err
	}
	defer unlockState(newLock)
	// Recheck after acquiring ownership: a stopped controller could have
	// replaced the source between the initial existence check and the lock.
	f, err = openStateRead(oldPath)
	if err != nil {
		return err
	}
	f.Close()
	if _, err := os.Lstat(newPath); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("destination Auto state already exists or cannot be checked: %v", err)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(newPath))
	if err != nil {
		return errors.Join(err, os.Rename(newPath, oldPath))
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return errors.Join(err, os.Rename(newPath, oldPath))
	}
	return nil
}

// RemoveStoppedState is called only after the tunnel service has stopped.
// Persistent .lock inodes are retained to avoid creating a second lock domain
// while a racing local opener holds the original inode; uninstall removes the
// whole private config directory. They contain no key or live socket.
func RemoveStoppedState(path string) error {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	l, err := lockState(path + ".lock")
	if err != nil {
		return err
	}
	defer unlockState(l)
	f, err := openStateRead(path)
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(path)
}
