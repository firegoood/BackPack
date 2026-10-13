#!/bin/bash
# Disposable privileged Linux container ONLY. 2 CPU quota belongs to the caller.
# Same production forwarding workload, MTU, delay, loss, 8-Mbit/s link and
# 2000-pps per-direction ICMP ceiling. Crypto remains deliberately different:
# native Wat uses AEAD + fresh bilateral keys; legacy xDi uses its unchanged KCP
# AES/CRC wire. This is not an equal-security capacity comparison or WAN result.
set -euo pipefail
test "${WATICMP_NETNS:-}" = 1
test "$(id -u)" = 0
test -f go.mod
out=${BENCH_OUT:-$(mktemp -d)}
mkdir -p "$out"
work=$(mktemp -d)
iran=bi-$$; kharej=bk-$$
pids=()
cleanup() {
 for pid in "${pids[@]}"; do kill "$pid" 2>/dev/null || true; done
 for pid in "${pids[@]}"; do wait "$pid" 2>/dev/null || true; done
 ip netns del "$iran" 2>/dev/null || true; ip netns del "$kharej" 2>/dev/null || true
 rm -rf -- "$work"
}
trap cleanup EXIT
go build -o "$work/fullpack" .
go build -o "$work/fixture" internal/e2e/testdata/waticmpfixture.go
ip netns add "$iran"; ip netns add "$kharej"
ip link add bia-$$ type veth peer name bka-$$
ip link set bia-$$ netns "$iran"; ip link set bka-$$ netns "$kharej"
ip -n "$iran" link set bia-$$ name eth0; ip -n "$kharej" link set bka-$$ name eth0
ip -n "$iran" addr add 10.67.0.1/24 dev eth0; ip -n "$kharej" addr add 10.67.0.2/24 dev eth0
for ns in "$iran" "$kharej"; do ip -n "$ns" link set lo up; ip -n "$ns" link set eth0 up; done
ip netns exec "$kharej" "$work/fixture" -mode backend & pids+=($!)
sleep .1
: > "$out/results.jsonl"
for tr in waticmp xdi; do
 for loss in ${BENCH_LOSSES:-0 1 5 10}; do
  for ns in "$iran" "$kharej"; do
   ip -n "$ns" link set eth0 mtu "${BENCH_MTU:-576}"
   ip netns exec "$ns" tc qdisc replace dev eth0 root netem delay 10ms 2ms loss "$loss%" duplicate .1% reorder 1% 50% rate 8mbit
   ip netns exec "$ns" tc qdisc replace dev eth0 clsact
   ip netns exec "$ns" tc filter add dev eth0 egress protocol ip pref 1 flower ip_proto icmp action police pkts_rate 2000 pkts_burst 200 conform-exceed drop
   for proto in tcp udp; do ip netns exec "$ns" tc filter add dev eth0 egress protocol ip pref 10 flower ip_proto "$proto" action drop; done
  done
  token=disposable-fair-icmp-secret-0123456789
  cat > "$work/iran.toml" <<EOF
[server]
bind_addr = "0.0.0.0:23110"
transport = "$tr"
token = "$token"
ports = ["22080=127.0.0.1:21080"]
accept_udp = true
max_connections = 192
heartbeat = 5
mux_con = 64
kcp_mtu = 512
kcp_sndwnd = 512
kcp_rcvwnd = 512
skip_optz = true
log_level = "error"
EOF
  cat > "$work/kharej.toml" <<EOF
[client]
remote_addr = "10.67.0.1:23110"
transport = "$tr"
token = "$token"
connection_pool = 4
retry_interval = 1
heartbeat = 5
kcp_mtu = 512
kcp_sndwnd = 512
kcp_rcvwnd = 512
skip_optz = true
log_level = "error"
EOF
  if test "$tr" = waticmp; then
   for role in iran kharej; do
    side=server; peer=10.67.0.2
    if test "$role" = kharej; then side=client; peer=10.67.0.1; fi
    cat >> "$work/$role.toml" <<EOF
[$side.waticmp]
enabled = true
peer_ip = "$peer"
tunnel_id = "benchmark-$loss-disposable-pair"
direction = "reverse"
max_payload = 512
max_streams = 256
paths = 4
max_bps = 1000000
EOF
   done
  fi
  chmod 600 "$work/"*.toml
  ip netns exec "$iran" "$work/fullpack" -c "$work/iran.toml" > "$out/$tr-$loss-iran.log" 2>&1 & spid=$!; pids+=("$spid")
  ip netns exec "$kharej" "$work/fullpack" -c "$work/kharej.toml" > "$out/$tr-$loss-kharej.log" 2>&1 & cpid=$!; pids+=("$cpid")
  ready=false
  for attempt in {1..30}; do
   if ip netns exec "$iran" "$work/fixture" -mode client -address 127.0.0.1:22080 -streams 1 -bytes 64 -timeout 5s > "$work/ready.log" 2>&1; then ready=true; break; fi
   sleep .5
  done
  if test "$ready" != true; then cat "$work/ready.log" "$out/$tr-$loss-"*.log; exit 1; fi
  for load in small medium; do
   streams=8; bytes=512
   if test "$load" = medium; then streams=100; bytes=32768; fi
   for repetition in 1 2; do
    label=$tr-$loss-$load-$repetition
    extra=()
    if test "${BENCH_REPORT_FAILURES:-0}" = 1; then extra=(-allow-failures); fi
    python3 internal/e2e/testdata/icmp-resources.py "$out/$label.resources.json" "$spid" "$cpid" & monitor=$!
    ip netns exec "$iran" cat /proc/net/snmp > "$out/$label.before-snmp"
    ip netns exec "$iran" "$work/fixture" -mode client -address 127.0.0.1:22080 -streams "$streams" -bytes "$bytes" -latency 30 -json -timeout 60s "${extra[@]}" > "$out/$label.json"
    touch "$out/$label.resources.json.stop"; wait "$monitor"
    ip netns exec "$iran" cat /proc/net/snmp > "$out/$label.after-snmp"
    python3 - "$out" "$label" "$tr" "$loss" "$load" "${BENCH_MTU:-576}" <<'PY'
import json,sys,pathlib
out,label,tr,loss,load,mtu=sys.argv[1:]; p=pathlib.Path(out)
r=json.load(open(p/(label+'.json'))); r.update(json.load(open(p/(label+'.resources.json'))))
def snmp(path):
 lines=path.read_text().splitlines()
 for i,s in enumerate(lines):
  if s.startswith('Icmp:'):
   return dict(zip(s.split()[1:], map(int,lines[i+1].split()[1:])))
a,b=snmp(p/(label+'.before-snmp')),snmp(p/(label+'.after-snmp'))
r.update(label=label,transport=tr,loss_percent=int(loss),mtu=int(mtu),load=load,icmp_out_packets=b['OutMsgs']-a['OutMsgs'],icmp_in_packets=b['InMsgs']-a['InMsgs'])
r['icmp_out_pps']=r['icmp_out_packets']/r['sample_seconds']
with open(p/'results.jsonl','a') as f: f.write(json.dumps(r)+'\n')
print(json.dumps(r))
PY
   done
  done
  if test "$loss" = 0; then
   label=$tr-overload-1000
   python3 internal/e2e/testdata/icmp-resources.py "$out/$label.resources.json" "$spid" "$cpid" & monitor=$!
   ip netns exec "$iran" "$work/fixture" -mode client -address 127.0.0.1:22080 -streams 1000 -bytes 1024 -json -allow-failures -timeout 30s > "$out/$label.json"
   touch "$out/$label.resources.json.stop"; wait "$monitor"
   cat "$out/$label.json" "$out/$label.resources.json"
   ip netns exec "$iran" "$work/fixture" -mode client -address 127.0.0.1:22080 -streams 1 -timeout 20s
  fi
  kill "$spid" "$cpid"; wait "$spid" || true; wait "$cpid" || true
  pids=("${pids[0]}")
  for ns in "$iran" "$kharej"; do ip netns exec "$ns" tc qdisc del dev eth0 clsact; done
 done
done
printf 'PRODUCTION ICMP BENCHMARK COMPLETE\n'
