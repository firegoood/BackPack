package config

// AutoConfig is an enrolled pair's policy. It never infers public endpoints
// from fallback addresses or creates an unapproved management listener.
type AutoConfig struct {
	// Enabled opts into the enrolled coordinator; false/absent keeps the manual engine and legacy fallback.
	Enabled bool `toml:"enabled" json:"enabled"`
	// ObserveOnly bootstraps the primary/last-good route and measures candidates without automatic cross-carrier changes.
	ObserveOnly bool `toml:"observe_only" json:"observeOnly"`
	// TunnelID is the immutable 16-64 safe-character pair identity.
	TunnelID string `toml:"tunnel_id" json:"tunnelId"`
	// IranID identifies the sole switching authority, independent of packet direction.
	IranID string `toml:"iran_id" json:"iranId"`
	// KharejID identifies the distinct enrolled follower; changing identities requires a new pair.
	KharejID string `toml:"kharej_id" json:"kharejId"`
	// These exact maps are authenticated as part of the manifest. The follower
	// dials only this enrolled backend allowlist, indexed by mapping number.
	Ports []string `toml:"ports" json:"ports"`
	// AcceptUDP permits framed UDP forwarding on the enrolled Iran user ports.
	AcceptUDP bool `toml:"accept_udp" json:"acceptUdp"`
	// Candidates lists 1-4 explicitly approved endpoints, independent of the active carrier.
	Candidates []AutoCandidate `toml:"candidates" json:"candidates"`
	// AllowPlaintextFallback must remain false: plaintext Auto adapters are unsupported in this version.
	AllowPlaintextFallback bool `toml:"allow_plaintext_fallback" json:"allowPlaintextFallback"`
	// AllowDegradationSwitch optionally permits changing a reachable path after sustained severe measured degradation.
	AllowDegradationSwitch bool `toml:"allow_degradation_switch" json:"allowDegradationSwitch"`
	// ProbeInterval is seconds between paced bulk tests, default 600, minimum 30; schedules include jitter.
	ProbeInterval int `toml:"probe_interval" json:"probeInterval"` // seconds, default 600
	// Cooldown is seconds between cross-carrier changes, default 300, minimum 5.
	Cooldown int `toml:"cooldown" json:"cooldown"` // seconds, default 300
	// MinimumDwell is seconds before optional degradation switching, default 600, minimum 5.
	MinimumDwell int `toml:"minimum_dwell" json:"minimumDwell"` // seconds, default 600
	// MaxSwitchesPerHour limits persisted cross-carrier commits, default 2, range 1-12.
	MaxSwitchesPerHour int `toml:"max_switches_per_hour" json:"maxSwitchesPerHour"` // default 2
	// FailureThreshold debounces carrier failure, default 3, range 3-20; skipped probes do not count.
	FailureThreshold int `toml:"failure_threshold" json:"failureThreshold"` // consecutive actual path failures, default 3
	// MaxConnections bounds simultaneous user connections, default 64, maximum 192; probes reserve separate streams.
	MaxConnections int `toml:"max_connections" json:"maxConnections"` // default 64, maximum 192
	// ManualLock fixes the allowed active candidate ID; runtime unlock cannot weaken an enrolled lock.
	ManualLock string `toml:"manual_lock" json:"manualLock"`
	// MaxMemoryMB skips probes above this local process RSS pressure threshold, default 256 MB; it is not a hard memory limit.
	MaxMemoryMB int `toml:"max_memory_mb" json:"maxMemoryMb"` // default 256
}

type AutoCandidate struct {
	// ID identifies an approved candidate, 1-16 safe characters, unique within this pair.
	ID string `toml:"id" json:"id"`
	// Transport is waticmp, stealth, quic, wss or wssmux; other Auto adapters are unverified and refused.
	Transport TransportType `toml:"transport" json:"transport"`
	// Disabled prevents this candidate from opening a listener or dialing after startup.
	Disabled bool `toml:"disabled" json:"disabled"`
	// Endpoint is the explicitly permitted Iran listener Kharej dials.
	Endpoint string `toml:"endpoint" json:"endpoint"`
	// LocalBind is an optional Iran-only local address using the same approved endpoint port; never exported to the peer.
	LocalBind string `toml:"local_bind" json:"localBind,omitempty"`
	// IranIP is Iran's explicit unicast IPv4 for a Wat candidate.
	IranIP string `toml:"iran_ip" json:"iranIp"`
	// KharejIP is Kharej's explicit unicast IPv4 for a Wat candidate.
	KharejIP string `toml:"kharej_ip" json:"kharejIp"`
	// Direction is normal or reverse for Wat Echo Requests; application users still enter on Iran.
	Direction string `toml:"direction" json:"direction"`
	// MaxPayload caps Wat datagrams at 256-1400 bytes, default 512, subject to authenticated PMTU discovery.
	MaxPayload int `toml:"max_payload" json:"maxPayload"`
}
