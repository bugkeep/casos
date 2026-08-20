import * as Setting from "../Setting";

const lang = () => ({"Accept-Language": Setting.getAcceptLanguage()});
const jsonHeaders = () => ({"Content-Type": "application/json", ...lang()});

export function searchArtifactHub(q, page = 1) {
  return fetch(`${Setting.ServerUrl}/api/search-artifact-hub?q=${encodeURIComponent(q)}&page=${page}&limit=20`, {
    credentials: "include", headers: lang(),
  }).then(r => r.json());
}

export function getHelmRepos() {
  return fetch(`${Setting.ServerUrl}/api/get-helm-repos`, {credentials: "include", headers: lang()}).then(r => r.json());
}

export function addHelmRepo(repo) {
  return fetch(`${Setting.ServerUrl}/api/add-helm-repo`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify(repo),
  }).then(r => r.json());
}

export function deleteHelmRepo(id) {
  return fetch(`${Setting.ServerUrl}/api/delete-helm-repo?id=${id}`, {
    method: "POST", credentials: "include", headers: lang(),
  }).then(r => r.json());
}

export function getRepoCharts(url) {
  return fetch(`${Setting.ServerUrl}/api/get-repo-charts?url=${encodeURIComponent(url)}`, {
    credentials: "include", headers: lang(),
  }).then(r => r.json());
}

export function getHelmChartValues(chart, repo, version, signal) {
  return fetch(
    `${Setting.ServerUrl}/api/get-helm-chart-values?chart=${encodeURIComponent(chart)}&repo=${encodeURIComponent(repo)}&version=${encodeURIComponent(version ?? "")}`,
    {credentials: "include", headers: lang(), signal}
  ).then(r => r.json());
}

export function getHelmReleases(namespace = "all") {
  return fetch(`${Setting.ServerUrl}/api/get-helm-releases?namespace=${namespace}`, {
    credentials: "include", headers: lang(),
  }).then(r => r.json());
}

export function getHelmOperationTask(id) {
  return fetch(`${Setting.ServerUrl}/api/get-helm-operation-task?id=${encodeURIComponent(id)}`, {
    credentials: "include", headers: lang(),
  }).then(r => r.json());
}

// Returns the most recent operation CasOS ran for a release, with its logs.
// A release CasOS never operated on answers with a null task.
export function getHelmReleaseOperation(name, namespace) {
  return fetch(
    `${Setting.ServerUrl}/api/get-helm-release-operation?name=${encodeURIComponent(name)}&namespace=${encodeURIComponent(namespace)}`,
    {credentials: "include", headers: lang()}
  ).then(r => r.json());
}

export function installHelmChart(payload) {
  return fetch(`${Setting.ServerUrl}/api/install-helm-chart`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify(payload),
  }).then(r => r.json());
}

// onLine(line) is called for each displayable SSE message; returns "DONE" on completion.
async function helmChartStream(endpoint, action, payload, onLine, signal) {
  const resp = await fetch(`${Setting.ServerUrl}${endpoint}`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify(payload), signal,
  });
  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  for (;;) {
    const {done, value} = await reader.read(); // eslint-disable-line no-await-in-loop
    if (done) {break;}
    buf += decoder.decode(value, {stream: true});
    const parts = buf.split("\n\n");
    buf = parts.pop();
    for (const part of parts) {
      const dataLine = part.split("\n").find(line => line.startsWith("data: "));
      if (!dataLine) {continue;}
      const event = JSON.parse(dataLine.slice(6));
      if (event.message) {
        onLine(event.type === "warning" ? `WARNING: ${event.message}` : event.message);
      }
      if (event.type === "error") {
        const actionError = new Error(event.message || `Helm ${action} failed`);
        actionError.code = event.error?.code;
        actionError.gvk = event.error?.gvk;
        throw actionError;
      }
      if (event.type === "done") {
        onLine("DONE");
        return "DONE";
      }
    }
  }
  throw new Error(`helm ${action} stream ended before completion`);
}

// Closing the browser stream does not cancel a submitted Helm operation.
export function installHelmChartStream(payload, onLine, signal) {
  return helmChartStream("/api/install-helm-chart-stream", "install", payload, onLine, signal);
}

export function upgradeHelmChartStream(payload, onLine, signal) {
  return helmChartStream("/api/upgrade-helm-release-stream", "upgrade", payload, onLine, signal);
}

export function upgradeHelmRelease(payload) {
  return fetch(`${Setting.ServerUrl}/api/upgrade-helm-release`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify(payload),
  }).then(r => r.json());
}

export function rollbackHelmRelease(payload) {
  return fetch(`${Setting.ServerUrl}/api/rollback-helm-release`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify(payload),
  }).then(r => r.json());
}

export function uninstallHelmRelease(payload) {
  return fetch(`${Setting.ServerUrl}/api/uninstall-helm-release`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify(payload),
  }).then(r => r.json());
}

export function getHelmReleaseHistory(name, namespace) {
  return fetch(
    `${Setting.ServerUrl}/api/get-helm-release-history?name=${encodeURIComponent(name)}&namespace=${encodeURIComponent(namespace)}`,
    {credentials: "include", headers: lang()}
  ).then(r => r.json());
}
