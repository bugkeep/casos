import * as Setting from "../Setting";

export function getAccount() {
  return fetch(`${Setting.ServerUrl}/api/get-account`, {
    method: "GET",
    credentials: "include",
  }).then(res => res.json());
}

export function getSigninOptions() {
  return fetch(`${Setting.ServerUrl}/api/get-signin-options`, {
    method: "GET",
    credentials: "include",
  }).then(res => res.json());
}

export function signinWithPassword(username, password) {
  return fetch(`${Setting.ServerUrl}/api/signin`, {
    method: "POST",
    credentials: "include",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({username, password}),
  }).then(res => res.json());
}

export function initializeLocalAdmin(password) {
  return fetch(`${Setting.ServerUrl}/api/initialize-local-admin`, {
    method: "POST",
    credentials: "include",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({password}),
  }).then(res => res.json());
}

export function signin(code, state) {
  return fetch(`${Setting.ServerUrl}/api/signin?code=${code}&state=${state}`, {
    method: "POST",
    credentials: "include",
  }).then(res => res.json());
}

export function signout() {
  return fetch(`${Setting.ServerUrl}/api/signout`, {
    method: "POST",
    credentials: "include",
  }).then(res => res.json());
}
