import * as Setting from "../Setting";

export function getConfigMaps(namespace = "", {page, limit} = {}) {
  const params = new URLSearchParams({namespace});
  if (page) {params.set("page", String(page));}
  if (limit) {params.set("limit", String(limit));}
  return fetch(`${Setting.ServerUrl}/api/get-configmaps?${params.toString()}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function getConfigMap(namespace, name) {
  return fetch(`${Setting.ServerUrl}/api/get-configmap?namespace=${encodeURIComponent(namespace)}&name=${encodeURIComponent(name)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addConfigMap(configmap) {
  return fetch(`${Setting.ServerUrl}/api/add-configmap`, {
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
  return fetch(`${Setting.ServerUrl}/api/update-configmap`, {
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
  return fetch(`${Setting.ServerUrl}/api/delete-configmap`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
