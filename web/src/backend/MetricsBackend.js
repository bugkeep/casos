import * as Setting from "../Setting";

export function getMetrics() {
  return Setting.apiFetch("/api/get-metrics", {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}
