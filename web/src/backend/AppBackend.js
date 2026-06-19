import * as Setting from "../Setting";

export function getAppTemplates() {
  return fetch(`${Setting.ServerUrl}/api/get-app-templates`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function deployApp(req) {
  return fetch(`${Setting.ServerUrl}/api/deploy-app`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(req),
  }).then(res => res.json());
}
