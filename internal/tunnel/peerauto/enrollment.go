package peerauto

import (
	"encoding/json"
	"fmt"
	"github.com/backpack/backpack/config"
	"io"
	"os"
)

// CheckEnrollmentState refuses a paired policy change before a UI writes an
// unusable config over a working pair. Re-enrollment uses a new tunnel identity
// and preserves the existing files/counters for rollback.
func CheckEnrollmentState(path string, a config.AutoConfig) error {
	if !a.Enabled {
		return nil
	}
	info, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 8192 {
		return fmt.Errorf("existing Auto fencing state is not a private regular bounded file")
	}
	f, e := openStateRead(path)
	if e != nil {
		return e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 8193))
	if e != nil || len(b) > 8192 {
		return fmt.Errorf("existing Auto fencing state cannot be read within its bound")
	}
	var st durableState
	if json.Unmarshal(b, &st) != nil || st.Version != 1 {
		return fmt.Errorf("existing Auto fencing state is invalid")
	}
	if st.Manifest != manifestHash(a) {
		return fmt.Errorf("shared Auto enrollment changed: preserve this working pair and its fencing state; create/re-enroll a new pair on both peers instead")
	}
	return nil
}
