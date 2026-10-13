package config

// WatICMPConfig is opt-in and distinct from xDi/KCP. It configures the native
// reverse-stream carrier; it is not a Direct L3/TUN carrier.
type WatICMPConfig struct {
	// Enabled explicitly opts into native Wat; absent/false preserves legacy behavior.
	Enabled bool `toml:"enabled" json:"enabled"`
	// PeerIP is the other host's explicit unicast IPv4 route; source IP alone is not authentication.
	PeerIP string `toml:"peer_ip" json:"peerIp"`
	// TunnelID identifies this enrolled pair; use a distinct 16-64 byte ID for each pair.
	TunnelID string `toml:"tunnel_id" json:"tunnelId"`
	// Direction is normal (Iran sends Echo Requests) or reverse (Kharej sends them).
	Direction string `toml:"direction" json:"direction"`
	// Paths selects 1-8 ICMP lanes; zero defaults to 4.
	Paths int `toml:"paths" json:"paths"`
	// MaxStreams bounds simultaneous native streams; zero defaults to 128, maximum 256.
	MaxStreams int `toml:"max_streams" json:"maxStreams"`
	// MaxPayload caps native datagrams at 256-1400 bytes; zero is 512 and increases require authenticated PMTU confirmation.
	MaxPayload int `toml:"max_payload" json:"maxPayload"`
	// MaxBps caps paced wire bytes per second; zero defaults to 1000000, maximum 4000000.
	MaxBps int `toml:"max_bps" json:"maxBps"`
}
