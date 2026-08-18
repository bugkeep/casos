import * as Setting from "../Setting";

export function requestLECert(payload) {
  return fetch(`${Setting.ServerUrl}/api/request-le-cert`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(payload),
  }).then(res => res.json());
}

export function uploadCert(payload) {
  return fetch(`${Setting.ServerUrl}/api/upload-cert`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(payload),
  }).then(res => res.json());
}

export function getCertStatus(namespace, ingressName) {
  return fetch(
    `${Setting.ServerUrl}/api/get-cert-status?namespace=${encodeURIComponent(namespace)}&ingressName=${encodeURIComponent(ingressName)}`,
    {
      method: "GET",
      credentials: "include",
      headers: {"Accept-Language": Setting.getAcceptLanguage()},
    }
  ).then(res => res.json());
}
