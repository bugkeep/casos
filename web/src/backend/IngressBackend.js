import * as Setting from "../Setting";

export function getIngresses(namespace = "") {
  return Setting.apiFetch(`/api/get-ingresses?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addIngress(ingress) {
  return Setting.apiFetch("/api/add-ingress", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(ingress),
  }).then(res => res.json());
}

export function updateIngress(ingress) {
  return Setting.apiFetch("/api/update-ingress", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(ingress),
  }).then(res => res.json());
}

export function deleteIngress(namespace, name) {
  return Setting.apiFetch("/api/delete-ingress", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
