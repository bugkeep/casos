import * as Setting from "../Setting";

export function getSecrets(namespace = "") {
  return Setting.apiFetch(`/api/get-secrets?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addSecret(secret) {
  return Setting.apiFetch("/api/add-secret", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(secret),
  }).then(res => res.json());
}

export function updateSecret(secret) {
  return Setting.apiFetch("/api/update-secret", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(secret),
  }).then(res => res.json());
}

export function deleteSecret(namespace, name) {
  return Setting.apiFetch("/api/delete-secret", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
