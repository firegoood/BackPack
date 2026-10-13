//go:build !linux

package enginectl

func claimSocket(string) (func(), error) { return func() {}, nil }
