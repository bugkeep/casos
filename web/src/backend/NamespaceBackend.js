import * as Setting from "../Setting";

export function getNamespaces() {
  return Setting.apiFetch("/api/get-namespaces", {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addNamespace(namespace) {
  return Setting.apiFetch("/api/add-namespace", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(namespace),
  }).then(res => res.json());
}

export function forceDeleteNamespace(name) {
  return Setting.apiFetch("/api/force-delete-namespace", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({name}),
  }).then(res => res.json());
}

export function deleteNamespace(name) {
  return Setting.apiFetch("/api/delete-namespace", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({name}),
  }).then(res => res.json());
}
