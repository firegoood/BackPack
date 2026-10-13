#!/bin/bash
# Disposable privileged Linux container ONLY. Both endpoints are data carriers;
# all operations below use the existing private Unix control socket.
set -euo pipefail
test "${WATICMP_NETNS:-}" = 1
test "$(id -u)" = 0
work=$(mktemp -d);suffix=$$
iran=fpa-iran-$suffix;kharej=fpa-kharej-$suffix
spid=;cpid=;bpid=
cleanup(){
	result=$?;if test "$result" != 0;then cat "$work/iran.log" "$work/kharej.log" 2>/dev/null||true;fi
 for pid in "$spid" "$cpid" "$bpid";do if test -n "$pid";then kill "$pid" 2>/dev/null||true;wait "$pid" 2>/dev/null||true;fi;done
 ip netns del "$iran" 2>/dev/null||true;ip netns del "$kharej" 2>/dev/null||true
 rm -rf -- "$work"
}
trap cleanup EXIT
go build -o "$work/fullpack" .
go build -o "$work/fixture" internal/e2e/testdata/waticmpfixture.go
ip netns add "$iran";ip netns add "$kharej"
ip link add fpa-i-$suffix type veth peer name fpa-k-$suffix
ip link set fpa-i-$suffix netns "$iran";ip link set fpa-k-$suffix netns "$kharej"
ip -n "$iran" link set fpa-i-$suffix name eth0;ip -n "$kharej" link set fpa-k-$suffix name eth0
ip -n "$iran" addr add 10.69.0.1/24 dev eth0;ip -n "$kharej" addr add 10.69.0.2/24 dev eth0
for ns in "$iran" "$kharej";do
 ip -n "$ns" link set lo up;ip -n "$ns" link set eth0 up;ip -n "$ns" link set eth0 mtu 700
 ip netns exec "$ns" tc qdisc replace dev eth0 root netem delay 10ms 2ms loss 0.5% duplicate 0.1% reorder 1% 50%
 ip netns exec "$ns" tc qdisc replace dev eth0 clsact
 ip netns exec "$ns" tc filter add dev eth0 egress protocol ip pref 10 flower ip_proto tcp action drop
 ip netns exec "$ns" tc filter add dev eth0 egress protocol ip pref 12 flower ip_proto udp action drop
done
# This stays installed throughout: Iran cannot initiate ICMP requests.
ip netns exec "$iran" tc filter add dev eth0 egress protocol ip pref 11 u32 match ip protocol 1 0xff match u8 8 0xff at 20 action drop
ip netns exec "$kharej" "$work/fixture" -mode backend & bpid=$!
cat > "$work/iran.toml" <<EOF
[server]
bind_addr = "0.0.0.0:0"
transport = "waticmp"
token = "disposable-auto-enrollment-secret-0123456789"
ports = ["22080=127.0.0.1:21080"]
accept_udp = true
skip_optz = true
EOF
cat > "$work/kharej.toml" <<EOF
[client]
remote_addr = "10.69.0.1:0"
transport = "waticmp"
token = "disposable-auto-enrollment-secret-0123456789"
skip_optz = true
EOF
for role in iran kharej;do
 cat >> "$work/$role.toml" <<EOF
[auto]
enabled = true
tunnel_id = "disposable-auto-$suffix"
iran_id = "disposable-enrolled-iran"
kharej_id = "disposable-enrolled-kharej"
ports = ["22080=127.0.0.1:21080"]
accept_udp = true
max_connections = 128
max_memory_mb = 384
cooldown = 5
minimum_dwell = 5
max_switches_per_hour = 12
allow_degradation_switch = ${DEGRADATION_ENABLED:-false}
probe_interval = 30
[[auto.candidates]]
id = "wat"
transport = "waticmp"
iran_ip = "10.69.0.1"
kharej_ip = "10.69.0.2"
direction = "reverse"
max_payload = 1400
[[auto.candidates]]
id = "noise"
transport = "stealth"
endpoint = "10.69.0.1:24443"
EOF
 chmod 600 "$work/$role.toml"
done
ip netns exec "$iran" "$work/fullpack" -c "$work/iran.toml" > "$work/iran.log" 2>&1 & spid=$!
start_peer(){ ip netns exec "$kharej" "$work/fullpack" -c "$work/kharej.toml" >> "$work/kharej.log" 2>&1 & cpid=$!; }
start_peer
status(){ "$work/fullpack" auto status "$1" --json; }
field(){ status "$1"|python3 -c "import json,sys;print(json.load(sys.stdin).get('$2',''))"; }
wait_route(){
 local wanted=$1;local start=$SECONDS
 for attempt in $(seq 1 "${PEER_WAIT_ATTEMPTS:-90}");do
  if status iran > "$work/status.json" 2>/dev/null && test "$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["active"])' "$work/status.json")" = "$wanted";then
   if test -z "$wanted"||test "$(field kharej active)" = "$wanted";then
    if test -n "$wanted";then test "$(field iran generation)" = "$(field kharej generation)";fi
    printf 'AGREED active=%s after=%ss generation=%s\n' "$wanted" "$((SECONDS-start))" "$(field iran generation)"
    return
   fi
  fi
  sleep 1
 done
 cat "$work/iran.log" "$work/kharej.log";status iran||true;status kharej||true;exit 1
}
traffic(){ ip netns exec "$iran" "$work/fixture" -mode client -address 127.0.0.1:22080 -streams 100; }
block_icmp(){ for ns in "$iran" "$kharej";do ip netns exec "$ns" tc filter add dev eth0 egress protocol ip pref 5 flower ip_proto icmp action drop;done; }
restore_icmp(){ for ns in "$iran" "$kharej";do ip netns exec "$ns" tc filter del dev eth0 egress protocol ip pref 5;done; }
restore_tcp(){ for ns in "$iran" "$kharej";do ip netns exec "$ns" tc filter del dev eth0 egress protocol ip pref 10;done; }
block_tcp(){ for ns in "$iran" "$kharej";do ip netns exec "$ns" tc filter add dev eth0 egress protocol ip pref 10 flower ip_proto tcp action drop;done; }
wait_route wat
printf 'BOOTSTRAP only reverse ICMP, all direct TCP/UDP blocked\n';traffic
if test "${WATICMP_DEGRADATION:-0}" = 1;then
 restore_tcp
 for n in {1..30};do
  if status iran | python3 -c 'import json,sys;raise SystemExit(not any(c["id"]=="noise" and c["ready"] for c in json.load(sys.stdin)["candidates"]))';then break;fi
  sleep 1
 done
 # Establish three real alternative measurements, then degrade only ICMP.
 # A 600-kbit/s bottleneck also saturates queues/loses control under the
 # reference engine's packet floor; retain that separate diagnostic case.
 # The default acceptance case keeps authenticated data/control reachable
 # while interactive RTT and useful round-trip throughput become severe.
 for n in 1 2 3;do "$work/fullpack" auto test-now iran noise;done
 for ns in "$iran" "$kharej";do
  ip netns exec "$ns" tc qdisc replace dev eth0 root handle 1: htb default 2
  rate=2mbit;delay=180ms
  if test "${DEGRADATION_SATURATED:-0}" = 1;then rate=600kbit;delay=10ms;fi
  ip netns exec "$ns" tc class add dev eth0 parent 1: classid 1:1 htb rate "$rate" ceil "$rate"
  ip netns exec "$ns" tc class add dev eth0 parent 1: classid 1:2 htb rate 10mbit ceil 10mbit
  ip netns exec "$ns" tc qdisc add dev eth0 parent 1:1 handle 10: netem delay "$delay" 2ms
  ip netns exec "$ns" tc filter add dev eth0 parent 1: protocol ip pref 1 flower ip_proto icmp flowid 1:1
 done
 for n in 1 2 3;do
  if ! "$work/fullpack" auto test-now iran wat;then
   status iran;status kharej
   for ns in "$iran" "$kharej";do ip netns exec "$ns" tc -s qdisc show dev eth0;done
   ip netns exec "$iran" "$work/fixture" -mode client -address 127.0.0.1:22080 -streams 1 -bytes 32768 -json -allow-failures -timeout 20s
   exit 1
  fi
 done
 # No manual selection or broken control channel: the opt-in policy must
 # confirm sustained degradation and choose the better enrolled alternative.
 PEER_WAIT_ATTEMPTS=240 wait_route noise
 status iran > "$work/degraded.json"
 python3 - "$work/degraded.json" <<'PY'
import json,sys
s=json.load(open(sys.argv[1])); print(s)
assert s['active']=='noise' and any('severe degradation' in e['reason'] and e['result']=='COMMITTED' for e in s['history']), 'switched for a different trigger'
PY
 traffic
 printf 'PASS sustained reachable ICMP degradation selected Noise through both-peer commit\n'
 exit 0
fi
generation=$(field iran generation)
block_icmp;sleep 2;restore_icmp;sleep 5
test "$(field iran active)" = wat;test "$(field iran generation)" = "$generation"
printf 'PASS transient 2s blackhole did not change generation\n'
"$work/fullpack" auto observe iran
block_icmp;restore_tcp
wait_route ''
sleep 8
test "$(field iran active)" = ''
printf 'PASS observe-only refused a cross-carrier switch while Noise was reachable\n'
"$work/fullpack" auto resume iran
printf 'OUTAGE complete native data+control blackhole; enrolled Noise data port restored\n'
wait_route noise;traffic
restore_icmp;sleep 12
test "$(field iran active)" = noise
printf 'PASS returning faster/previous candidate alone did not force failback\n'
block_tcp
printf 'OUTAGE Noise data+control blackhole; only reverse ICMP remains\n'
wait_route wat;traffic
block_icmp
wait_route ''
if ip netns exec "$iran" "$work/fixture" -mode client -address 127.0.0.1:22080 -streams 1 -timeout 3s > "$work/no-route.log" 2>&1;then printf 'ERROR no-route admitted users\n';exit 1;fi
printf 'PASS NO_ROUTE rejects new users without rebinding their ports\n'
restore_icmp;wait_route wat;traffic
kill "$cpid";wait "$cpid"||true;cpid=
sleep 3;start_peer
wait_route wat;traffic
printf 'PASS peer restart converged with fresh incarnation and durable generation\n'
status iran;status kharej
cat "$work/iran.log" "$work/kharej.log"
printf 'PEER AUTO TWO-NODE FAILURE/RECOVERY MATRIX COMPLETE\n'
