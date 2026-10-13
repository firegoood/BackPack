//go:build !linux

package peerauto

import (
	"errors"
	"os"
)

func lockState(string) (*os.File, error)     { return nil, errors.New("peer Auto is Linux-only") }
func unlockState(f *os.File)                 { f.Close() }
func openStateRead(string) (*os.File, error) { return nil, errors.New("peer Auto is Linux-only") }
