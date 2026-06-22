function normalizeManagedNodeName(name) {
  if (typeof name !== "string") {
    return "";
  }

  return name.trim().toLowerCase();
}

export function buildManagedNodeNameSet(managedNodes) {
  return new Set(
    (managedNodes ?? [])
      .map(node => normalizeManagedNodeName(node?.name))
      .filter(Boolean)
  );
}

export function isManagedClusterNode(nodeName, managedNodeNameSet) {
  const normalizedName = normalizeManagedNodeName(nodeName);
  if (!normalizedName) {
    return false;
  }

  return managedNodeNameSet.has(normalizedName);
}
