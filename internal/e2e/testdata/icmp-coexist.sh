#!/bin/bash
# Disposable privileged Linux container only; no production firewall changes.
set -euo pipefail
test "${WATICMP_NETNS:-}" = 1
work=$(mktemp -d); iran=ci-$$; kharej=ck-$$; pids=()
cleanup(){ for p in "${pids[@]}"; do kill "$p" 2>/dev/null||true; done; for p in "${pids[@]}"; do wait "$p" 2>/dev/null||true; done; ip netns del "$iran" 2>/dev/null||true; ip netns del "$kharej" 2>/dev/null||true; rm -rf -- "$work"; }
trap cleanup EXIT
go build -o "$work/fullpack" .
go build -o "$work/fixture" internal/e2e/testdata/waticmpfixture.go
ip netns add "$iran"; ip netns add "$kharej"
ip link add cia-$$ type veth peer name cka-$$
ip link set cia-$$ netns "$iran"; ip link set cka-$$ netns "$kharej"
ip -n "$iran" link set cia-$$ name eth0; ip -n "$kharej" link set cka-$$ name eth0
ip -n "$iran" addr add 10.66.0.1/24 dev eth0; ip -n "$kharej" addr add 10.66.0.2/24 dev eth0
ip -n "$kharej" addr add 10.66.0.3/24 dev eth0
for ns in "$iran" "$kharej"; do
 ip -n "$ns" link set lo up; ip -n "$ns" link set eth0 up
 ip netns exec "$ns" tc qdisc replace dev eth0 root netem delay 10ms 2ms loss 1% duplicate .1% reorder 1% 50%
 ip netns exec "$ns" tc qdisc replace dev eth0 clsact
 for proto in tcp udp; do ip netns exec "$ns" tc filter add dev eth0 egress protocol ip pref 10 flower ip_proto "$proto" action drop; done
done
ip netns exec "$kharej" "$work/fixture" -mode backend & pids+=($!)
sleep .1
for number in 1 2 3; do
 tr=waticmp; direction=normal; if test "$number" = 2; then direction=reverse; fi; if test "$number" = 3; then tr=xdi; fi
 for role in iran kharej; do
  side=server; address='bind_addr = "0.0.0.0:23110"'; peer=10.66.0.2
  if test "$role" = kharej; then side=client; address='remote_addr = "10.66.0.1:23110"'; peer=10.66.0.1; fi
  file="$work/$role$number.toml"
  cat > "$file" <<EOF
[$side]
$address
transport = "$tr"
token = "coexistent-secret-$number-0123456789abcdef"
heartbeat = 5
retry_interval = 1
skip_optz = true
log_level = "error"
kcp_mtu = 512
EOF
  if test "$role" = iran; then echo "ports = [\"$((22080+number))=127.0.0.1:21080\"]" >> "$file"; echo 'accept_udp = true' >> "$file"; fi
  if test "$tr" = waticmp; then cat >> "$file" <<EOF
[$side.waticmp]
enabled = true
peer_ip = "$peer"
tunnel_id = "coexistent-native-tunnel-$number"
direction = "$direction"
max_payload = 1400
max_streams = 256
EOF
  fi
  chmod 600 "$file"
  ip netns exec "${!role}" "$work/fullpack" -c "$file" > "$work/$role$number.log" 2>&1 & pids+=($!)
 done
 ready=false
 for n in {1..20}; do
  if ip netns exec "$iran" "$work/fixture" -mode client -address "127.0.0.1:$((22080+number))" -streams 1 -timeout 5s > "$work/ready.log" 2>&1; then ready=true; break; fi
  sleep .5
 done
 if test "$ready" != true; then cat "$work/ready.log" "$work/"*.log; exit 1; fi
done
ip netns exec "$iran" ping -c 4 -W 2 10.66.0.2
ip netns exec "$kharej" ping -c 4 -W 2 10.66.0.1
ip netns exec "$iran" python3 internal/e2e/testdata/icmp-negative.py capture "$work/captured.json" & capture=$!
ip netns exec "$iran" "$work/fixture" -mode client -address 127.0.0.1:22082 -streams 20 -timeout 20s
wait "$capture"
# A new wrong-key controller cannot join either already seated native identity.
sed 's/coexistent-secret-2-0123456789abcdef/wrong-key-cannot-authenticate-0123456789/' "$work/kharej2.toml" > "$work/wrong.toml"
ip netns exec "$kharej" "$work/fullpack" -c "$work/wrong.toml" > "$work/wrong.log" 2>&1 & wrong=$!; pids+=("$wrong")
ip netns exec "$kharej" python3 internal/e2e/testdata/icmp-negative.py flood "$work/captured.json"
python3 - "$work" <<'PY'
import json,socket,sys
def status(name):
 with socket.socket(socket.AF_UNIX) as s:
  s.settimeout(2); s.connect('/run/fullpack/'+name+'.sock'); s.sendall(b'{"op":"status"}\n'); return json.loads(s.makefile('rb').readline())['body']
assert not status('wrong')['connected'], 'wrong-key controller authenticated'
for name in ('iran1','iran2'):
 r=status(name); assert r['connected']; d=r['diagnostics']['transport_detail']; assert d['authenticated_payload']==1400,d
 print(name,json.dumps(d))
 if name=='iran2':
  assert d['engine']['ReplayDropped']>0 and d['engine']['AuthDropped']>0, 'negative packets never reached authenticated native replay/AEAD checks'
PY
kill "$wrong"; wait "$wrong"||true; unset 'pids[-1]'
# Independent one-way loss and corrupted packets, then verify all three users.
ip netns exec "$iran" tc qdisc replace dev eth0 root netem delay 10ms 2ms loss 5% corrupt .1% duplicate .1% reorder 1% 50%
for number in 1 2 3; do
 ip netns exec "$iran" "$work/fixture" -mode client -address "127.0.0.1:$((22080+number))" -streams 20 -timeout 30s
done
printf 'PASS ping + xDi + two native identities, wrong key/source, captured replay/corruption flood and asymmetric loss\n'
