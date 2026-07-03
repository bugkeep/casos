const childProcess = require("child_process");
const fs = require("fs");
const {
  E2E_APISERVER_URL,
  SSH_HOST,
  SSH_PORT,
  SSH_USER,
  WORKER_DIAGNOSTICS_LOG,
} = require("./app-store-access-url-config");
const {logAppStoreDiagnostic} = require("./app-store-access-url-log");

async function resetWorkerDiagnosticsLog() {
  try {
    await fs.promises.rm(WORKER_DIAGNOSTICS_LOG, {force: true});
  } catch (error) {
    console.warn(`Failed to reset worker diagnostics log: ${error.message}`);
  }
}

function collectWorkerNodeDiagnostics(reason) {
  const keyPath = process.env.E2E_APP_STORE_SSH_KEY_PATH;
  if (!keyPath || !SSH_USER || !SSH_HOST) {
    logAppStoreDiagnostic("worker-diagnostics-skip", {
      reason,
      detail: "SSH key path, user, or host is not available",
    });
    return Promise.resolve("");
  }
  if (/[\0\r\n]/.test(E2E_APISERVER_URL)) {
    logAppStoreDiagnostic("worker-diagnostics-skip", {
      reason,
      detail: "apiserver URL contains unsupported control characters",
    });
    return Promise.resolve("");
  }

  const remoteScript = [
    "set +e",
    "echo '== diagnostic context =='",
    `echo ${shellSingleQuote(`reason: ${reason}`)}`,
    "date -u || true",
    "hostnamectl || true",
    "echo '== network =='",
    "ip -brief addr || true",
    "ip route || true",
    "echo '== worker kubeconfig endpoint =='",
    "grep -E '^    server:' /etc/kubernetes/worker.kubeconfig || true",
    "echo '== apiserver readyz with worker CA =='",
    `apiserver_url=${shellSingleQuote(E2E_APISERVER_URL)}`,
    "curl -v --cacert /etc/kubernetes/ca.crt --connect-timeout 5 --max-time 20 \"$apiserver_url/readyz\" 2>&1 || true",
    "echo '== kubelet service unit =='",
    "sed -n '1,160p' /etc/systemd/system/kubelet.service || true",
    "echo '== kubelet config =='",
    "sed -n '1,160p' /var/lib/kubelet/config.yaml || true",
    "echo '== kubelet status =='",
    "systemctl status kubelet --no-pager || true",
    "echo '== kubelet journal =='",
    "journalctl -u kubelet --no-pager -n 240 || true",
    "echo '== containerd status =='",
    "systemctl status containerd --no-pager || true",
    "echo '== containerd journal =='",
    "journalctl -u containerd --no-pager -n 120 || true",
  ].join("\n");

  const args = [
    "-o", "StrictHostKeyChecking=no",
    "-o", "UserKnownHostsFile=/dev/null",
    "-o", "ConnectTimeout=10",
    "-i", keyPath,
    "-p", String(SSH_PORT),
    `${SSH_USER}@${SSH_HOST}`,
    `bash -c ${shellSingleQuote(remoteScript)}`,
  ];

  logAppStoreDiagnostic("worker-diagnostics-start", {reason, path: WORKER_DIAGNOSTICS_LOG});
  return new Promise(resolve => {
    childProcess.execFile("ssh", args, {timeout: 35 * 1000, maxBuffer: 2 * 1024 * 1024}, (error, stdout, stderr) => {
      const output = [
        `# worker diagnostics: ${reason}`,
        `# collectedAt: ${new Date().toISOString()}`,
        stdout,
        stderr ? `# stderr\n${stderr}` : "",
        error ? `# ssh error\n${error.message}` : "",
        "",
      ].filter(Boolean).join("\n");

      fs.promises.appendFile(WORKER_DIAGNOSTICS_LOG, output)
        .then(() => {
          logAppStoreDiagnostic("worker-diagnostics-finish", {
            reason,
            path: WORKER_DIAGNOSTICS_LOG,
            exitCode: error?.code ?? (error?.signal ? -1 : 0),
            signal: error?.signal || "",
          });
          resolve(error ? "" : WORKER_DIAGNOSTICS_LOG);
        })
        .catch(writeError => {
          logAppStoreDiagnostic("worker-diagnostics-write-fail", {error: writeError.message});
          logAppStoreDiagnostic("worker-diagnostics-finish", {
            reason,
            path: "",
            exitCode: error?.code ?? (error?.signal ? -1 : 0),
            signal: error?.signal || "",
          });
          resolve("");
        });
    });
  });
}

function shellSingleQuote(value) {
  return `'${String(value).replace(/'/g, "'\\''")}'`;
}

module.exports = {
  collectWorkerNodeDiagnostics,
  resetWorkerDiagnosticsLog,
};
