#!/usr/bin/env bash
set +e

vm_dir="${RUNNER_TEMP}/casos-worker-vm"
run_hash="$(printf '%s' "${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}" | sha256sum | awk '{print $1}')"
run_suffix="${run_hash:0:6}"
subnet_octet=$((16#${run_hash:0:2} % 50 + 200))
bridge_name="cbr${run_suffix}"
tap_name="ctap${run_suffix}"
worker_cidr="192.168.${subnet_octet}.0/24"
default_iface="$(ip -4 route list default | awk 'NR == 1 {print $5}')"

if [[ -f "$vm_dir/qemu.pid" ]]; then
  qemu_pid="$(cat "$vm_dir/qemu.pid")"
  kill "$qemu_pid" 2>/dev/null || true
  for _ in {1..10}; do
    kill -0 "$qemu_pid" 2>/dev/null || break
    sleep 1
  done
  kill -9 "$qemu_pid" 2>/dev/null || true
fi

sudo iptables -t nat -D POSTROUTING -s "$worker_cidr" -o "$default_iface" -j MASQUERADE 2>/dev/null || true
sudo iptables -D FORWARD -i "$bridge_name" -o "$default_iface" -j ACCEPT 2>/dev/null || true
sudo iptables -D FORWARD -i "$default_iface" -o "$bridge_name" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true
sudo ip link delete "$tap_name" 2>/dev/null || true
sudo ip link delete "$bridge_name" 2>/dev/null || true
