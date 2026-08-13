import * as Setting from "../Setting";

export function getStorageClasses() {
  return Setting.apiFetch("/api/get-storageclasses", {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addStorageClass(storageClass) {
  return Setting.apiFetch("/api/add-storageclass", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(storageClass),
  }).then(res => res.json());
}

export function updateStorageClass(storageClass) {
  return Setting.apiFetch("/api/update-storageclass", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(storageClass),
  }).then(res => res.json());
}

export function deleteStorageClass(name) {
  return Setting.apiFetch("/api/delete-storageclass", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({name}),
  }).then(res => res.json());
}
