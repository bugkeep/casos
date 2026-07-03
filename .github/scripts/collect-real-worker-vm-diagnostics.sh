#!/usr/bin/env bash
set -o pipefail

vm_dir="${RUNNER_TEMP}/casos-worker-vm"
diag_dir="$vm_dir/diagnostics"
mkdir -p "$diag_dir" || exit 0

key_path="${E2E_REAL_WORKER_SSH_KEY_PATH:-${RUNNER_TEMP}/casos-worker-key}"
worker_host="${E2E_REAL_WORKER_SSH_HOST:-}"
worker_port="${E2E_REAL_WORKER_SSH_PORT:-22}"
worker_user="${E2E_REAL_WORKER_SSH_USER:-root}"

{
  # These values are logged verbatim into CI diagnostics artifacts.
  # Keep them limited to non-secret URLs and temporary paths.
  echo "E2E_APISERVER_URL=${E2E_APISERVER_URL:-}"
  echo "E2E_DATA_DIR=${E2E_DATA_DIR:-}"
  echo "apiserverBind=${apiserverBind:-}"
  echo
  ip addr
  echo
  ip route
  echo
  ss -lntp || true
  echo
  if [[ -n "${E2E_APISERVER_URL:-}" ]]; then
    curl -kvsS --connect-timeout 5 "${E2E_APISERVER_URL%/}/readyz" || true
    echo
    host_port="${E2E_APISERVER_URL#https://}"
    host_port="${host_port%%/*}"
    if command -v openssl >/dev/null 2>&1; then
      echo | timeout 10 openssl s_client -connect "$host_port" -servername "${host_port%:*}" 2>/dev/null \
        | timeout 10 openssl x509 -noout -subject -ext subjectAltName || true
    fi
  fi
} > "$diag_dir/runner-network.txt" 2>&1

if [[ -z "$worker_host" || ! -f "$key_path" ]]; then
  {
    echo "Worker SSH diagnostics skipped."
    echo "worker_host=$worker_host"
    echo "key_path=$key_path"
  } > "$diag_dir/worker-ssh-skipped.txt"
  exit 0
fi

ssh_args=(
  # Ephemeral CI VM: host key changes each run, so strict checking is not feasible.
  -o StrictHostKeyChecking=no
  -o UserKnownHostsFile=/dev/null
  -o ConnectTimeout=5
  -p "$worker_port"
  -i "$key_path"
)

run_diag() {
  local name="$1"
  local command="$2"
  {
    echo "$ $command"
    ssh "${ssh_args[@]}" "${worker_user}@${worker_host}" "$command" || true
  } > "$diag_dir/${name}.txt" 2>&1
}

shell_single_quote() {
  printf "'%s'" "$(printf "%s" "$1" | sed "s/'/'\"'\"'/g")"
}

worker_apiserver_url="${E2E_APISERVER_URL%/}"
worker_apiserver_url_quoted="$(shell_single_quote "$worker_apiserver_url")"

run_diag "worker-network" "date -u; hostname -f || hostname; ip addr; ip route; cat /etc/resolv.conf"
run_diag "worker-apiserver" "apiserver_url=$worker_apiserver_url_quoted; if [ -n \"\$apiserver_url\" ]; then curl -kvsS --connect-timeout 5 \"\$apiserver_url/readyz\" || true; fi"
run_diag "worker-services" "systemctl status containerd kubelet kube-proxy --no-pager || true"
run_diag "worker-journal-containerd" "journalctl -u containerd -n 200 --no-pager || true"
run_diag "worker-journal-kubelet" "journalctl -u kubelet -n 300 --no-pager || true"
run_diag "worker-journal-kube-proxy" "journalctl -u kube-proxy -n 200 --no-pager || true"
run_diag "worker-kubelet-files" "ls -la /etc/kubernetes /var/lib/kubelet /etc/cni/net.d /opt/cni/bin 2>/dev/null || true; if [ -f /etc/kubernetes/worker.kubeconfig ]; then grep -n 'server:' /etc/kubernetes/worker.kubeconfig || true; fi; if [ -f /var/lib/kubelet/config.yaml ]; then sed -n '1,160p' /var/lib/kubelet/config.yaml; fi"

exit 0
