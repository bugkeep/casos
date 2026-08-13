import * as Setting from "../Setting";

export function getConfigMaps(namespace = "") {
  return Setting.apiFetch(`/api/get-configmaps?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addConfigMap(configmap) {
  return Setting.apiFetch("/api/add-configmap", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(configmap),
  }).then(res => res.json());
}

export function updateConfigMap(configmap) {
  return Setting.apiFetch("/api/update-configmap", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(configmap),
  }).then(res => res.json());
}

export function deleteConfigMap(namespace, name) {
  return Setting.apiFetch("/api/delete-configmap", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
