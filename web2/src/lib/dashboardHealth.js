const HEALTH_STATES = new Set(["healthy", "unhealthy", "unknown"]);

// The backend owns the healthy/unhealthy/unknown decision — it is the only side
// that knows whether the node and Pod lists were actually loaded. Anything it
// did not send is "unknown", never "healthy". notReadyNodes is derived here only
// to fill in the alert text.
export function getDashboardHealthState(stats) {
  const nodesTotal = typeof stats?.nodesTotal === "number" ? stats.nodesTotal : 0;
  const nodesReady = typeof stats?.nodesReady === "number" ? stats.nodesReady : 0;

  return {
    healthStatus: HEALTH_STATES.has(stats?.healthStatus) ? stats.healthStatus : "unknown",
    notReadyNodes: Math.max(nodesTotal - nodesReady, 0),
  };
}
