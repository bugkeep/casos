import * as Setting from "../Setting";

export function signout() {
  return Setting.apiFetch("/api/auth/logout", {
    method: "POST",
    credentials: "include",
  }).then(res => res.json());
}

export function getAuthStatus() {
  return Setting.apiFetch("/api/auth/status").then(res => Setting.handleFetchResponse(res));
}

export function setup(username, password) {
  return authPost("/api/auth/setup", {username, password});
}

export function login(username, password) {
  return authPost("/api/auth/login", {username, password});
}

export function recover(newPassword) {
  return authPost("/api/auth/recover", {newPassword});
}

export function changePassword(currentPassword, newPassword) {
  return authPost("/api/auth/password", {currentPassword, newPassword});
}

function authPost(path, body) {
  return Setting.apiFetch(path, {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify(body),
  }).then(res => Setting.handleFetchResponse(res));
}
