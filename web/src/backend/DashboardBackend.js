import * as Setting from "../Setting";

export function getDashboard() {
  return fetch(`${Setting.ServerUrl}/api/get-dashboard`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}
