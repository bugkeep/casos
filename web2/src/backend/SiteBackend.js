import * as Setting from "../Setting";

export function getGlobalSites() {
  return fetch(`${Setting.ServerUrl}/api/get-global-sites`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => Setting.handleFetchResponse(res));
}

export function getSite(owner, name) {
  return fetch(`${Setting.ServerUrl}/api/get-site?id=${owner}/${encodeURIComponent(name)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => Setting.handleFetchResponse(res));
}

export function getBuiltInSite() {
  return fetch(`${Setting.ServerUrl}/api/get-built-in-site`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => Setting.handleFetchResponse(res));
}

export function updateSite(owner, name, site) {
  const newSite = Setting.deepCopy(site);
  return fetch(`${Setting.ServerUrl}/api/update-site?id=${owner}/${encodeURIComponent(name)}`, {
    method: "POST",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
    body: JSON.stringify(newSite),
  }).then(res => Setting.handleFetchResponse(res));
}

export function addSite(site) {
  return fetch(`${Setting.ServerUrl}/api/add-site`, {
    method: "POST",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
    body: JSON.stringify(site),
  }).then(res => Setting.handleFetchResponse(res));
}

export function deleteSite(site) {
  return fetch(`${Setting.ServerUrl}/api/delete-site`, {
    method: "POST",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
    body: JSON.stringify(site),
  }).then(res => Setting.handleFetchResponse(res));
}
