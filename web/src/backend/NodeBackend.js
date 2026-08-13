import * as Setting from "../Setting";

export function getNodes() {
  return Setting.apiFetch("/api/get-nodes", {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function updateNode(node) {
  return Setting.apiFetch("/api/update-node", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(node),
  }).then(res => res.json());
}

export function deleteNode(name) {
  return Setting.apiFetch("/api/delete-node", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({name}),
  }).then(res => res.json());
}

export function getWorkerKubeconfig(nodeName) {
  return Setting.apiFetch(`/api/get-worker-kubeconfig?nodeName=${encodeURIComponent(nodeName)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}
