// Standalone disposable-network fixture; testdata is excluded from release builds.
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"sync"
	"time"
)

func main() {
	mode := flag.String("mode", "backend", "backend or client")
	addr := flag.String("address", "127.0.0.1:21080", "test-only loopback endpoint")
	streams := flag.Int("streams", 100, "actual concurrent TCP connections")
	timeout := flag.Duration("timeout", 45*time.Second, "test transaction deadline")
	payload := flag.Int("bytes", 32768, "bytes per TCP connection, 1..1048576")
	latency := flag.Int("latency", 0, "64-byte interactive RTT samples before bulk")
	jsonOut := flag.Bool("json", false, "machine-readable measurements")
	allowFailures := flag.Bool("allow-failures", false, "report overload failures without a false all-transfers PASS")
	flag.Parse()
	if *mode == "backend" {
		l, err := net.Listen("tcp", *addr)
		must(err)
		pc, err := net.ListenPacket("udp", *addr)
		must(err)
		go func() {
			buf := make([]byte, 65536)
			for {
				n, a, e := pc.ReadFrom(buf)
				if e != nil {
					return
				}
				_, _ = pc.WriteTo(buf[:n], a)
			}
		}()
		for {
			c, e := l.Accept()
			must(e)
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}
	if *mode != "client" || *streams < 1 || *streams > 1000 || *payload < 1 || *payload > 1048576 || *latency < 0 || *latency > 1000 {
		panic("invalid test invocation")
	}
	rtts := make([]float64, 0, *latency)
	if *latency > 0 {
		c, err := net.DialTimeout("tcp", *addr, 5*time.Second)
		must(err)
		p := make([]byte, 64)
		_, err = rand.Read(p)
		must(err)
		for n := 0; n < *latency; n++ {
			_ = c.SetDeadline(time.Now().Add(*timeout))
			start := time.Now()
			_, err = c.Write(p)
			must(err)
			got := make([]byte, len(p))
			_, err = io.ReadFull(c, got)
			must(err)
			if !bytes.Equal(p, got) {
				panic("interactive integrity mismatch")
			}
			rtts = append(rtts, float64(time.Since(start))/float64(time.Millisecond))
		}
		must(c.Close())
	}
	start := time.Now()
	var wg sync.WaitGroup
	errs := make(chan error, *streams)
	for n := 0; n < *streams; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, e := net.DialTimeout("tcp", *addr, 5*time.Second)
			if e != nil {
				errs <- e
				return
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(*timeout))
			p := make([]byte, *payload)
			_, e = rand.Read(p)
			if e == nil {
				_, e = c.Write(p)
			}
			got := make([]byte, len(p))
			if e == nil {
				_, e = io.ReadFull(c, got)
			}
			if e == nil && !bytes.Equal(got, p) {
				e = fmt.Errorf("TCP integrity mismatch")
			}
			if e != nil {
				errs <- e
			}
		}()
	}
	wg.Wait()
	close(errs)
	bulkElapsed := time.Since(start)
	failures := 0
	firstError := ""
	for err := range errs {
		failures++
		if firstError == "" {
			firstError = err.Error()
		}
	}
	if failures > 0 && !*allowFailures {
		must(fmt.Errorf("%d/%d TCP transactions failed: %s", failures, *streams, firstError))
	}
	c, err := net.Dial("udp", *addr)
	must(err)
	defer c.Close()
	for _, size := range []int{1, 64, 1400, 8000} {
		_ = c.SetDeadline(time.Now().Add(min(*timeout, 15*time.Second)))
		p := make([]byte, size)
		_, err = rand.Read(p)
		must(err)
		_, err = c.Write(p)
		must(err)
		got := make([]byte, size+1)
		n, err := c.Read(got)
		must(err)
		if n != size || !bytes.Equal(p, got[:n]) {
			panic("UDP boundary/integrity mismatch")
		}
	}
	if *jsonOut {
		p50, p95, jitter := 0.0, 0.0, 0.0
		for i := 1; i < len(rtts); i++ {
			d := rtts[i] - rtts[i-1]
			if d < 0 {
				d = -d
			}
			jitter += d
		}
		if len(rtts) > 1 {
			jitter /= float64(len(rtts) - 1)
		}
		sort.Float64s(rtts)
		if len(rtts) > 0 {
			p50 = rtts[len(rtts)/2]
			p95 = rtts[min(len(rtts)-1, len(rtts)*95/100)]
		}
		must(json.NewEncoder(os.Stdout).Encode(map[string]any{"connections_attempted": *streams, "connections_passed": *streams - failures, "failed": failures,
			"first_error": firstError, "bytes_each_direction": (*streams - failures) * *payload, "bulk_seconds": bulkElapsed.Seconds(),
			"useful_mbps_each_direction": float64((*streams-failures)**payload) * 8 / bulkElapsed.Seconds() / 1e6, "rtt_samples": len(rtts),
			"rtt_p50_ms": p50, "rtt_p95_ms": p95, "jitter_mean_abs_successive_ms": jitter, "udp_verified_sizes": []int{1, 64, 1400, 8000}}))
	} else {
		fmt.Printf("PASS connections=%d TCP_verified_each_direction=%d UDP_sizes=1,64,1400,8000 elapsed=%s\n", *streams-failures, (*streams-failures)**payload, time.Since(start))
	}
}
func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
