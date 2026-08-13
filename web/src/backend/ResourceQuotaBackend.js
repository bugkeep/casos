import * as Setting from "../Setting";

export function getResourceQuotas(namespace = "") {
  return Setting.apiFetch(`/api/get-resourcequotas?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addResourceQuota(resourcequota) {
  return Setting.apiFetch("/api/add-resourcequota", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(resourcequota),
  }).then(res => res.json());
}

export function updateResourceQuota(resourcequota) {
  return Setting.apiFetch("/api/update-resourcequota", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(resourcequota),
  }).then(res => res.json());
}

export function deleteResourceQuota(namespace, name) {
  return Setting.apiFetch("/api/delete-resourcequota", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
