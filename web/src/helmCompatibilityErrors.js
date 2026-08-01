export const helmCompatibilityMessageKeys = {
  INVALID_MANIFEST: "helm:Invalid Helm manifest",
  RESOURCE_NOT_SERVED: "helm:Helm resource not served",
  RESOURCE_NOT_FOUND: "helm:Helm resource not found",
  FORBIDDEN_BY_RBAC: "helm:Helm compatibility check forbidden",
  CONFLICT: "helm:Helm resource conflict",
  DISCOVERY_FAILED: "helm:Kubernetes discovery failed",
  API_UNAVAILABLE: "helm:Kubernetes API unavailable",
  HELM_DRY_RUN_FAILED: "helm:Helm dry run failed",
};

export function resolveHelmCompatibilityError(error, t) {
  const messageKey = helmCompatibilityMessageKeys[error?.code];
  if (!messageKey) {
    return {message: error?.message || "Helm install failed", gvk: "", detail: ""};
  }
  return {
    message: t(messageKey),
    gvk: error?.gvk || "",
    detail: error?.message || "",
  };
}
