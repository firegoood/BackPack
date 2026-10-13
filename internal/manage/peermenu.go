package manage

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/backpack/backpack/config"
	"github.com/backpack/backpack/internal/enginectl"
	"github.com/backpack/backpack/internal/tui"
	"github.com/backpack/backpack/internal/tunnel/peerauto"
)

func askWatDirection(current string) string {
	idx := tui.ChooseOpt("ICMP Packet Direction (Both Ends Must Match)", []tui.Option{{Title: "Normal — Iran sends Echo Requests", Desc: "Kharej sends authenticated Echo Replies"}, {Title: "Reverse — Kharej sends Echo Requests", Desc: "works when Iran-originated ICMP requests are blocked"}})
	if idx == 0 {
		return "normal"
	}
	if idx == 1 {
		return "reverse"
	}
	return current
}
func setupPeerServer(transport string) {
	tui.Info("Experimental reverse TCP/UDP forwarding. Wat requires Linux root/CAP_NET_RAW. Direct GRE/L3 is unsupported.")
	n := NewTunnel{Role: "server", Transport: transport, Name: uniqueName(tui.PromptDefault("Tunnel Name", "server-"+transport)), Token: tui.PromptDefault("Enrollment Secret (At Least 32 Bytes)", randomToken(64)), Ports: tui.Prompt("Forwarded Ports: ")}
	public := PublicIPv4()
	if public == "-" {
		public = ""
	}
	iran := tui.PromptDefault("Iran Explicit IPv4 / Approved Listener Host", public)
	accept := tui.Confirm("Forward UDP As Well As TCP", false)
	n.Tune = &FineTune{AcceptUDP: accept, sent: map[string]bool{"acceptUDP": true}}
	if transport == "waticmp" {
		direction := askWatDirection("")
		if direction == "" {
			return
		}
		n.WatICMP = &config.WatICMPConfig{Enabled: true, PeerIP: tui.Prompt("Kharej Explicit IPv4: "), TunnelID: randomToken(32), Direction: direction, MaxPayload: 512, MaxStreams: 128, Paths: 4}
	} else {
		a := config.AutoConfig{Enabled: true, ObserveOnly: tui.Confirm("Start In Observe-Only Mode (No Automatic Cross-Carrier Switch)", true), TunnelID: randomToken(32), IranID: randomToken(32), KharejID: randomToken(32), Ports: parsePorts(n.Ports), AcceptUDP: accept}
		for len(a.Candidates) < 4 {
			opts := []tui.Option{{Title: "Wat ICMP", Desc: "explicit two peer IPv4 addresses, normal/reverse direction"}, {Title: "TCP + Stealth", Desc: "encrypted Noise; explicitly approved Iran TCP listener"}, {Title: "QUIC", Desc: "encrypted; explicitly approved Iran UDP listener"}, {Title: "WSS", Desc: "TLS-bound peer authentication; explicitly approved Iran TCP listener"}}
			if len(a.Candidates) > 0 {
				opts = append(opts, tui.Option{Title: "Finish Candidate Enrollment"})
			}
			pick := tui.ChooseOpt("Approved Carrier — All Its Inbound Ports Must Be Allowed", opts)
			if pick < 0 {
				return
			}
			if pick == 4 {
				break
			}
			p := config.AutoCandidate{ID: tui.PromptDefault("Candidate ID (1-16 Safe Characters)", fmt.Sprintf("carrier%d", len(a.Candidates)+1))}
			if pick == 0 {
				p.Transport = config.WATICMP
				p.IranIP = iran
				p.KharejIP = tui.Prompt("Kharej Explicit IPv4: ")
				p.Direction = askWatDirection("")
				p.MaxPayload = 512
				if p.Direction == "" {
					return
				}
			} else {
				p.Transport = []config.TransportType{config.STEALTH, config.QUIC, config.WSS}[pick-1]
				port := tui.Prompt("Approved Iran Data Listener Port: ")
				if !validPort(port) {
					tui.Error("Invalid data listener port.")
					return
				}
				p.Endpoint = net.JoinHostPort(iran, port)
				p.LocalBind = net.JoinHostPort("0.0.0.0", port)
			}
			a.Candidates = append(a.Candidates, p)
		}
		n.Auto = &a
	}
	s, e := specFromPeerNew(n)
	if e != nil {
		tui.Error(e.Error())
		tui.PressEnter()
		return
	}
	link := pendingReverseLink(s, iran, linkExtras{})
	if link == "" {
		tui.Error("Enrollment cannot produce a valid peer link; check both addresses and maps.")
		tui.PressEnter()
		return
	}
	summarisePeer(s)
	tui.Warn("This Setup Link Contains The Enrollment Secret — Keep It Private.")
	fmt.Println("sudo fullpack link apply '" + link + "'")
	if tui.Confirm("Create This Explicitly Enrolled Tunnel", true) {
		finishSetup(s)
	}
	tui.PressEnter()
}
func setupPeerClient(transport string) {
	raw := tui.Prompt("Trusted Version-3 Peer Setup Link: ")
	l, e := DecodeShareLink(raw)
	if e != nil {
		tui.Error(e.Error())
		tui.PressEnter()
		return
	}
	if l.Tr != transport || l.Kind != "reverse" || l.From != "iran" {
		tui.Error("This link is not the selected Iran peer enrollment.")
		tui.PressEnter()
		return
	}
	f := MirrorForPeer(l)
	n := f.ToNewTunnel()
	n.Name = uniqueName(tui.PromptDefault("Tunnel Name", f.Name))
	s, e := specFromPeerNew(n)
	if e != nil {
		tui.Error(e.Error())
		tui.PressEnter()
		return
	}
	summarisePeer(s)
	if tui.Confirm("Create This Peer With The Same Approved Manifest", true) {
		finishSetup(s)
	}
	tui.PressEnter()
}
func summarisePeer(s TunnelSpec) {
	tui.Info("Reverse Forwarding — " + selectedTransport(s))
	if s.Auto.Enabled {
		tui.Info("Authority: Iran " + s.Auto.IranID + "; foreign peer: " + s.Auto.KharejID)
		for _, p := range s.Auto.Candidates {
			if p.Transport == config.WATICMP {
				tui.Info(p.ID + ": Wat " + p.Direction + " ICMP, Iran=" + p.IranIP + ", Kharej=" + p.KharejIP)
			} else {
				tui.Info(p.ID + ": " + string(p.Transport) + " data endpoint " + p.Endpoint)
			}
		}
		tui.Info("Observe-only: " + onOff(s.Auto.ObserveOnly) + "; probes, commits and recovery use these approved carriers only.")
	} else {
		tui.Info("Wat peer: " + s.WatICMP.PeerIP + "; direction: " + s.WatICMP.Direction + "; no TCP/UDP tunnel port.")
	}
	if s.Role == "server" {
		tui.Info("User ports: " + strings.Join(s.Ports, ", "))
	}
}

func editPeerMenu(name string, s TunnelSpec) {
	if s.Auto.TunnelID != "" && !s.Auto.Enabled {
		tui.Info("Auto is disabled. Retain the enrolled state; restore the matching Auto config on both peers or create a separate manual tunnel.")
		tui.PressEnter()
		return
	}
	summarisePeer(s)
	if !s.Auto.Enabled {
		if tui.Confirm("Edit Native Peer / Direction (Update Both Ends)", false) {
			s.WatICMP.PeerIP = tui.PromptDefault("Peer IPv4", s.WatICMP.PeerIP)
			s.WatICMP.Direction = askWatDirection(s.WatICMP.Direction)
			if s.Role == "client" {
				s.RemoteAddr = net.JoinHostPort(s.WatICMP.PeerIP, "0")
			}
			report(applySpec(s), "native settings updated")
		}
		return
	}
	var status peerauto.Status
	if e := enginectl.Ask(name, enginectl.OpAutoStatus, &status); e != nil {
		tui.Error(e.Error())
		tui.PressEnter()
		return
	}
	b, _ := json.MarshalIndent(status, "", "  ")
	fmt.Println(string(b))
	choices := []tui.Option{{Title: "Run Protocol Tests Now"}, {Title: "Manual Carrier Override"}, {Title: "Lock To A Carrier"}, {Title: "Disable A Standby Carrier Until Restart"}, {Title: "Remove Runtime Lock"}, {Title: "Observe Only"}, {Title: "Resume Auto Decisions"}}
	pick := tui.ChooseOpt("Auto Operations", choices)
	if pick < 0 {
		return
	}
	op := []string{"test-now", "switch", "lock", "disable-carrier", "unlock", "observe", "resume"}[pick]
	a := peerauto.Action{Operation: op}
	if pick < 4 {
		a.Candidate = tui.Prompt("Enrolled Candidate ID: ")
	}
	report(enginectl.AskBody(name, enginectl.OpAutoAction, a, nil), "Auto operation completed")
}
