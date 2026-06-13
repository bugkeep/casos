import * as Setting from "../Setting";

export function getClusterRoleBindings() {
  return fetch(`${Setting.ServerUrl}/api/get-clusterrolebindings`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addClusterRoleBinding(crb) {
  return fetch(`${Setting.ServerUrl}/api/add-clusterrolebinding`, {
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
  return fetch(`${Setting.ServerUrl}/api/update-clusterrolebinding`, {
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
  return fetch(`${Setting.ServerUrl}/api/delete-clusterrolebinding`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({name}),
  }).then(res => res.json());
}
