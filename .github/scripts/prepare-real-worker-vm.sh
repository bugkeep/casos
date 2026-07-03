#!/usr/bin/env bash
set -euo pipefail

: "${E2E_APISERVER_PORT:=16443}"
: "${GITHUB_RUN_ID:?GITHUB_RUN_ID must be set}"
: "${GITHUB_RUN_ATTEMPT:?GITHUB_RUN_ATTEMPT must be set}"
: "${RUNNER_TEMP:?RUNNER_TEMP must be set}"

sudo apt-get update
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
  ca-certificates \
  cloud-image-utils \
  curl \
  iproute2 \
  iptables \
  openssh-client \
  qemu-system-x86 \
  qemu-utils

sudo modprobe kvm || true
if [[ ! -e /dev/kvm ]]; then
  echo "KVM device is unavailable on this runner; cannot start a real worker VM." >&2
  exit 1
fi
sudo chown "${USER}:$(id -gn)" /dev/kvm
sudo chmod 660 /dev/kvm

vm_dir="${RUNNER_TEMP}/casos-worker-vm"
mkdir -p "$vm_dir"
real_worker_data_dir="${RUNNER_TEMP}/casos-real-worker-e2e-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}"
rm -rf "$real_worker_data_dir"
mkdir -p "$real_worker_data_dir"

key_path="${RUNNER_TEMP}/casos-worker-key"
rm -f "$key_path" "$key_path.pub"
ssh-keygen -t ed25519 -N "" -f "$key_path"

run_hash="$(printf '%s' "${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}" | sha256sum | awk '{print $1}')"
run_suffix="${run_hash:0:6}"
subnet_octet=$((16#${run_hash:0:2} % 50 + 200))
bridge_name="cbr${run_suffix}"
tap_name="ctap${run_suffix}"
host_bridge_ip="192.168.${subnet_octet}.1"
worker_vm_ip="192.168.${subnet_octet}.2"
worker_cidr="192.168.${subnet_octet}.0/24"
worker_mac="52:54:00:${run_hash:0:2}:${run_hash:2:2}:${run_hash:4:2}"
default_iface="$(ip -4 route list default | awk 'NR == 1 {print $5}')"

sudo ip link add "$bridge_name" type bridge
sudo ip addr add "${host_bridge_ip}/24" dev "$bridge_name"
sudo ip link set dev "$bridge_name" type bridge stp_state 0 forward_delay 0
sudo ip link set "$bridge_name" up
sudo ip tuntap add dev "$tap_name" mode tap user "$USER"
sudo ip link set "$tap_name" master "$bridge_name"
sudo ip link set "$tap_name" up
sudo sysctl -w net.ipv4.ip_forward=1
sudo iptables -t nat -A POSTROUTING -s "$worker_cidr" -o "$default_iface" -j MASQUERADE
sudo iptables -A FORWARD -i "$bridge_name" -o "$default_iface" -j ACCEPT
sudo iptables -A FORWARD -i "$default_iface" -o "$bridge_name" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT

base_image="$vm_dir/ubuntu-jammy-server-cloudimg-amd64.img"
worker_disk="$vm_dir/worker.qcow2"
seed_image="$vm_dir/seed.img"
user_data="$vm_dir/user-data"
meta_data="$vm_dir/meta-data"
network_config="$vm_dir/network-config"

curl -fsSL --retry 3 --retry-delay 5 --retry-connrefused \
  "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img" \
  -o "$base_image"
curl -fsSL --retry 3 --retry-delay 5 --retry-connrefused \
  "https://cloud-images.ubuntu.com/jammy/current/SHA256SUMS" \
  -o "$vm_dir/SHA256SUMS"
base_image_checksum="$(grep -E '(^|[ *])jammy-server-cloudimg-amd64\.img$' "$vm_dir/SHA256SUMS" | awk 'NR == 1 {print $1}')"
if [[ -z "$base_image_checksum" ]]; then
  echo "ERROR: could not find checksum for Ubuntu cloud image." >&2
  exit 1
fi
echo "${base_image_checksum}  ${base_image}" | sha256sum -c -
qemu-img create -f qcow2 -F qcow2 -b "$base_image" "$worker_disk" 40G

cat > "$user_data" <<EOF
#cloud-config
users:
  - name: root
    lock_passwd: true
    ssh_authorized_keys:
      - $(cat "$key_path.pub")
disable_root: false
ssh_pwauth: false
package_update: true
packages:
  - ca-certificates
  - curl
  - openssh-server
write_files:
  - path: /etc/ssh/sshd_config.d/casos-worker.conf
    permissions: '0644'
    content: |
      PermitRootLogin prohibit-password
      PasswordAuthentication no
runcmd:
  - systemctl restart ssh
EOF

cat > "$meta_data" <<EOF
instance-id: casos-worker-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}
local-hostname: casos-worker
EOF

cat > "$network_config" <<EOF
version: 2
ethernets:
  worker0:
    match:
      macaddress: "$worker_mac"
    addresses:
      - ${worker_vm_ip}/24
    routes:
      - to: default
        via: $host_bridge_ip
    nameservers:
      addresses:
        - 1.1.1.1
        - 8.8.8.8
EOF

cloud-localds --network-config="$network_config" "$seed_image" "$user_data" "$meta_data"

qemu-system-x86_64 \
  -daemonize \
  -enable-kvm \
  -cpu host \
  -smp 2 \
  -m 2048 \
  -drive "file=$worker_disk,if=virtio,format=qcow2" \
  -drive "file=$seed_image,if=virtio,format=raw,readonly=on" \
  -netdev "tap,id=net0,ifname=$tap_name,script=no,downscript=no" \
  -device "virtio-net-pci,netdev=net0,mac=$worker_mac" \
  -display none \
  -serial "file:$vm_dir/serial.log" \
  -pidfile "$vm_dir/qemu.pid"

ssh_args=(
  -o StrictHostKeyChecking=no
  -o UserKnownHostsFile=/dev/null
  -o ConnectTimeout=5
  -i "$key_path"
)

ssh_ready=false
for i in {1..90}; do
  echo "VM SSH connectivity check: attempt $i/90..." >&2
  if ssh "${ssh_args[@]}" root@"$worker_vm_ip" true 2>/dev/null; then
    ssh_ready=true
    break
  fi
  sleep 5
done

if [[ "$ssh_ready" != "true" ]]; then
  echo "QEMU worker VM did not become SSH-ready." >&2
  sudo cat "$vm_dir/serial.log" >&2 || true
  exit 1
fi

if ! ssh "${ssh_args[@]}" root@"$worker_vm_ip" "timeout 600 cloud-init status --wait >/dev/null" 2>/dev/null; then
  echo "QEMU worker VM SSH is reachable, but cloud-init did not finish successfully." >&2
  sudo cat "$vm_dir/serial.log" >&2 || true
  exit 1
fi

{
  echo "E2E_REAL_WORKER_SSH_HOST=$worker_vm_ip"
  echo "E2E_REAL_WORKER_SSH_PORT=22"
  echo "E2E_REAL_WORKER_SSH_USER=root"
  echo "E2E_REAL_WORKER_SSH_KEY_PATH=$key_path"
  echo "E2E_APISERVER_URL=https://${host_bridge_ip}:${E2E_APISERVER_PORT}"
  echo "E2E_DATA_DIR=$real_worker_data_dir"
  echo "apiserverBind=${host_bridge_ip}"
} >> "$GITHUB_ENV"

echo "Real worker VM is ready at $worker_vm_ip."
