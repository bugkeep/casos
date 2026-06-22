import * as Setting from "../Setting";

export function getNodes() {
  return fetch(`${Setting.ServerUrl}/api/get-nodes`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function updateNode(node) {
  return fetch(`${Setting.ServerUrl}/api/update-node`, {
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
  return fetch(`${Setting.ServerUrl}/api/delete-node`, {
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
  return fetch(`${Setting.ServerUrl}/api/get-worker-kubeconfig?nodeName=${encodeURIComponent(nodeName)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addManagedNode(node) {
  return fetch(`${Setting.ServerUrl}/api/add-managed-node`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(node),
  }).then(res => res.json());
}

export function preflightManagedNode(node) {
  return fetch(`${Setting.ServerUrl}/api/preflight-managed-node`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(node),
  }).then(res => res.json());
}

export function getManagedNodes() {
  return fetch(`${Setting.ServerUrl}/api/get-managed-nodes`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function getNodeDeployTasks(nodeId) {
  return fetch(`${Setting.ServerUrl}/api/get-node-deploy-tasks?nodeId=${encodeURIComponent(nodeId)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function getNodeDeployLogs(taskId) {
  return fetch(`${Setting.ServerUrl}/api/get-node-deploy-logs?taskId=${encodeURIComponent(taskId)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function repairManagedNode(id) {
  return fetch(`${Setting.ServerUrl}/api/repair-managed-node`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({id}),
  }).then(res => res.json());
}

export function removeManagedNode(id) {
  return fetch(`${Setting.ServerUrl}/api/remove-managed-node`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({id}),
  }).then(res => res.json());
}
