const fs = require("fs");
const path = require("path");
const {
  ACCESS_URL_FAILURE_CATEGORIES,
  isKnownAccessUrlFailureCategory,
  summarizeAccessUrlOutcomes,
} = require("./access-url-diagnostics");

const ACCESS_URL_GATE_SOURCE = "app-store-access-gate";
const CLASSIFICATION_DETAIL_NOT_RECORDED = "classification detail was not recorded";
const DIAGNOSTIC_DETAIL_NOT_RECORDED = "diagnostic detail was not recorded";
// CI upload-artifact collects ./web/ui-app-store-access-summary.json.
const ACCESS_URL_SUMMARY_PATH = path.resolve(__dirname, "../ui-app-store-access-summary.json");

function buildAccessUrlSummaryPayload(outcomes, details = {}) {
  const safeOutcomes = outcomes ?? [];
  if (!Array.isArray(safeOutcomes)) {
    throw new TypeError("buildAccessUrlSummaryPayload: outcomes must be an array");
  }
  if (!details || typeof details !== "object" || Array.isArray(details)) {
    throw new TypeError("buildAccessUrlSummaryPayload: details must be an object");
  }
  const safeDetails = {...details};
  delete safeDetails.version;
  delete safeDetails.generatedAt;
  delete safeDetails.summary;
  delete safeDetails.outcomes;
  const compactOutcomes = safeOutcomes.map(compactSummaryOutcome);
  return {
    version: 1,
    generatedAt: new Date().toISOString(),
    ...safeDetails,
    summary: summarizeAccessUrlOutcomes(compactOutcomes),
    outcomes: compactOutcomes,
  };
}

async function writeAccessUrlSummary(outcomes, details = {}, outputPath = ACCESS_URL_SUMMARY_PATH) {
  let payload;
  try {
    payload = buildAccessUrlSummaryPayload(outcomes, details);
  } catch (error) {
    throw new Error(`failed to build App Store Access URL summary payload: ${error.message}`);
  }
  try {
    await fs.promises.mkdir(path.dirname(path.resolve(outputPath)), {recursive: true});
    await fs.promises.writeFile(outputPath, `${JSON.stringify(payload, null, 2)}\n`, "utf8");
  } catch (error) {
    throw new Error(`failed to write App Store Access URL summary to ${outputPath}: ${error.message}`);
  }
  return payload;
}

function diagnosticOutcome(category, detail, context = {}) {
  return {
    source: ACCESS_URL_GATE_SOURCE,
    chartName: "diagnostic",
    releaseName: "diagnostic",
    namespace: "default",
    ...context,
    // Keep diagnostic outcomes non-overridable; callers may add context, not change the verdict.
    reachable: false,
    attemptCount: 0,
    classification: {
      category: normalizeDiagnosticCategory(category),
      detail: detail || DIAGNOSTIC_DETAIL_NOT_RECORDED,
    },
  };
}

function normalizeDiagnosticCategory(category) {
  return isKnownAccessUrlFailureCategory(category)
    ? category
    : ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC;
}

function compactSummaryOutcome(outcome) {
  const safeOutcome = outcome || {};
  const compact = {
    source: safeOutcome.source,
    repoName: safeOutcome.repoName,
    chartName: safeOutcome.chartName,
    chartVersion: safeOutcome.chartVersion,
    releaseName: safeOutcome.releaseName,
    namespace: safeOutcome.namespace,
    deploymentName: safeOutcome.deploymentName,
    serviceName: safeOutcome.serviceName,
    serviceType: safeOutcome.serviceType,
    accessUrlType: safeOutcome.accessUrlType,
    ingressName: safeOutcome.ingressName,
    ingressHost: safeOutcome.ingressHost,
    accessUrl: safeOutcome.accessUrl,
    reachable: Boolean(safeOutcome.reachable),
    attemptCount: safeOutcome.attemptCount ?? 0,
    expectReachable: safeOutcome.expectReachable,
    expectedAccessUrlType: safeOutcome.expectedAccessUrlType,
    expectedCategory: safeOutcome.expectedCategory,
    expectedFailureLocation: safeOutcome.expectedFailureLocation,
    detail: safeOutcome.detail,
  };
  if (safeOutcome.classification) {
    const category = normalizeDiagnosticCategory(safeOutcome.classification.category);
    compact.classification = {
      category,
      scope: safeOutcome.classification.scope,
      reportable: safeOutcome.classification.reportable,
      reportReason: safeOutcome.classification.reportReason,
      detail: safeOutcome.classification.detail || CLASSIFICATION_DETAIL_NOT_RECORDED,
      hint: safeOutcome.classification.hint,
    };
  }
  return compact;
}

module.exports = {
  ACCESS_URL_GATE_SOURCE,
  ACCESS_URL_SUMMARY_PATH,
  buildAccessUrlSummaryPayload,
  diagnosticOutcome,
  writeAccessUrlSummary,
};
