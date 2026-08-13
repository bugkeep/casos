import * as Setting from "../Setting";

export function getDeployments(namespace = "") {
  return Setting.apiFetch(`/api/get-deployments?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addDeployment(deployment) {
  return Setting.apiFetch("/api/add-deployment", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(deployment),
  }).then(res => res.json());
}

export function updateDeployment(deployment) {
  return Setting.apiFetch("/api/update-deployment", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(deployment),
  }).then(res => res.json());
}

export function restartDeployment(namespace, name) {
  return Setting.apiFetch("/api/restart-deployment", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}

export function deleteDeployment(namespace, name) {
  return Setting.apiFetch("/api/delete-deployment", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
