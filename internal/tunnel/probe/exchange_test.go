package probe

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestPacedBulkMeasuresASlowPassingPathAndJoinsCancellation(t *testing.T) {
	local, peer := net.Pipe()
	defer local.Close()
	defer peer.Close()
	done := make(chan error, 1)
	go func() { time.Sleep(4250 * time.Millisecond); _, err := RespondPaced(peer, 32768); done <- err }()
	if _, _, err := ExchangePaced(t.Context(), local, 32768); err != nil {
		t.Fatal("slow passing path treated as failed:", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	local2, peer2 := net.Pipe()
	defer peer2.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, _, err := ExchangePaced(ctx, local2, 32768); err == nil {
		t.Fatal("unanswered probe passed")
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancel did not join the blocked writer")
	}
}
