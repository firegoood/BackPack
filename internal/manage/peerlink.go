package manage

import (
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/backpack/backpack/config"
)

type watEnrollment struct {
	IranIP   string               `json:"iranIp"`
	KharejIP string               `json:"kharejIp"`
	Settings config.WatICMPConfig `json:"settings"`
}

// Format 3 is an explicit upgrade boundary. Old format 1/2 links are unchanged.
// This is a credential-bearing OOB enrollment, not a publicly shareable URL.
func peerLinkMAC(l ShareLink) string {
	l.EnrollmentMAC = ""
	b, _ := json.Marshal(l)
	h := hmac.New(sha256.New, []byte(l.Tok))
	h.Write([]byte("github.com/backpack/backpack/peer-enrollment/v3:"))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}
func encodePeerLink(l ShareLink) (string, error) {
	l.V = 3
	l.EnrollmentMAC = peerLinkMAC(l)
	if e := validatePeerLink(l); e != nil {
		return "", e
	}
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	if e := json.NewEncoder(z).Encode(l); e != nil {
		return "", e
	}
	if e := z.Close(); e != nil {
		return "", e
	}
	return shareScheme + "3." + base64.RawURLEncoding.EncodeToString(b.Bytes()), nil
}
func decodePeerLink(payload string) (ShareLink, error) {
	var l ShareLink
	if len(payload) > 96<<10 {
		return l, fmt.Errorf("peer setup link exceeds size limit")
	}
	b, e := base64.RawURLEncoding.DecodeString(payload)
	if e != nil {
		return l, e
	}
	z, e := gzip.NewReader(bytes.NewReader(b))
	if e != nil {
		return l, e
	}
	defer z.Close()
	b, e = io.ReadAll(io.LimitReader(z, (64<<10)+1))
	if e != nil || len(b) > 64<<10 {
		return l, fmt.Errorf("damaged or oversized peer setup link")
	}
	if e = strictJSON(b, &l); e != nil {
		return ShareLink{}, e
	}
	if e = validatePeerLink(l); e != nil {
		return ShareLink{}, e
	}
	return l, nil
}
func strictJSON(b []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return e
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("trailing enrollment data")
	}
	return nil
}
func validatePeerLink(l ShareLink) error {
	if l.V != 3 || l.Kind != "reverse" || (l.From != "iran" && l.From != "kharej") || len(l.Tok) < 32 || ((l.WatEnrollment == "") == (l.AutoEnrollment == "")) {
		return fmt.Errorf("invalid version-3 peer enrollment")
	}
	mac, e := hex.DecodeString(l.EnrollmentMAC)
	want, _ := hex.DecodeString(peerLinkMAC(l))
	if e != nil || !hmac.Equal(mac, want) {
		return fmt.Errorf("peer setup link authentication/integrity check failed; copy the trusted enrollment again")
	}
	if l.AutoEnrollment != "" {
		var a config.AutoConfig
		if len(l.AutoEnrollment) > 8192 {
			return fmt.Errorf("Auto enrollment exceeds size limit")
		}
		if e = strictJSON([]byte(l.AutoEnrollment), &a); e != nil {
			return e
		}
		if l.Tr != "auto" || !a.Enabled {
			return fmt.Errorf("Auto enrollment missing explicit opt-in")
		}
		for _, p := range a.Candidates {
			if p.LocalBind != "" {
				return fmt.Errorf("remote link cannot set local listener overrides")
			}
		}
	} else {
		var w watEnrollment
		if e = strictJSON([]byte(l.WatEnrollment), &w); e != nil {
			return e
		}
		if l.Tr != "waticmp" || w.Settings.PeerIP != "" {
			return fmt.Errorf("invalid native Wat enrollment")
		}
		for _, ip := range []string{w.IranIP, w.KharejIP} {
			p := net.ParseIP(ip)
			if p == nil || p.To4() == nil || p.IsUnspecified() || p.IsMulticast() {
				return fmt.Errorf("Wat setup link needs both explicit peer IPv4 addresses")
			}
		}
	}
	f := mirrorPeerLink(l)
	_, e = specFromPeerNew(f.ToNewTunnel())
	return e
}
func mirrorPeerLink(l ShareLink) PeerForm {
	f := PeerForm{Kind: "reverse", Side: l.PeerSide(), Name: peerName(l), Token: l.Tok, Transport: l.Tr, TunnelPort: "0", Ports: l.Ports, AcceptUDP: l.AcceptUDP, Paired: []string{"token", "waticmp", "auto"}, Note: "Matching new FullPack versions required. Only the explicitly enrolled data endpoints may listen; keep this credential-bearing link private."}
	if l.AutoEnrollment != "" {
		var a config.AutoConfig
		if err := strictJSON([]byte(l.AutoEnrollment), &a); err != nil {
			return PeerForm{Note: "invalid Auto enrollment: copy and validate the trusted setup link again"}
		}
		f.Auto = &a
		f.AcceptUDP = a.AcceptUDP
		f.Ports = strings.Join(a.Ports, ", ")
		for _, p := range a.Candidates {
			if p.Transport == config.WATICMP {
				f.ServerAddr = p.IranIP
				break
			}
			if !p.Disabled {
				f.ServerAddr = hostOf(p.Endpoint)
				break
			}
		}
	}
	if l.WatEnrollment != "" {
		var w watEnrollment
		if err := strictJSON([]byte(l.WatEnrollment), &w); err != nil {
			return PeerForm{Note: "invalid Wat enrollment: copy and validate the trusted setup link again"}
		}
		x := w.Settings
		if f.Side == "iran" {
			x.PeerIP = w.KharejIP
			f.ServerAddr = w.KharejIP
		} else {
			x.PeerIP = w.IranIP
			f.ServerAddr = w.IranIP
		}
		f.WatICMP = &x
	}
	return f
}
func sharePeerLink(name, host string, c config.Config) (string, error) {
	if c.Client.RemoteAddr != "" {
		return "", fmt.Errorf("export the enrollment from Iran, which owns the approved forwarded maps")
	}
	l := ShareLink{Name: name, Kind: "reverse", From: "iran", Tr: "waticmp", Port: "0", Host: host}
	x := c.Server.WatICMP
	l.Tok = c.Server.Token
	l.Ports = strings.Join(c.Server.Ports, ", ")
	l.AcceptUDP = c.Server.ForwardsUDP()
	if c.Client.RemoteAddr != "" {
		l.From = "kharej"
		x = c.Client.WatICMP
		l.Tok = c.Client.Token
	}
	if c.Auto.Enabled {
		a := c.Auto
		a.Candidates = append([]config.AutoCandidate(nil), a.Candidates...)
		for i := range a.Candidates {
			a.Candidates[i].LocalBind = ""
		}
		a.MaxMemoryMB = 0 // a local pressure setting, never privileged peer material
		b, e := json.Marshal(a)
		if e != nil {
			return "", e
		}
		l.Tr = "auto"
		l.AutoEnrollment = string(b)
	} else {
		local := strings.TrimSpace(host)
		if local == "" {
			return "", fmt.Errorf("enter this side's explicit IPv4 address for the native peer link")
		}
		w := watEnrollment{Settings: x}
		w.Settings.PeerIP = ""
		if l.From == "iran" {
			w.IranIP, w.KharejIP = local, x.PeerIP
		} else {
			w.IranIP, w.KharejIP = x.PeerIP, local
		}
		b, e := json.Marshal(w)
		if e != nil {
			return "", e
		}
		l.WatEnrollment = string(b)
	}
	return l.Encode()
}
