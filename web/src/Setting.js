import {message} from "antd";

export const ServerUrl = "";

export function showMessage(type, msg) {
  if (type === "success") {
    message.success(msg);
  } else if (type === "error") {
    message.error(msg);
  } else if (type === "info") {
    message.info(msg);
  }
}

export function getItem(label, key, icon, children) {
  return {key, icon, children, label};
}

export function getAcceptLanguage() {
  return "en";
}

export function isMobile() {
  return window.innerWidth < 768;
}
