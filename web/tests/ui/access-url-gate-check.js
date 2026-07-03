const assert = require("assert");
const fs = require("fs");
const path = require("path");
const {spawnSync} = require("child_process");
const {
  ACCESS_URL_FAILURE_CATEGORIES,
  ACCESS_URL_TYPES,
  classifyAccessUrlFailure,
} = require("./access-url-diagnostics");
const {
  buildAccessUrlSummaryPayload,
} = require("../../scripts/app-store-access-url-summary");

const gateScriptPath = path.join(__dirname, "../../scripts/app-store-access-url-gate.js");
const gateTempDir = fs.mkdtempSync(path.join(__dirname, ".access-url-gate-"));

function writeGatePayload(name, payload) {
  const outputPath = path.join(gateTempDir, name);
  fs.writeFileSync(outputPath, JSON.stringify(payload, null, 2), "utf8");
  return outputPath;
}

function runGate(summaryPath, args = [], env = {}) {
  const result = spawnSync(process.execPath, [gateScriptPath, summaryPath, ...args], {
    encoding: "utf8",
    env: {
      ...process.env,
      APP_STORE_ACCESS_EXIT_CODE_PATH: path.join(gateTempDir, "missing-exit-code.txt"),
      ...env,
    },
  });
  return {
    ...result,
    outputText: `${result.stdout || ""}${result.stderr || ""}`,
  };
}

try {
  const normalizedPayload = buildAccessUrlSummaryPayload([
    {
      reachable: false,
      chartName: "legacy-chart",
      releaseName: "legacy-release",
      classification: {
        category: "legacy-category",
      },
    },
  ]);
  assert.strictEqual(
    normalizedPayload.outcomes[0].classification.category,
    ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC,
    "summary payloads should normalize unrecognized categories before writing JSON"
  );
  assert.match(normalizedPayload.outcomes[0].classification.detail, /not recorded/);
  assert.doesNotMatch(JSON.stringify(normalizedPayload), /unknown/i);

  const reportableGate = runGate(writeGatePayload("reportable.json", {
    version: 1,
    outcomes: [
      {
        reachable: false,
        chartName: "podinfo",
        releaseName: "podinfo-reportable",
        namespace: "default",
        deploymentName: "podinfo",
        serviceName: "podinfo",
        serviceType: "NodePort",
        accessUrl: "http://192.168.250.2:31080",
        classification: classifyAccessUrlFailure({error: new Error("net::ERR_CONNECTION_REFUSED")}),
      },
    ],
  }));
  assert.strictEqual(reportableGate.status, 1, "reportable Access URL categories should fail the product gate");
  assert.match(reportableGate.outputText, /::error::/);
  assert.match(reportableGate.outputText, new RegExp(ACCESS_URL_FAILURE_CATEGORIES.NODE_PORT_UNREACHABLE));
  assert.match(reportableGate.outputText, /deployment=default\/podinfo/);
  assert.match(reportableGate.outputText, /service=default\/podinfo:NodePort/);
  assert.doesNotMatch(reportableGate.outputText, /unknown/i);

  const diagnosticGate = runGate(writeGatePayload("diagnostic.json", {
    version: 1,
    outcomes: [
      {
        reachable: false,
        chartName: "podinfo",
        releaseName: "podinfo-diagnostic",
        classification: classifyAccessUrlFailure({responseStatus: 503, body: "service unavailable"}),
      },
    ],
  }));
  assert.strictEqual(diagnosticGate.status, 0, "diagnostic-only Access URL categories should not fail the product gate");
  assert.match(diagnosticGate.outputText, /::warning::/);
  assert.match(diagnosticGate.outputText, new RegExp(ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC));
  assert.doesNotMatch(diagnosticGate.outputText, /::error::/);
  assert.doesNotMatch(diagnosticGate.outputText, /unknown/i);

  const unreachableDiagnosticGate = runGate(writeGatePayload("unreachable-diagnostic.json", {
    version: 1,
    outcomes: [
      {
        reachable: false,
        chartName: "podinfo",
        releaseName: "podinfo-domain",
        namespace: "default",
        deploymentName: "podinfo-domain",
        serviceName: "podinfo-domain",
        serviceType: "ClusterIP",
        accessUrlType: ACCESS_URL_TYPES.DOMAIN,
        accessUrl: "http://podinfo-domain.casos.invalid",
        classification: classifyAccessUrlFailure({
          accessUrlType: ACCESS_URL_TYPES.DOMAIN,
          error: new Error("net::ERR_NAME_NOT_RESOLVED"),
        }),
      },
    ],
  }));
  assert.strictEqual(
    unreachableDiagnosticGate.status,
    1,
    "a rendered Access URL that cannot be opened should fail the UI gate even when the category is diagnostic"
  );
  assert.match(unreachableDiagnosticGate.outputText, /::error::/);
  assert.match(unreachableDiagnosticGate.outputText, new RegExp(ACCESS_URL_FAILURE_CATEGORIES.DOMAIN_ACCESS_URL_UNREACHABLE));
  assert.match(unreachableDiagnosticGate.outputText, /target=domain/);
  assert.doesNotMatch(unreachableDiagnosticGate.outputText, /unknown/i);

  const mixedDiagnosticGate = runGate(writeGatePayload("mixed-diagnostic.json", {
    version: 1,
    outcomes: [
      {
        reachable: false,
        chartName: "podinfo",
        releaseName: "podinfo-ci",
        namespace: "default",
        classification: classifyAccessUrlFailure({
          appWorkloadDiagnostic: true,
          detail: "release did not create a NodePort Service",
        }),
      },
      {
        reachable: false,
        chartName: "podinfo",
        releaseName: "podinfo-np0",
        namespace: "default",
        deploymentName: "podinfo-np0",
        serviceName: "podinfo-np0",
        serviceType: "NodePort",
        accessUrlType: ACCESS_URL_TYPES.NODEPORT,
        accessUrl: "http://192.168.223.2:31081",
        classification: classifyAccessUrlFailure({
          appWorkloadDiagnostic: true,
          detail: "NodePort service has no running pods",
        }),
      },
    ],
  }));
  assert.strictEqual(mixedDiagnosticGate.status, 1, "mixed diagnostics should fail only for the rendered URL outcome");
  const mixedErrorLines = mixedDiagnosticGate.outputText
    .split(/\r?\n/)
    .filter(line => line.includes("::error::"));
  assert.ok(mixedErrorLines.some(line => line.includes("podinfo-np0")));
  assert.ok(
    mixedErrorLines.every(line => !line.includes("podinfo-ci")),
    "UI failure errors should not include no-URL diagnostic examples from the same category"
  );

  const missingArgValueGate = runGate(writeGatePayload("arg.json", {version: 1, outcomes: []}), [
    "--fallback-category",
    "--fallback-detail", "detail should not be consumed as a category",
  ]);
  assert.strictEqual(missingArgValueGate.status, 0, "invalid gate arguments should stay diagnostic");
  assert.match(missingArgValueGate.outputText, /::warning::/);
  assert.match(missingArgValueGate.outputText, /requires a value/);
  assert.doesNotMatch(missingArgValueGate.outputText, /unknown/i);

  const missingGate = runGate(path.join(gateTempDir, "missing.json"), [
    "--fallback-category", ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC,
    "--fallback-detail", "selector did not produce an App Store Access URL target",
  ]);
  assert.strictEqual(missingGate.status, 0, "missing summary should be a diagnostic warning, not a product failure");
  assert.match(missingGate.outputText, /::warning::/);
  assert.match(missingGate.outputText, new RegExp(ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC));
  assert.doesNotMatch(missingGate.outputText, /unknown/i);

  const invalidSummaryPath = path.join(gateTempDir, "invalid.json");
  fs.writeFileSync(invalidSummaryPath, "{not json", "utf8");
  const invalidGate = runGate(invalidSummaryPath, [
    "--fallback-category", ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC,
    "--fallback-detail", "QEMU worker preparation failed before the experiment",
  ]);
  assert.strictEqual(invalidGate.status, 0, "invalid summary should stay diagnostic");
  assert.match(invalidGate.outputText, new RegExp(ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC));
  assert.doesNotMatch(invalidGate.outputText, /unknown/i);
  assert.strictEqual(fs.readFileSync(`${invalidSummaryPath}.original`, "utf8"), "{not json");

  const qemuMissingSummaryPath = path.join(gateTempDir, "qemu-missing.json");
  const qemuGate = runGate(qemuMissingSummaryPath, [], {
    HAS_APP_STORE_ACCESS_TESTS: "true",
    PREPARE_APP_STORE_WORKER_OUTCOME: "failure",
  });
  assert.strictEqual(qemuGate.status, 0, "QEMU setup failures should remain CI diagnostics");
  assert.match(qemuGate.outputText, new RegExp(ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC));
  assert.match(qemuGate.outputText, /QEMU worker VM preparation/);
  assert.doesNotMatch(qemuGate.outputText, /unknown/i);

  const unselectedGate = runGate(path.join(gateTempDir, "unselected.json"), [], {
    HAS_APP_STORE_ACCESS_TESTS: "false",
    PREPARE_APP_STORE_WORKER_OUTCOME: "skipped",
  });
  assert.strictEqual(unselectedGate.status, 0, "selector-empty access tests should stay diagnostic");
  assert.match(unselectedGate.outputText, new RegExp(ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC));
  assert.match(unselectedGate.outputText, /changed-path selector/);
  assert.doesNotMatch(unselectedGate.outputText, /unknown/i);

  const exitCodePath = path.join(gateTempDir, "experiment-exit-code.txt");
  fs.writeFileSync(exitCodePath, "124\n", "utf8");
  const playwrightExitGate = runGate(path.join(gateTempDir, "playwright-missing.json"), [], {
    APP_STORE_ACCESS_EXIT_CODE_PATH: exitCodePath,
    HAS_APP_STORE_ACCESS_TESTS: "true",
    PREPARE_APP_STORE_WORKER_OUTCOME: "success",
  });
  assert.strictEqual(playwrightExitGate.status, 0, "Playwright harness exits should stay diagnostic without reportable summary evidence");
  assert.match(playwrightExitGate.outputText, /exited with code 124/);
  assert.match(playwrightExitGate.outputText, new RegExp(ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC));
  assert.doesNotMatch(playwrightExitGate.outputText, /unknown/i);

  const emptyExitCodePath = path.join(gateTempDir, "empty-exit-code.txt");
  fs.writeFileSync(emptyExitCodePath, "", "utf8");
  const emptyExitCodeGate = runGate(path.join(gateTempDir, "empty-exit-missing-summary.json"), [], {
    APP_STORE_ACCESS_EXIT_CODE_PATH: emptyExitCodePath,
    HAS_APP_STORE_ACCESS_TESTS: "true",
    PREPARE_APP_STORE_WORKER_OUTCOME: "success",
  });
  assert.strictEqual(emptyExitCodeGate.status, 0, "empty exit-code files should stay generic diagnostics");
  assert.match(emptyExitCodeGate.outputText, /did not produce a product-gate summary/);
  assert.doesNotMatch(emptyExitCodeGate.outputText, /exited with code not-recorded/);
  assert.doesNotMatch(emptyExitCodeGate.outputText, /unknown/i);

  const malformedSummaryGate = runGate(writeGatePayload("malformed-summary.json", {
    version: 1,
    summary: {
      totalCount: 1,
      diagnosticCategories: [
        {
          count: 1,
          examples: [{detail: "legacy summary bucket omitted category"}],
        },
      ],
    },
  }));
  assert.strictEqual(malformedSummaryGate.status, 0, "malformed legacy summary buckets should stay diagnostic");
  assert.match(malformedSummaryGate.outputText, new RegExp(ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC));
  assert.doesNotMatch(malformedSummaryGate.outputText, /undefined/);
  assert.doesNotMatch(malformedSummaryGate.outputText, /unknown/i);

  const legacyGate = runGate(writeGatePayload("legacy.json", {
    version: 1,
    outcomes: [
      {
        reachable: false,
        chartName: "legacy-chart",
        releaseName: "legacy-release",
        classification: {
          category: "legacy-category",
          detail: "legacy caller returned a removed category",
        },
      },
    ],
  }));
  assert.strictEqual(legacyGate.status, 0, "legacy categories should normalize to a diagnostic gate result");
  assert.match(legacyGate.outputText, new RegExp(ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC));
  assert.doesNotMatch(legacyGate.outputText, /legacy-category/);
  assert.doesNotMatch(legacyGate.outputText, /unknown/i);
} finally {
  fs.rmSync(gateTempDir, {force: true, recursive: true});
}

console.log("access URL gate checks passed");
