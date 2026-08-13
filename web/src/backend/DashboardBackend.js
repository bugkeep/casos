import * as Setting from "../Setting";

export function getDashboard() {
  return Setting.apiFetch("/api/get-dashboard", {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}
