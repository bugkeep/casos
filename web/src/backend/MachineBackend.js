import * as Setting from "../Setting";

export function getGlobalMachines() {
  return Setting.apiFetch("/api/get-global-machines", {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => Setting.handleFetchResponse(res));
}

export function getMachine(owner, name) {
  return Setting.apiFetch(`/api/get-machine?id=${owner}/${encodeURIComponent(name)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => Setting.handleFetchResponse(res));
}

export function updateMachine(owner, name, machine) {
  const newMachine = Setting.deepCopy(machine);
  return Setting.apiFetch(`/api/update-machine?id=${owner}/${encodeURIComponent(name)}`, {
    method: "POST",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
    body: JSON.stringify(newMachine),
  }).then(res => Setting.handleFetchResponse(res));
}

export function addMachine(machine) {
  return Setting.apiFetch("/api/add-machine", {
    method: "POST",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
    body: JSON.stringify(machine),
  }).then(res => Setting.handleFetchResponse(res));
}

export function addLocalWSLMachine() {
  return Setting.apiFetch("/api/add-local-wsl-machine", {
    method: "POST",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => Setting.handleFetchResponse(res));
}

export function deleteMachine(machine) {
  return Setting.apiFetch("/api/delete-machine", {
    method: "POST",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
    body: JSON.stringify(machine),
  }).then(res => Setting.handleFetchResponse(res));
}
