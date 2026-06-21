import * as Setting from "./Setting";

function toAbsoluteUrl(path) {
  if (!path) {
    return "";
  }
  if (path.startsWith("http://") || path.startsWith("https://")) {
    return path;
  }
  return `${Setting.ServerUrl}${path}`;
}

export function getServiceAccessInfo(service) {
  if (!service) {
    return {urls: [], message: ""};
  }
  return {
    urls: service.accessReady && service.accessUrl ? [toAbsoluteUrl(service.accessUrl)] : [],
    message: service.accessMessage ?? "",
  };
}

export function getDeploymentAccessInfo(deployment, services) {
  if (!deployment) {
    return {urls: [], message: ""};
  }
  const service = (services ?? []).find(s =>
    s.namespace === deployment.namespace &&
    s.name === deployment.name &&
    (s.type === "NodePort" || s.type === "LoadBalancer")
  );
  return getServiceAccessInfo(service);
}
