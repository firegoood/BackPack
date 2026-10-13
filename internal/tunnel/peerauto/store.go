package peerauto

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type durableState struct {
	Version     int     `json:"version"`
	Manifest    string  `json:"manifest"`
	Generation  uint64  `json:"generation"`
	Active      string  `json:"active"`
	LastGood    string  `json:"lastGood"`
	Intent      string  `json:"intent,omitempty"`
	Transaction string  `json:"transaction,omitempty"`
	Switches    []int64 `json:"switches,omitempty"`
	LastChange  int64   `json:"lastChange,omitempty"`
}
type stateStore struct {
	mu    sync.Mutex
	path  string
	lock  *os.File
	state durableState
}

func openStore(path, manifest string) (*stateStore, error) {
	if path == "" {
		return nil, errors.New("Auto needs a durable state path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	lock, err := lockState(path + ".lock")
	if err != nil {
		return nil, err
	}
	s := &stateStore{path: path, lock: lock, state: durableState{Version: 1, Manifest: manifest}}
	if info, e := os.Lstat(path); e == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		s.Close()
		return nil, errors.New("Auto state must be a regular private file, not a symlink")
	}
	f, err := openStateRead(path)
	if err == nil {
		st, e := f.Stat()
		if e != nil || st.Mode().Perm()&0077 != 0 {
			f.Close()
			s.Close()
			return nil, errors.New("Auto state must have private permissions")
		}
		b, e := io.ReadAll(io.LimitReader(f, 8193))
		f.Close()
		if e != nil || len(b) > 8192 || json.Unmarshal(b, &s.state) != nil || s.state.Version != 1 || s.state.Generation >= 1<<62 || len(s.state.Switches) > 12 {
			s.Close()
			return nil, errors.New("invalid Auto state; restore a verified backup or re-enroll both peers")
		}
		if s.state.Manifest != manifest {
			s.Close()
			return nil, errors.New("enrollment policy changed: preserve old state and re-enroll this pair before Auto activation")
		}
	} else if !os.IsNotExist(err) {
		s.Close()
		return nil, err
	}
	// Persist a reboot boundary. Fresh cryptographic incarnation is separate:
	// restored snapshots cannot reuse control/data authorizations or nonces.
	s.state.Generation++
	if err := s.save(s.state); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}
func (s *stateStore) Close() {
	if s.lock != nil {
		unlockState(s.lock)
		s.lock = nil
	}
}
func (s *stateStore) snapshot() durableState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state
	st.Switches = append([]int64(nil), st.Switches...)
	return st
}
func (s *stateStore) save(st durableState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st.Generation < s.state.Generation || st.Generation > 1<<62 {
		return errors.New("stale/exhausted generation")
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".auto-state-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, s.path)
	}
	if err == nil {
		d, e := os.Open(filepath.Dir(s.path))
		if e == nil {
			e = d.Sync()
			d.Close()
		}
		err = e
	}
	if err != nil {
		return fmt.Errorf("persist Auto fencing: %w", err)
	}
	s.state = st
	return nil
}
