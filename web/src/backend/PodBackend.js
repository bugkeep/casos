import * as Setting from "../Setting";

export function getPods() {
  return fetch(`${Setting.ServerUrl}/api/get-pods`, {
    method: "GET",
    credentials: "include",
    headers: {
      "Accept-Language": Setting.getAcceptLanguage(),
    },
  }).then(res => res.json());
}
