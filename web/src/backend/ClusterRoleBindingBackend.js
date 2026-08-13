import * as Setting from "../Setting";

export function getClusterRoleBindings() {
  return Setting.apiFetch("/api/get-clusterrolebindings", {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addClusterRoleBinding(crb) {
  return Setting.apiFetch("/api/add-clusterrolebinding", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(crb),
  }).then(res => res.json());
}

export function updateClusterRoleBinding(crb) {
  return Setting.apiFetch("/api/update-clusterrolebinding", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(crb),
  }).then(res => res.json());
}

export function deleteClusterRoleBinding(name) {
  return Setting.apiFetch("/api/delete-clusterrolebinding", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({name}),
  }).then(res => res.json());
}
