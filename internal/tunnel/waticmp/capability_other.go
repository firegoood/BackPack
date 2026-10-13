//go:build !linux

package waticmp

func Availability() (bool, string) { return false, "native Wat ICMP requires Linux" }
