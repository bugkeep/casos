import * as Setting from "../Setting";

export function getAccount() {
  const fromPath = encodeURIComponent(window.location.pathname);
  return fetch(`${Setting.ServerUrl}/api/get-account?fromPath=${fromPath}`, {
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

export function updateAccount(account) {
  return fetch(`${Setting.ServerUrl}/api/update-account`, {
    method: "POST",
    credentials: "include",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify(account),
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
