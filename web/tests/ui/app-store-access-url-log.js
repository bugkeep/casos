const {randomUUID} = require("crypto");

const LOG_PREFIX = "[app-store-access]";

function emitAppStoreLog(payload) {
  console.log(`${LOG_PREFIX} ${JSON.stringify(payload)}`);
}

function createAppStoreStageTracker(baseDetails = {}) {
  const startedAt = Date.now();
  const durations = [];

  function log(event, details = {}) {
    emitAppStoreLog({
      event,
      elapsedMs: Date.now() - startedAt,
      ...baseDetails,
      ...details,
    });
  }

  async function run(stage, action, details = {}, options = {}) {
    const stageStartedAt = Date.now();
    log("stage-start", {stage, timeoutMs: options.timeoutMs, ...details});
    try {
      const result = await runWithOptionalTimeout(stage, action, options.timeoutMs);
      const durationMs = Date.now() - stageStartedAt;
      durations.push({stage, durationMs});
      log("stage-finish", {stage, durationMs});
      return result;
    } catch (error) {
      log("stage-fail", {stage, durationMs: Date.now() - stageStartedAt, error: compactError(error)});
      throw error;
    }
  }

  function finish() {
    const totalDurationMs = Date.now() - startedAt;
    const slowestDurations = [...durations]
      .sort((left, right) => right.durationMs - left.durationMs)
      .slice(0, 8);
    log("stage-summary", {totalDurationMs, durations, slowestDurations});
  }

  return {log, run, finish};
}

async function runWithOptionalTimeout(stage, action, timeoutMs) {
  if (!timeoutMs) {
    return action();
  }
  let timeout;
  try {
    return await Promise.race([
      action(),
      new Promise((_, reject) => {
        timeout = setTimeout(() => {
          reject(new Error(`stage ${stage} timed out after ${timeoutMs}ms`));
        }, timeoutMs);
      }),
    ]);
  } finally {
    clearTimeout(timeout);
  }
}

function compactError(error) {
  return {
    name: error?.name || "Error",
    message: compactErrorMessage(error).slice(0, 2000),
  };
}

function compactErrorMessage(error) {
  if (error?.message) {
    return String(error.message);
  }
  if (error === null || error === undefined || error === "") {
    return "error message unavailable";
  }
  if (typeof error === "string") {
    return error;
  }
  if (typeof error === "object") {
    return stringifyLogObject(error);
  }
  return String(error);
}

function stringifyLogObject(value) {
  try {
    const json = JSON.stringify(value);
    return json && json !== "{}" ? json : "error object without message";
  } catch (error) {
    return "error object could not be stringified";
  }
}

function logAppStoreProgress(stage, startedAt, details = {}) {
  emitAppStoreLog({
    event: "stage-progress",
    stage,
    elapsedMs: Date.now() - startedAt,
    ...details,
  });
}

function logAppStoreDiagnostic(event, details = {}) {
  emitAppStoreLog({
    event,
    ...details,
  });
}

function compactTask(task) {
  if (!task) {
    return null;
  }
  return {
    id: task.id,
    status: task.status,
    phase: task.phase,
    errorMsg: task.errorMsg || "",
    updatedAt: task.updatedAt || task.updateTime || task.updatedTime || "",
  };
}

function compactRelease(release) {
  if (!release) {
    return null;
  }
  return {
    name: release.name,
    namespace: release.namespace,
    status: release.status,
    chart: release.chart,
  };
}

function compactAppStoreChart(chart) {
  if (!chart) {
    return null;
  }
  return {
    key: chart.key,
    chartName: chart.chartName,
    chartVersion: chart.chartVersion,
    releasePrefix: chart.releasePrefix,
    nodePort: chart.nodePort,
    expectReachable: chart.expectReachable,
    expectedAccessUrlType: chart.expectedAccessUrlType,
    expectedCategory: chart.expectedCategory,
    expectedFailureLocation: chart.expectedFailureLocation,
    sampleReason: chart.sampleReason,
  };
}

function compactAccessOutcome(outcome) {
  if (!outcome) {
    return null;
  }
  return {
    chartName: outcome.chartName,
    chartVersion: outcome.chartVersion,
    releaseName: outcome.releaseName,
    namespace: outcome.namespace,
    deploymentName: outcome.deploymentName,
    serviceName: outcome.serviceName,
    serviceType: outcome.serviceType,
    accessUrlType: outcome.accessUrlType,
    ingressName: outcome.ingressName,
    ingressHost: outcome.ingressHost,
    accessUrl: outcome.accessUrl,
    reachable: outcome.reachable,
    attemptCount: outcome.attemptCount,
    expectedAccessUrlType: outcome.expectedAccessUrlType,
    expectedCategory: outcome.expectedCategory,
    expectedFailureLocation: outcome.expectedFailureLocation,
    category: outcome.classification?.category,
    scope: outcome.classification?.scope,
    reportable: outcome.classification?.reportable,
    reportReason: outcome.classification?.reportReason,
    detail: outcome.classification?.detail,
  };
}

function compactDeployLogs(logs, limit = 8) {
  return (logs || []).slice(-limit).filter(Boolean).map(log => ({
    level: log.level,
    message: log.message,
    createdAt: log.createdAt,
  }));
}

function makeResourceSuffix() {
  const raw = process.env.GITHUB_RUN_ID || process.env.E2E_TEST_TOKEN || String(process.pid);
  const runPart = String(raw).replace(/[^a-z0-9-]/gi, "").toLowerCase().slice(-8) || "local";
  return `${runPart}-${randomUUID().slice(0, 8)}`;
}

module.exports = {
  compactAccessOutcome,
  compactAppStoreChart,
  compactDeployLogs,
  compactRelease,
  compactTask,
  createAppStoreStageTracker,
  logAppStoreDiagnostic,
  logAppStoreProgress,
  makeResourceSuffix,
};
