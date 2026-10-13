package manage

import "fmt"

// PeerNewTunnel accepts only the new credential-bearing enrollment format.
// Legacy panel/fleet pairing is unchanged. The operator must obtain this link
// over a trusted out-of-band channel; its embedded PSK cannot authenticate the
// channel that delivered a wholly substituted link.
func PeerNewTunnel(raw, name, transport string) (NewTunnel, error) {
	l, err := DecodeShareLink(raw)
	if err != nil {
		return NewTunnel{}, err
	}
	if l.V != 3 || l.From != "iran" || l.Kind != "reverse" || (l.Tr != "auto" && l.Tr != "waticmp") || l.Tr != transport {
		return NewTunnel{}, fmt.Errorf("use the matching trusted version-3 Iran enrollment link; both peers require this feature version")
	}
	n := MirrorForPeer(l).ToNewTunnel()
	if name != "" {
		n.Name = name
	}
	if _, err := specFromPeerNew(n); err != nil {
		return NewTunnel{}, err
	}
	return n, nil
}
