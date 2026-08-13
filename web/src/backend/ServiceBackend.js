import * as Setting from "../Setting";

export function getServices(namespace = "") {
  return Setting.apiFetch(`/api/get-services?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addService(svc) {
  return Setting.apiFetch("/api/add-service", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(svc),
  }).then(res => res.json());
}

export function updateService(svc) {
  return Setting.apiFetch("/api/update-service", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(svc),
  }).then(res => res.json());
}

export function deleteService(namespace, name) {
  return Setting.apiFetch("/api/delete-service", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
