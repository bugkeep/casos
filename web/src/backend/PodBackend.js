import * as Setting from "../Setting";

export function getPods(namespace = "") {
  return Setting.apiFetch(`/api/get-pods?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addPod(pod) {
  return Setting.apiFetch("/api/add-pod", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(pod),
  }).then(res => res.json());
}

export function updatePod(pod) {
  return Setting.apiFetch("/api/update-pod", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(pod),
  }).then(res => res.json());
}

export function getPodEvents(namespace, name) {
  return Setting.apiFetch(`/api/get-pod-events?namespace=${encodeURIComponent(namespace)}&name=${encodeURIComponent(name)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function getPodLogs(namespace, name, container = "", tailLines = 500) {
  const params = new URLSearchParams({namespace, name, tailLines});
  if (container) {params.set("container", container);}
  return Setting.apiFetch(`/api/get-pod-logs?${params}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function searchDockerHubImages(q) {
  return Setting.apiFetch(`/api/search-docker-hub-images?q=${encodeURIComponent(q)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function getDockerHubImageTags(image) {
  return Setting.apiFetch(`/api/get-docker-hub-image-tags?image=${encodeURIComponent(image)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function listPodFiles(namespace, name, container, dirPath) {
  const params = new URLSearchParams({namespace, name, container, path: dirPath});
  return Setting.apiFetch(`/api/pod-file-list?${params}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function downloadPodFile(namespace, name, container, filePath) {
  const params = new URLSearchParams({namespace, name, container, path: filePath});
  return Setting.apiFetch(`/api/pod-file-download?${params}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  });
}

export function uploadPodFile(namespace, name, container, destDir, file) {
  const form = new FormData();
  form.append("namespace", namespace);
  form.append("name", name);
  form.append("container", container);
  form.append("destDir", destDir);
  form.append("file", file);
  return Setting.apiFetch("/api/pod-file-upload", {
    method: "POST",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
    body: form,
  }).then(res => res.json());
}

export function deletePod(namespace, name) {
  return Setting.apiFetch("/api/delete-pod", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
