package manage

import (
	"fmt"
	"reflect"
	"strings"
)

func editedPeerSpec(s TunnelSpec, e TunnelEdit) (TunnelSpec, bool, error) {
	before := s
	if e.Auto != nil && s.Auto.TunnelID == "" {
		return s, false, fmt.Errorf("create a new explicitly enrolled pair to enable Auto on a legacy tunnel")
	}
	if e.Managed != nil || e.Pck != nil || e.Conn != nil || e.Limits != nil || e.ProxyProtocol != nil || e.Preset != "" || e.ACMEDomain != nil || e.ACMEEmail != nil {
		return s, false, fmt.Errorf("use the native/Auto settings; these legacy options are not supported")
	}
	if s.Auto.TunnelID != "" || e.Auto != nil {
		if e.Auto != nil {
			s.Auto = *e.Auto
		}
		// Shared enrollment edits are refused by the state guard before mutation.
		// Local observe/pressure choices remain editable without a new enrollment.
		if e.Ports != "" {
			s.Ports = parsePorts(e.Ports)
		}
		if e.Transport != "" && e.Transport != "auto" && e.Transport != s.Transport {
			return s, false, fmt.Errorf("disable Auto on both peers before changing the manual transport")
		}
	} else {
		if e.WatICMP != nil {
			s.WatICMP = *e.WatICMP
		}
		if e.Transport != "" && e.Transport != "waticmp" {
			return s, false, fmt.Errorf("use a new manual tunnel to change the transport")
		}
		if e.Ports != "" {
			s.Ports = parsePorts(e.Ports)
			if err := validatePortSpecs(s.Ports); err != nil {
				return s, false, err
			}
		}
		if e.ServerAddr != "" || e.TunnelPort != "" && e.TunnelPort != "0" {
			return s, false, fmt.Errorf("Wat has no TCP/UDP tunnel port; use its explicit peer IPv4 setting")
		}
		if s.Role == "client" {
			s.RemoteAddr = s.WatICMP.PeerIP + ":0"
		}
	}
	if e.Tune != nil {
		e.Tune.apply(&s)
	}
	if err := s.ValidatePeerFeatures(); err != nil {
		return before, false, err
	}
	return s, !reflect.DeepEqual(before, s), nil
}

func isPeerSpec(s TunnelSpec) bool {
	return s.Auto.TunnelID != "" || strings.EqualFold(s.Transport, "waticmp")
}
