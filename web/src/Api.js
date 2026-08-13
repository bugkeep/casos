let serverUrl = "";
let authStatus = null;

export function setServerUrl(value) {
  serverUrl = value;
}

export function setAuthStatus(status) {
  authStatus = status;
}

export function apiFetch(path, options = {}) {
  const method = (options.method || "GET").toUpperCase();
  const headers = new Headers(options.headers || {});
  const baseUrl = new URL(serverUrl || window.location.origin);
  const requestUrl = new URL(path, baseUrl);
  const isCasOSRequest = requestUrl.origin === baseUrl.origin && (/^\/api(?:\/|$)/.test(requestUrl.pathname) || /^\/k8s(?:\/|$)/.test(requestUrl.pathname));
  if (isCasOSRequest && ["POST", "PUT", "PATCH", "DELETE"].includes(method) && authStatus?.csrfToken) {
    headers.set("X-CSRF-Token", authStatus.csrfToken);
  }
  const requestOptions = {
    ...options,
    method,
    headers,
  };
  if (isCasOSRequest) {
    requestOptions.credentials = "include";
  }
  return fetch(requestUrl.toString(), requestOptions);
}
