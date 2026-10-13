package manage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	"github.com/backpack/backpack/config"
	"github.com/backpack/backpack/internal/tunnel/peerauto"
)

// The same schema/validators serve terminal setup, web setup and peer links.
// ICMP has no TCP/UDP tunnel port; 0 is only the engine's role marker.
func specFromPeerNew(n NewTunnel) (TunnelSpec, error) {
	s := TunnelSpec{Name: strings.TrimSpace(n.Name), Role: n.Role, Transport: n.Transport, Token: strings.TrimSpace(n.Token), LogLevel: "info", Heartbeat: 5, RetryInterval: 1, ConnectionPool: 4}
	if !validName(s.Name) {
		return s, fmt.Errorf("invalid tunnel name")
	}
	if s.Role != "server" && s.Role != "client" {
		return s, fmt.Errorf("role must be server or client")
	}
	if n.IPv6 || n.ProxyProtocol || n.Managed != nil || n.Pck != nil || n.Conn != nil || n.Limits != nil {
		return s, fmt.Errorf("these legacy/helper settings are not supported by Wat/Auto setup")
	}
	if n.Tune != nil {
		n.Tune.apply(&s)
	}
	if s.Role == "server" {
		s.BindAddr = "0.0.0.0:0"
		s.Ports = parsePorts(n.Ports)
		if e := validatePortSpecs(s.Ports); e != nil {
			return s, e
		}
		if len(s.Ports) == 0 {
			return s, fmt.Errorf("at least one forwarded port is required")
		}
	}
	if n.Transport == "auto" {
		if n.Auto == nil || !n.Auto.Enabled {
			return s, fmt.Errorf("Auto requires explicit enabled enrollment with both peer IDs and approved candidates")
		}
		s.Auto = peerauto.Defaults(*n.Auto)
		s.Auto.Candidates = append([]config.AutoCandidate(nil), s.Auto.Candidates...)
		if len(s.Auto.Candidates) == 0 {
			return s, fmt.Errorf("no enrolled candidates")
		}
		first := -1
		for i, p := range s.Auto.Candidates {
			if !p.Disabled {
				first = i
				break
			}
		}
		if first < 0 {
			return s, fmt.Errorf("all candidates are disabled")
		}
		p := s.Auto.Candidates[first]
		s.Transport = string(p.Transport)
		s.AcceptUDP = s.Auto.AcceptUDP
		if s.Role == "server" {
			if len(s.Auto.Ports) == 0 {
				s.Auto.Ports = append([]string(nil), s.Ports...)
			}
		} else {
			s.Ports = nil
		}
		if p.Transport == config.WATICMP {
			h := sha256.Sum256([]byte(s.Auto.TunnelID + ":" + p.ID))
			peer := p.IranIP
			if s.Role == "server" {
				peer = p.KharejIP
			}
			s.WatICMP = config.WatICMPConfig{Enabled: true, PeerIP: peer, TunnelID: hex.EncodeToString(h[:]), Direction: p.Direction, MaxStreams: min(256, s.Auto.MaxConnections+16), MaxPayload: p.MaxPayload}
			if s.Role == "client" {
				s.RemoteAddr = net.JoinHostPort(p.IranIP, "0")
			}
		} else {
			if s.Role == "server" {
				s.BindAddr = p.LocalBind
				if s.BindAddr == "" {
					s.BindAddr = p.Endpoint
				}
			} else {
				s.RemoteAddr = p.Endpoint
			}
		}
	} else {
		if n.WatICMP == nil {
			return s, fmt.Errorf("Wat ICMP requires explicit settings")
		}
		s.WatICMP = *n.WatICMP
		if s.Role == "client" {
			s.RemoteAddr = net.JoinHostPort(s.WatICMP.PeerIP, "0")
		}
	}
	if e := s.ValidatePeerFeatures(); e != nil {
		return s, e
	}
	return s, nil
}
