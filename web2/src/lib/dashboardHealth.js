const HEALTH_STATES = new Set(["healthy", "unhealthy", "unknown"]);

export function getDashboardHealthState(stats) {
  stats ??= {};
  const unhealthyPods = Array.isArray(stats.unhealthyPods) ? stats.unhealthyPods : [];
  const nodeCountsAvailable = typeof stats.nodesTotal === "number" &&
    typeof stats.nodesReady === "number";
  const knownNodeFailure = nodeCountsAvailable && stats.nodesTotal > 0 &&
    stats.nodesReady !== stats.nodesTotal;
  const notReadyNodes = knownNodeFailure
    ? Math.max(stats.nodesTotal - stats.nodesReady, 0)
    : 0;

  let healthStatus = "unknown";
  if (knownNodeFailure || unhealthyPods.length > 0) {
    healthStatus = "unhealthy";
  } else if (HEALTH_STATES.has(stats.healthStatus)) {
    healthStatus = stats.healthStatus;
  } else if (stats.healthStatus === undefined) {
    if (typeof stats.healthy === "boolean") {
      healthStatus = stats.healthy ? "healthy" : "unhealthy";
    } else if (nodeCountsAvailable && stats.nodesTotal > 0 &&
      stats.nodesReady === stats.nodesTotal) {
      healthStatus = "healthy";
    }
  }

  return {healthStatus, notReadyNodes};
}
