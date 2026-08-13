import * as Setting from "../Setting";

export function getStatefulSets(namespace = "") {
  return Setting.apiFetch(`/api/get-statefulsets?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addStatefulSet(statefulset) {
  return Setting.apiFetch("/api/add-statefulset", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(statefulset),
  }).then(res => res.json());
}

export function updateStatefulSet(statefulset) {
  return Setting.apiFetch("/api/update-statefulset", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(statefulset),
  }).then(res => res.json());
}

export function deleteStatefulSet(namespace, name) {
  return Setting.apiFetch("/api/delete-statefulset", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
