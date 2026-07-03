const fs = require("fs");
const path = require("path");
const {
  ACCESS_URL_FAILURE_CATEGORIES,
  shouldFailAccessUrlOutcome,
  summarizeAccessUrlOutcomes,
} = require("./access-url-diagnostics");
const {
  ACCESS_URL_GATE_SOURCE,
  buildAccessUrlSummaryPayload,
  diagnosticOutcome,
} = require("./app-store-access-url-summary");

const DEFAULT_FALLBACK_CATEGORY = ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC;
const DEFAULT_FALLBACK_DETAIL = "App Store Access URL experiment did not produce a product-gate summary";
const SUMMARY_SOURCE_SYNTHETIC = "synthetic-fallback";
const SUMMARY_SOURCE_WITHOUT_OUTCOMES = "summary-without-outcomes";
const DIAGNOSTIC_DETAIL_NO_OUTCOMES = "summary file did not include replayable outcomes";
const DETAIL_MISSING_FROM_SUMMARY_CATEGORY = "summary category did not include detail";
const DEFAULT_EXIT_CODE_PATH = "ui-app-store-access-exit-code.txt";
const LOG_VALUE_NOT_RECORDED = "not-recorded";

function parseArgs(argv = process.argv) {
  const args = argv.slice(2);
  const summaryPath = args.shift();
  const options = {
    fallbackCategory: DEFAULT_FALLBACK_CATEGORY,
    fallbackDetail: DEFAULT_FALLBACK_DETAIL,
  };
  while (args.length > 0) {
    const name = args.shift();
    if (name === "--fallback-category") {
      options.fallbackCategory = readOptionValue(name, args);
      continue;
    }
    if (name === "--fallback-detail") {
      options.fallbackDetail = readOptionValue(name, args);
      continue;
    }
    throw new Error(`Unsupported argument: ${name}`);
  }
  return {summaryPath, options};
}

function readOptionValue(name, args) {
  const value = args.shift();
  if (!value || value.startsWith("--")) {
    throw new Error(`${name} requires a value`);
  }
  return value;
}

function main(argv = process.argv) {
  let parsed;
  try {
    parsed = parseArgs(argv);
  } catch (error) {
    process.stderr.write(`::warning::${escapeCommandText(error.message)}\n`);
    return 0;
  }

  if (!parsed.summaryPath) {
    process.stderr.write("::warning::Usage: node scripts/app-store-access-url-gate.js <summary.json> [--fallback-category category] [--fallback-detail detail]\n");
    return 0;
  }

  const options = resolveFallbackOptions(parsed.options);
  const result = readGateSummary(parsed.summaryPath, options);
  result.failingAccessUrlSummary = summarizeAccessUrlOutcomes(
    extractFailingAccessUrlOutcomes(result.payload?.outcomes)
  );
  if (result.synthetic) {
    writeSyntheticSummary(parsed.summaryPath, result.payload);
  }
  printGateSummary(result);
  return result.failingAccessUrlSummary.failureCount > 0 ? 1 : 0;
}

function resolveFallbackOptions(options = {}, env = process.env) {
  const resolved = {
    fallbackCategory: options.fallbackCategory || DEFAULT_FALLBACK_CATEGORY,
    fallbackDetail: options.fallbackDetail || DEFAULT_FALLBACK_DETAIL,
  };
  const hasAppStoreAccessTestsRaw = env.HAS_APP_STORE_ACCESS_TESTS;
  const hasAppStoreAccessTests = String(hasAppStoreAccessTestsRaw || "").trim().toLowerCase();
  if (hasAppStoreAccessTestsRaw !== undefined && hasAppStoreAccessTests !== "true") {
    return {
      ...resolved,
      fallbackCategory: ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC,
      fallbackDetail: "No App Store Access URL UI regression tests were selected by the changed-path selector",
    };
  }

  if (hasAppStoreAccessTests === "true") {
    const prepareOutcome = String(env.PREPARE_APP_STORE_WORKER_OUTCOME || "skipped").trim() || "skipped";
    if (prepareOutcome !== "success") {
      return {
        ...resolved,
        fallbackCategory: ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC,
        fallbackDetail: `QEMU worker VM preparation did not complete successfully: ${prepareOutcome}`,
      };
    }
  }

  const exitCodePath = env.APP_STORE_ACCESS_EXIT_CODE_PATH || DEFAULT_EXIT_CODE_PATH;
  const exitCode = readExperimentExitCode(exitCodePath);
  if (exitCode !== null) {
    return {
      ...resolved,
      fallbackDetail: `Playwright App Store Access URL experiment exited with code ${exitCode} before writing a product-gate summary`,
    };
  }
  return resolved;
}

function readExperimentExitCode(exitCodePath) {
  try {
    if (!fs.existsSync(exitCodePath)) {
      return null;
    }
    const content = fs.readFileSync(exitCodePath, "utf8").trim();
    return content || null;
  } catch (error) {
    process.stderr.write(`::warning::${escapeCommandText(`failed to read App Store Access URL experiment exit code: ${error.message}`)}\n`);
    return null;
  }
}

function readGateSummary(summaryPath, options = {}) {
  let payload;
  try {
    payload = JSON.parse(fs.readFileSync(summaryPath, "utf8"));
  } catch (error) {
    const detail = error.code === "ENOENT"
      ? options.fallbackDetail || DEFAULT_FALLBACK_DETAIL
      : `${options.fallbackDetail || DEFAULT_FALLBACK_DETAIL}: ${error.message}`;
    const syntheticPayload = buildAccessUrlSummaryPayload([
      diagnosticOutcome(options.fallbackCategory, detail, {
        source: ACCESS_URL_GATE_SOURCE,
      }),
    ], {
      status: "diagnostic",
      summarySource: SUMMARY_SOURCE_SYNTHETIC,
    });
    return {
      payload: syntheticPayload,
      summary: syntheticPayload.summary,
      synthetic: true,
      source: SUMMARY_SOURCE_SYNTHETIC,
      detail,
    };
  }

  const outcomes = extractOutcomes(payload);
  if (outcomes.length === 0 && payload?.summary?.totalCount > 0) {
    const syntheticPayload = buildAccessUrlSummaryPayload([
      diagnosticOutcome(DEFAULT_FALLBACK_CATEGORY, DIAGNOSTIC_DETAIL_NO_OUTCOMES, {
        source: ACCESS_URL_GATE_SOURCE,
      }),
    ], {
      status: "diagnostic",
      summarySource: SUMMARY_SOURCE_WITHOUT_OUTCOMES,
    });
    return {
      payload: syntheticPayload,
      summary: syntheticPayload.summary,
      synthetic: true,
      source: SUMMARY_SOURCE_WITHOUT_OUTCOMES,
      detail: DIAGNOSTIC_DETAIL_NO_OUTCOMES,
    };
  }

  const summary = summarizeAccessUrlOutcomes(outcomes);
  return {
    payload: {
      ...payload,
      summary,
      outcomes,
    },
    summary,
    synthetic: false,
    source: payload?.summarySource || "summary-file",
    detail: payload?.status || "",
  };
}

function extractOutcomes(payload) {
  if (Array.isArray(payload?.outcomes)) {
    return payload.outcomes;
  }

  const categories = [
    ...arrayOrEmpty(payload?.summary?.categories),
    ...arrayOrEmpty(payload?.summary?.diagnosticCategories),
  ];
  return categories.flatMap(category => {
    const bucket = category || {};
    const examples = Array.isArray(bucket.examples) && bucket.examples.length > 0
      ? bucket.examples
      : [{}];
    return examples.map(example => ({
      ...example,
      // Category examples are failure buckets. Preserve a future explicit value, default to failed.
      reachable: example.reachable ?? false,
      classification: {
        category: bucket.category || ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC,
        detail: example.detail || bucket.reportReason || bucket.hint || DETAIL_MISSING_FROM_SUMMARY_CATEGORY,
      },
    }));
  });
}

function extractFailingAccessUrlOutcomes(outcomes) {
  return arrayOrEmpty(outcomes).filter(shouldFailAccessUrlOutcome);
}

function arrayOrEmpty(value) {
  return Array.isArray(value) ? value : [];
}

function writeSyntheticSummary(summaryPath, payload) {
  try {
    fs.mkdirSync(path.dirname(path.resolve(summaryPath)), {recursive: true});
    if (fs.existsSync(summaryPath)) {
      fs.copyFileSync(summaryPath, `${summaryPath}.original`);
    }
    fs.writeFileSync(summaryPath, `${JSON.stringify(payload, null, 2)}\n`, "utf8");
  } catch (error) {
    process.stderr.write(`::warning::${escapeCommandText(`failed to write synthetic App Store Access URL summary: ${error.message}`)}\n`);
  }
}

function printGateSummary(result) {
  const summary = result.summary;
  process.stdout.write([
    "[app-store-access-gate]",
    `source=${sanitizeLogValue(result.source)}`,
    `total=${summary.totalCount}`,
    `reachable=${summary.reachableCount}`,
    `failures=${summary.failureCount}`,
    `reportable=${summary.reportableFailureCount}`,
    `diagnostic=${summary.diagnosticFailureCount}`,
    `failedAccessUrls=${result.failingAccessUrlSummary?.failureCount || 0}`,
  ].join(" ") + "\n");

  for (const category of result.failingAccessUrlSummary?.categories || []) {
    process.stderr.write(`::error::${escapeCommandText(formatCategoryLine("CasOS Access URL product candidate", category))}\n`);
  }
  const failingDiagnosticCategories = new Set((result.failingAccessUrlSummary?.diagnosticCategories || [])
    .map(category => category.category));
  for (const category of result.failingAccessUrlSummary?.diagnosticCategories || []) {
    process.stderr.write(`::error::${escapeCommandText(formatCategoryLine("App Store Access URL UI failure", category))}\n`);
  }
  for (const category of summary.diagnosticCategories) {
    if (failingDiagnosticCategories.has(category.category)) {
      continue;
    }
    process.stderr.write(`::warning::${escapeCommandText(formatCategoryLine("App Store Access URL diagnostic", category))}\n`);
  }
  if (summary.failureCount === 0) {
    process.stderr.write("::warning::App Store Access URL gate found no failed Access URL samples to classify\n");
  }
  if (result.synthetic) {
    process.stderr.write(`::warning::${escapeCommandText(`App Store Access URL gate used fallback summary: ${result.detail || DEFAULT_FALLBACK_DETAIL}`)}\n`);
  }
}

function formatCategoryLine(prefix, category) {
  return [
    prefix,
    `category=${category.category || LOG_VALUE_NOT_RECORDED}`,
    `scope=${category.scope || LOG_VALUE_NOT_RECORDED}`,
    `count=${category.count}`,
    `reason=${category.reportReason || LOG_VALUE_NOT_RECORDED}`,
    `examples=${formatExamples(category.examples)}`,
  ].join("; ");
}

function formatExamples(examples = []) {
  if (examples.length === 0) {
    return LOG_VALUE_NOT_RECORDED;
  }
  return examples.map(example => [
    `repo=${example.repoName || "repo-not-recorded"}`,
    `chart=${example.chartName || "chart-not-recorded"}`,
    `release=${formatNamespacedValue(example.namespace, example.releaseName || "release-not-recorded")}`,
    `deployment=${formatNamespacedValue(example.namespace, example.deploymentName || "deployment-not-recorded")}`,
    `service=${formatServiceValue(example)}`,
    `target=${example.accessUrlType || "target-not-recorded"}`,
    `ingress=${formatNamespacedValue(example.namespace, example.ingressName || "ingress-not-recorded")}`,
    `url=${example.accessUrl || "url-not-recorded"}`,
    `detail=${example.detail || "detail-not-recorded"}`,
  ].map(sanitizeLogValue).join("|")).join(",");
}

function formatNamespacedValue(namespace, name) {
  return `${namespace || "default"}/${name}`;
}

function formatServiceValue(example) {
  const serviceName = example.serviceName || "service-not-recorded";
  const serviceType = example.serviceType ? `:${example.serviceType}` : "";
  return formatNamespacedValue(example.namespace, `${serviceName}${serviceType}`);
}

function sanitizeLogValue(value) {
  return String(value).replace(/[\r\n]/g, " ").trim();
}

function escapeCommandText(value) {
  return String(value)
    .replace(/%/g, "%25")
    .replace(/\r/g, "%0D")
    .replace(/\n/g, "%0A");
}

if (require.main === module) {
  process.exitCode = main();
}

module.exports = {
  extractOutcomes,
  main,
  readGateSummary,
  resolveFallbackOptions,
};
