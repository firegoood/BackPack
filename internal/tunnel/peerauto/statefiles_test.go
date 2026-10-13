//go:build linux

package peerauto

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenamePreservesFencingAndRefusesLiveOrForeignState(t *testing.T) {
	d := t.TempDir()
	oldPath, newPath := filepath.Join(d, "old.json"), filepath.Join(d, "new.json")
	s, err := openStore(oldPath, "enrolled-manifest")
	if err != nil {
		t.Fatal(err)
	}
	st := s.snapshot()
	st.Generation = 91
	st.LastGood = "wat"
	if err = s.save(st); err != nil {
		t.Fatal(err)
	}
	if MoveStoppedState(oldPath, newPath) == nil {
		t.Fatal("moved beneath live controller")
	}
	s.Close()
	if err = MoveStoppedState(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	s, err = openStore(newPath, "enrolled-manifest")
	if err != nil {
		t.Fatal(err)
	}
	if s.snapshot().Generation != 92 || s.snapshot().LastGood != "wat" || s.snapshot().Active != "" {
		t.Fatal(s.snapshot())
	}
	s.Close()
	if err = os.WriteFile(oldPath, []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	if MoveStoppedState(newPath, oldPath) == nil {
		t.Fatal("overwrote unrelated state")
	}
	if err = RemoveStoppedState(newPath); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
