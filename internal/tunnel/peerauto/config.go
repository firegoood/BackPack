// Package peerauto owns an opted-in enrolled tunnel's listeners, carrier
// sessions and switching decisions. Manual engines and legacy chain stay separate.
package peerauto

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"regexp"
	"runtime"
	"strconv"

	"github.com/backpack/backpack/config"
	"github.com/backpack/backpack/internal/tunnel/portmap"
	"github.com/backpack/backpack/internal/tunnel/waticmp"
	"github.com/backpack/backpack/internal/utils/network"
)

var identity = regexp.MustCompile(`^[a-zA-Z0-9_-]{16,64}$`)
var candidateID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,16}$`)

func Defaults(a config.AutoConfig) config.AutoConfig {
	if a.ProbeInterval == 0 {
		a.ProbeInterval = 600
	}
	if a.Cooldown == 0 {
		a.Cooldown = 300
	}
	if a.MinimumDwell == 0 {
		a.MinimumDwell = 600
	}
	if a.MaxSwitchesPerHour == 0 {
		a.MaxSwitchesPerHour = 2
	}
	if a.FailureThreshold == 0 {
		a.FailureThreshold = 3
	}
	if a.MaxConnections == 0 {
		a.MaxConnections = 64
	}
	if a.MaxMemoryMB == 0 {
		a.MaxMemoryMB = 256
	}
	return a
}

func Validate(c *config.Config) error {
	if !c.Auto.Enabled {
		return nil
	}
	if runtime.GOOS != "linux" || os.Getenv("BACKPACK_DISABLE_AUTO") == "1" {
		return fmt.Errorf("peer Auto is disabled on this host")
	}
	a := Defaults(c.Auto)
	if a.AllowPlaintextFallback {
		return fmt.Errorf("plaintext Auto adapters are not verified in this version; allow_plaintext_fallback must remain false")
	}
	if a.MaxMemoryMB < 64 || a.MaxMemoryMB > 4096 {
		return fmt.Errorf("Auto memory pressure guard must be 64-4096 MB")
	}
	if c.L3.Enabled() || c.Direct.Enabled() {
		return fmt.Errorf("peer Auto currently supports reverse stream forwarding; Direct/L3 is unsupported")
	}
	if (c.Server.BindAddr != "") == (c.Client.RemoteAddr != "") {
		return fmt.Errorf("Auto needs exactly one Iran/server or Kharej/client role")
	}
	if !identity.MatchString(a.TunnelID) || !identity.MatchString(a.IranID) || !identity.MatchString(a.KharejID) || a.IranID == a.KharejID {
		return fmt.Errorf("Auto requires distinct stable enrolled Iran/Kharej identities and a tunnel ID (16-64 safe characters)")
	}
	token := c.Client.Token
	if c.Server.BindAddr != "" {
		token = c.Server.Token
	}
	if len(token) < 32 {
		return fmt.Errorf("Auto enrollment secret must have at least 32 bytes")
	}
	if len(c.Server.FallbackTransports)+len(c.Client.FallbackTransports)+len(c.Client.FallbackAddrs) != 0 || c.Client.HealthFailover || c.Client.LoadBalance {
		return fmt.Errorf("Auto is the sole switching owner; remove legacy carrier/address fallback for this opted-in tunnel")
	}
	if c.Server.Naive.Enabled() || c.Client.Naive.Enabled() || c.Server.Xray.Enabled() || c.Client.Xray.Enabled() || c.Server.PPROF || c.Client.PPROF {
		return fmt.Errorf("managed helpers/profiling are not supported by peer Auto; keep this tunnel in manual mode")
	}
	if c.Server.ProxyProtocol || c.Server.ZeroCopy || c.Client.ZeroCopy || c.Server.Sniffer || c.Client.Sniffer || c.Server.WebPort != 0 || c.Client.WebPort != 0 || c.Server.BandwidthMbps != 0 {
		return fmt.Errorf("Auto does not implement Proxy Protocol, zero-copy, per-port sniffer/monitor or the legacy bandwidth limit; disable these explicitly")
	}
	if c.Client.Proxy != "" || c.Client.LocalAddr != "" || c.Client.Interface != "" || c.Client.EdgeIP != "" || c.Client.MSS != 0 || c.Server.MSS != 0 {
		return fmt.Errorf("Auto does not implement legacy proxy, source/interface binding, CDN edge override or MSS settings; remove them explicitly")
	}
	if c.Server.BindAddr != "" && c.Server.AcceptUDP != nil && c.Server.ForwardsUDP() != a.AcceptUDP {
		return fmt.Errorf("Auto UDP policy must match the Iran server setting")
	}
	maps, err := portmap.Expand(a.Ports, "127.0.0.1")
	if c.Server.SimpleAuth || c.Client.SimpleAuth {
		return fmt.Errorf("Auto cannot use raw-token/simple-auth TLS proxy mode")
	}
	if err != nil || len(maps) == 0 || len(maps) > 32 {
		return fmt.Errorf("Auto requires 1-32 valid enrolled forwarded port mappings: %v", err)
	}
	if c.Server.BindAddr != "" {
		x, err := portmap.Expand(c.Server.Ports, "127.0.0.1")
		b, _ := json.Marshal(x)
		d, _ := json.Marshal(maps)
		if err != nil || string(b) != string(d) {
			return fmt.Errorf("Iran forwarded maps must match the enrolled Auto manifest")
		}
	}
	if a.MaxConnections < 1 || a.MaxConnections > 192 || a.ProbeInterval < 30 || a.ProbeInterval > 86400 || a.Cooldown < 5 || a.MinimumDwell < 5 || a.MaxSwitchesPerHour < 1 || a.MaxSwitchesPerHour > 12 || a.FailureThreshold < 3 || a.FailureThreshold > 20 {
		return fmt.Errorf("Auto resource/timing policy outside its conservative bounds")
	}
	if len(a.Candidates) < 1 || len(a.Candidates) > 4 {
		return fmt.Errorf("Auto needs 1-4 explicitly enrolled candidates")
	}
	seen, endpoints, enabled := map[string]bool{}, map[string]bool{}, 0
	lockFound := a.ManualLock == ""
	for _, p := range a.Candidates {
		if !candidateID.MatchString(p.ID) || seen[p.ID] {
			return fmt.Errorf("invalid/duplicate Auto candidate ID")
		}
		seen[p.ID] = true
		switch p.Transport {
		case config.WATICMP, config.STEALTH, config.QUIC, config.WSS, config.WSSMUX:
		default:
			return fmt.Errorf("candidate %s uses an unverified Auto adapter %s", p.ID, p.Transport)
		}
		if p.Disabled {
			continue
		}
		enabled++
		if p.ID == a.ManualLock {
			lockFound = true
		}
		switch p.Transport {
		case config.WATICMP:
			if err := watSettings(a, p, token, true).Validate(); err != nil {
				return err
			}
			if err := watSettings(a, p, token, false).Validate(); err != nil {
				return err
			}
		case config.STEALTH, config.QUIC, config.WSS, config.WSSMUX:
			host, port, err := net.SplitHostPort(p.Endpoint)
			n, _ := strconv.Atoi(port)
			if err != nil || host == "" || n < 1 || n > 65535 {
				return fmt.Errorf("candidate %s needs an explicitly permitted Iran host:port", p.ID)
			}
			// A wildcard/NAT local bind can collide even when public hostnames
			// differ. Require a distinct enrolled port within each socket family.
			key := "tcp:" + port
			if p.Transport == config.QUIC {
				key = "udp:" + port
			}
			if endpoints[key] {
				return fmt.Errorf("candidate listeners conflict: %s", key)
			}
			for _, mapping := range maps {
				_, userPort, _ := net.SplitHostPort(mapping.Listen)
				if userPort == port && (p.Transport != config.QUIC || a.AcceptUDP) {
					return fmt.Errorf("candidate %s port conflicts with a forwarded user listener", p.ID)
				}
			}
			endpoints[key] = true
			if p.LocalBind != "" {
				_, bindPort, err := net.SplitHostPort(p.LocalBind)
				if err != nil || bindPort != port {
					return fmt.Errorf("candidate local bind must use the enrolled endpoint port")
				}
			}
		default:
			return fmt.Errorf("Auto candidate %s: transport %s has no verified peer adapter; use manual mode", p.ID, p.Transport)
		}
	}
	if enabled == 0 || !lockFound {
		return fmt.Errorf("Auto has no enabled candidate or manual lock names a disabled candidate")
	}
	return nil
}

func manifestHash(a config.AutoConfig) string {
	// Local runtime state/observe choice never changes the enrolled authority,
	// endpoints, security floor or forwarding authorization.
	a.ObserveOnly = false
	a.MaxMemoryMB = 0
	a.Candidates = append([]config.AutoCandidate(nil), a.Candidates...)
	for i := range a.Candidates {
		a.Candidates[i].LocalBind = ""
	}
	b, _ := json.Marshal(Defaults(a))
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func watSettings(a config.AutoConfig, p config.AutoCandidate, token string, iran bool) waticmp.Settings {
	ip := p.IranIP
	if iran {
		ip = p.KharejIP
	}
	h := sha256.Sum256([]byte(a.TunnelID + ":" + p.ID))
	return waticmp.Settings{Enabled: true, PeerIP: ip, TunnelID: hex.EncodeToString(h[:]), Direction: p.Direction, Token: token,
		MaxStreams: min(256, Defaults(a).MaxConnections+16), MaxPayload: p.MaxPayload, MaxBps: 1_000_000, Paths: 4}
}

func tlsSettings(c *config.Config) network.TLSSettings {
	return network.TLSSettings{CertFile: c.Server.TLSCertFile, KeyFile: c.Server.TLSKeyFile, ACMEDomain: c.Server.ACMEDomain, ACMEEmail: c.Server.ACMEEmail}
}
