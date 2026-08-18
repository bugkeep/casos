import * as Setting from "../Setting";

export function getServiceAccounts(namespace = "") {
  return fetch(`${Setting.ServerUrl}/api/get-serviceaccounts?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addServiceAccount(sa) {
  return fetch(`${Setting.ServerUrl}/api/add-serviceaccount`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(sa),
  }).then(res => res.json());
}

export function updateServiceAccount(sa) {
  return fetch(`${Setting.ServerUrl}/api/update-serviceaccount`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(sa),
  }).then(res => res.json());
}

export function deleteServiceAccount(namespace, name) {
  return fetch(`${Setting.ServerUrl}/api/delete-serviceaccount`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
