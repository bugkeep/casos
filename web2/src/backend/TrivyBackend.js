import * as Setting from "../Setting";

export function getTrivyScanResults() {
  return fetch(`${Setting.ServerUrl}/api/get-trivy-scan-results`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function triggerTrivyScan(image) {
  return fetch(`${Setting.ServerUrl}/api/trigger-trivy-scan`, {
    method: "POST",
    credentials: "include",
    headers: {"Content-Type": "application/json", "Accept-Language": Setting.getAcceptLanguage()},
    body: JSON.stringify({image}),
  }).then(res => res.json());
}

export function deleteTrivyScanResult(id) {
  return fetch(`${Setting.ServerUrl}/api/delete-trivy-scan-result`, {
    method: "POST",
    credentials: "include",
    headers: {"Content-Type": "application/json", "Accept-Language": Setting.getAcceptLanguage()},
    body: JSON.stringify({id}),
  }).then(res => res.json());
}
