import * as Setting from "../Setting";

export function getDaemonSets(namespace = "") {
  return Setting.apiFetch(`/api/get-daemonsets?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addDaemonSet(daemonset) {
  return Setting.apiFetch("/api/add-daemonset", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(daemonset),
  }).then(res => res.json());
}

export function updateDaemonSet(daemonset) {
  return Setting.apiFetch("/api/update-daemonset", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(daemonset),
  }).then(res => res.json());
}

export function deleteDaemonSet(namespace, name) {
  return Setting.apiFetch("/api/delete-daemonset", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
