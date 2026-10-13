//go:build linux

package waticmp

import "golang.org/x/sys/unix"

// Availability does not allocate a socket or change host networking. Actual
// endpoint creation still reports kernel/interface errors separately.
func Availability() (bool, string) {
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var data [2]unix.CapUserData
	if e := unix.Capget(&header, &data[0]); e != nil {
		return false, "cannot inspect effective CAP_NET_RAW"
	}
	if data[0].Effective&(1<<13) == 0 {
		return false, "effective CAP_NET_RAW is absent; run the tunnel with the explicitly approved raw-socket privilege"
	}
	return true, "native ICMP raw-socket privilege available"
}
