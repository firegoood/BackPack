package waticmp

import "github.com/backpack/backpack/config"

func SettingsOf(c config.WatICMPConfig, token string) Settings {
	return Settings{Enabled: c.Enabled, PeerIP: c.PeerIP, TunnelID: c.TunnelID, Direction: c.Direction,
		Paths: c.Paths, MaxStreams: c.MaxStreams, MaxPayload: c.MaxPayload, MaxBps: c.MaxBps, Token: token}
}
