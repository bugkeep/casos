import * as Setting from "../Setting";

export function getHPAs(namespace = "") {
  return fetch(`${Setting.ServerUrl}/api/get-hpas?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addHPA(hpa) {
  return fetch(`${Setting.ServerUrl}/api/add-hpa`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(hpa),
  }).then(res => res.json());
}

export function updateHPA(hpa) {
  return fetch(`${Setting.ServerUrl}/api/update-hpa`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(hpa),
  }).then(res => res.json());
}

export function deleteHPA(namespace, name) {
  return fetch(`${Setting.ServerUrl}/api/delete-hpa`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
