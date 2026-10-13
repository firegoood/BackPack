#!/bin/bash
# Disposable privileged container only. Never run this on production hosts.
# Usage: WATICMP_NETNS=1 bash internal/e2e/testdata/waticmp-netns.sh
set -euo pipefail
test "${WATICMP_NETNS:-}" = 1
test "$(id -u)" = 0
test -f go.mod
work=$(mktemp -d)
suffix=$$
iran=fpiran-$suffix
kharej=fpkharej-$suffix
pids=()
cleanup() {
  for pid in "${pids[@]}"; do kill "$pid" 2>/dev/null || true; done
  for pid in "${pids[@]}"; do wait "$pid" 2>/dev/null || true; done
  ip netns del "$iran" 2>/dev/null || true
  ip netns del "$kharej" 2>/dev/null || true
  # The created mktemp directory is the only removed path.
  rm -rf -- "$work"
}
trap cleanup EXIT
go build -o "$work/fullpack" .
go build -o "$work/fixture" internal/e2e/testdata/waticmpfixture.go
ip netns add "$iran"; ip netns add "$kharej"
ip link add fpia-$suffix type veth peer name fpka-$suffix
ip link set fpia-$suffix netns "$iran"; ip link set fpka-$suffix netns "$kharej"
ip -n "$iran" link set fpia-$suffix name eth0
ip -n "$kharej" link set fpka-$suffix name eth0
ip -n "$iran" addr add 10.68.0.1/24 dev eth0
ip -n "$kharej" addr add 10.68.0.2/24 dev eth0
for ns in "$iran" "$kharej"; do ip -n "$ns" link set lo up; ip -n "$ns" link set eth0 up; done
ip netns exec "$kharej" "$work/fixture" -mode backend & pids+=($!)
sleep .1
for direction in normal reverse; do
 for mtu in 576 640 700; do
  ip -n "$iran" link set eth0 mtu "$mtu"; ip -n "$kharej" link set eth0 mtu "$mtu"
  for ns in "$iran" "$kharej"; do
   ip netns exec "$ns" tc qdisc replace dev eth0 root netem delay 10ms 2ms loss 0.5% duplicate 0.1% reorder 1% 50%
  done
  # Block every cross-node TCP/UDP packet. In reverse mode also block all
  # Iran-originated Echo Requests: the only permitted initiation is Kharej.
  ip netns exec "$iran" tc qdisc replace dev eth0 clsact
  ip netns exec "$kharej" tc qdisc replace dev eth0 clsact
  for ns in "$iran" "$kharej"; do
   for proto in tcp udp; do
    ip netns exec "$ns" tc filter add dev eth0 egress protocol ip pref 10 flower ip_proto "$proto" action drop
   done
  done
  if test "$direction" = reverse; then
   ip netns exec "$iran" tc filter add dev eth0 egress protocol ip pref 11 u32 match ip protocol 1 0xff match u8 8 0xff at 20 action drop
  fi
  token=disposable-native-network-secret-0123456789
  id=native-netns-$direction-$mtu-$suffix
  cat > "$work/iran.toml" <<EOF
[server]
bind_addr = "0.0.0.0:0"
transport = "waticmp"
token = "$token"
ports = ["22080=127.0.0.1:21080"]
accept_udp = true
heartbeat = 5
skip_optz = true
log_level = "info"
[server.waticmp]
enabled = true
peer_ip = "10.68.0.2"
tunnel_id = "$id"
direction = "$direction"
max_payload = 1400
max_streams = 256
EOF
  cat > "$work/kharej.toml" <<EOF
[client]
remote_addr = "10.68.0.1:0"
transport = "waticmp"
token = "$token"
connection_pool = 4
retry_interval = 1
skip_optz = true
log_level = "info"
[client.waticmp]
enabled = true
peer_ip = "10.68.0.1"
tunnel_id = "$id"
direction = "$direction"
max_payload = 1400
max_streams = 256
EOF
  if test "${WATICMP_PEER_TEST:-0}" = 1; then
   for role in iran kharej; do
    cat >> "$work/$role.toml" <<EOF
[auto]
enabled = true
tunnel_id = "$id"
iran_id = "disposable-enrolled-iran"
kharej_id = "disposable-enrolled-kharej"
ports = ["22080=127.0.0.1:21080"]
accept_udp = true
max_connections = 128
[[auto.candidates]]
id = "wat"
transport = "waticmp"
iran_ip = "10.68.0.1"
kharej_ip = "10.68.0.2"
direction = "$direction"
max_payload = 1400
EOF
   done
  fi
  chmod 600 "$work/iran.toml" "$work/kharej.toml"
  ip netns exec "$iran" "$work/fullpack" -c "$work/iran.toml" > "$work/iran.log" 2>&1 & spid=$!; pids+=("$spid")
  ip netns exec "$kharej" "$work/fullpack" -c "$work/kharej.toml" > "$work/kharej.log" 2>&1 & cpid=$!; pids+=("$cpid")
  ready=false
  for attempt in {1..20}; do
   if ip netns exec "$iran" "$work/fixture" -mode client -address 127.0.0.1:22080 -streams 1 > "$work/ready.log" 2>&1; then ready=true; break; fi
   sleep .5
  done
  if test "$ready" != true; then cat "$work/ready.log" "$work/iran.log" "$work/kharej.log"; exit 1; fi
  printf 'direction=%s mtu=%s cross-node-TCP/UDP=blocked Iran-EchoRequest=%s\n' "$direction" "$mtu" "$direction"
  if ! ip netns exec "$iran" "$work/fixture" -mode client -address 127.0.0.1:22080 -streams 100; then cat "$work/iran.log" "$work/kharej.log"; exit 1; fi
  kill "$spid" "$cpid"; wait "$spid" || true; wait "$cpid" || true
  pids=("${pids[0]}")
  # Each case is a different disposable enrollment, never a production reset.
  if test "${WATICMP_PEER_TEST:-0}" = 1; then
   rm -f -- "$work/iran.toml.auto-state.json" "$work/kharej.toml.auto-state.json" "$work/iran.toml.auto-state.json.lock" "$work/kharej.toml.auto-state.json.lock"
  fi
  for ns in "$iran" "$kharej"; do ip netns exec "$ns" tc qdisc del dev eth0 clsact; done
 done
done
printf 'NATIVE TWO-NODE MATRIX COMPLETE\n'
