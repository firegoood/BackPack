package cmd

import (
	"strings"
	"testing"

	"github.com/backpack/backpack/config"
)

func TestNativeWatRejectsBroadcastAndIgnoredSourceSettings(t *testing.T) {
	base := func() *config.Config {
		return &config.Config{Client: config.ClientConfig{Transport: config.WATICMP, Token: strings.Repeat("k", 32), WatICMP: config.WatICMPConfig{Enabled: true, PeerIP: "192.0.2.1", TunnelID: "test-native-safe-identity", Direction: "reverse"}}}
	}
	if err := checkWatICMP(base()); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*config.Config){
		func(c *config.Config) { c.Client.WatICMP.PeerIP = "255.255.255.255" },
		func(c *config.Config) { c.Client.Interface = "eth0" },
		func(c *config.Config) { c.Client.LocalAddr = "192.0.2.2" },
		func(c *config.Config) { c.Client.MSS = 1200 },
		func(c *config.Config) { c.Client.Proxy = "socks5://127.0.0.1:1080" },
	} {
		c := base()
		mutate(c)
		if checkWatICMP(c) == nil {
			t.Fatal("unsafe/ignored native setting accepted")
		}
	}
}
