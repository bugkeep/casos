import * as Setting from "../Setting";

export function getNetworkPolicies(namespace = "") {
  return fetch(`${Setting.ServerUrl}/api/get-networkpolicies?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addNetworkPolicy(networkpolicy) {
  return fetch(`${Setting.ServerUrl}/api/add-networkpolicy`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(networkpolicy),
  }).then(res => res.json());
}

export function updateNetworkPolicy(networkpolicy) {
  return fetch(`${Setting.ServerUrl}/api/update-networkpolicy`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(networkpolicy),
  }).then(res => res.json());
}

export function deleteNetworkPolicy(namespace, name) {
  return fetch(`${Setting.ServerUrl}/api/delete-networkpolicy`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
