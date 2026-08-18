import * as Setting from "../Setting";

export function getResourceQuotas(namespace = "") {
  return fetch(`${Setting.ServerUrl}/api/get-resourcequotas?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addResourceQuota(resourcequota) {
  return fetch(`${Setting.ServerUrl}/api/add-resourcequota`, {
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
  return fetch(`${Setting.ServerUrl}/api/update-resourcequota`, {
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
  return fetch(`${Setting.ServerUrl}/api/delete-resourcequota`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
