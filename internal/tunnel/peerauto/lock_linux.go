//go:build linux

package peerauto

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func lockState(path string) (*os.File, error) {
	// Refuse symlinks: the configured state path is never a remote argument.
	fd, e := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), path)
	info, statErr := f.Stat()
	if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		f.Close()
		return nil, fmt.Errorf("Auto lock must be a private regular file")
	}
	if e = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, fmt.Errorf("another coordinator owns this tunnel: %w", e)
	}
	return f, nil
}
func unlockState(f *os.File) { unix.Flock(int(f.Fd()), unix.LOCK_UN); f.Close() }

func openStateRead(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 8192 {
		f.Close()
		return nil, fmt.Errorf("Auto state must be a regular private file")
	}
	return f, nil
}
