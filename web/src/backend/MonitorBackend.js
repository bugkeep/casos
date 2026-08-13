import * as Setting from "../Setting";

function getHeaders() {
  return {"Accept-Language": Setting.getAcceptLanguage()};
}

export function getMonitorOverview() {
  return Setting.apiFetch("/api/get-monitor-overview", {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
  }).then(res => res.json());
}

export function getMonitorEvents(namespace = "", limit = 100) {
  const params = new URLSearchParams({limit});
  if (namespace) {params.set("namespace", namespace);}
  return Setting.apiFetch(`/api/get-monitor-events?${params}`, {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
  }).then(res => res.json());
}

export function getMonitorIssues() {
  return Setting.apiFetch("/api/get-monitor-issues", {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
  }).then(res => res.json());
}

export function getMonitorDiagnosis(issue, tailLines = 100, previous = true) {
  const params = new URLSearchParams({
    kind: issue.kind || "",
    name: issue.name || "",
    tailLines,
    previous,
  });
  if (issue.namespace) {params.set("namespace", issue.namespace);}
  return Setting.apiFetch(`/api/get-monitor-diagnosis?${params}`, {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
  }).then(res => res.json());
}
