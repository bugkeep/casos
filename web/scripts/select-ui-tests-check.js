const assert = require("assert");
const {execFileSync, spawnSync} = require("child_process");
const fs = require("fs");
const path = require("path");
const {
  ALL_REGRESSION_TESTS,
  APP_STORE_ACCESS_TEST,
  selectRegressionTests,
  splitHeavyRegressionTests,
} = require("./select-ui-tests");

function expectSelection(name, changedFiles, expectedTests) {
  assert.deepStrictEqual(selectRegressionTests(changedFiles), expectedTests, name);
}

expectSelection(
  "worker node UI changes select worker node regression only",
  ["web/src/MachineNodeDeployPanel.js"],
  ["tests/ui/worker-node.spec.js"]
);

expectSelection(
  "worker node deploy backend changes select the heavy access regression too",
  ["object/machine_node_deploy.go", "web/src/backend/MachineNodeDeployBackend.js"],
  ["tests/ui/worker-node.spec.js", APP_STORE_ACCESS_TEST]
);

expectSelection(
  "app store and helm changes select app store access regression",
  ["web/src/AppStorePage.js", "web/src/HelmInstallModal.js", "controllers/helm.go", "store/helm.go"],
  [APP_STORE_ACCESS_TEST]
);

expectSelection(
  "access URL display changes select app store access regression",
  ["web/src/backend/ServiceBackend.js", "web/src/backend/NodeBackend.js", "web/src/DeploymentListPage.js", "web/src/ServiceListPage.js"],
  [APP_STORE_ACCESS_TEST]
);

expectSelection(
  "access URL diagnostics changes select app store access regression",
  [
    "web/tests/ui/access-url-diagnostics.js",
    "web/tests/ui/access-url-diagnostics-check.js",
    "web/tests/ui/access-url-gate-check.js",
  ],
  [APP_STORE_ACCESS_TEST]
);

expectSelection(
  "app store access helper changes select app store access regression",
  ["web/tests/ui/app-store-access-url-worker.js", "web/tests/ui/app-store-access-url-log.js"],
  [APP_STORE_ACCESS_TEST]
);

expectSelection(
  "site changes rely on fixed smoke coverage",
  ["web/src/SiteEditPage.js"],
  []
);

expectSelection(
  "site list and backend changes select site regression",
  ["web/src/SiteListPage.js", "web/src/backend/SiteBackend.js", "object/site.go"],
  ["tests/ui/site-e2e.spec.js"]
);

expectSelection(
  "docs-only changes do not request extra regression tests",
  ["README.md", "docs/ci.md"],
  []
);

expectSelection(
  "UI test infrastructure changes run all regression tests",
  ["web/tests/ui/e2e-helpers.js"],
  [...ALL_REGRESSION_TESTS, APP_STORE_ACCESS_TEST]
);

expectSelection(
  "worker node test changes stay on the standard worker regression",
  ["web/tests/ui/worker-node.spec.js"],
  ["tests/ui/worker-node.spec.js"]
);

expectSelection(
  "machine frontend CRUD changes stay on the standard worker regression",
  ["web/src/MachineListPage.js", "web/src/MachineEditPage.js", "web/src/backend/MachineBackend.js"],
  ["tests/ui/worker-node.spec.js"]
);

expectSelection(
  "unknown frontend code changes run all regression tests",
  ["web/src/DashboardPage.js"],
  ALL_REGRESSION_TESTS
);

expectSelection(
  "worker node controller deployment files still select the heavy access regression",
  ["controllers/machine_node_deploy.go"],
  ["tests/ui/worker-node.spec.js", APP_STORE_ACCESS_TEST]
);

expectSelection(
  "worker node deployment files only match the targeted deployment helpers",
  ["deploy/containerd_config.go", "deploy/installer.go"],
  ["tests/ui/worker-node.spec.js", APP_STORE_ACCESS_TEST]
);

expectSelection(
  "standard regression triggers preserve an earlier heavy selection",
  ["web/src/backend/MachineNodeDeployBackend.js", "conf/app.conf"],
  [...ALL_REGRESSION_TESTS, APP_STORE_ACCESS_TEST]
);

expectSelection(
  "UI selector script changes run all regression tests",
  ["web/scripts/select-ui-tests.js"],
  [...ALL_REGRESSION_TESTS, APP_STORE_ACCESS_TEST]
);

expectSelection(
  "App Store Access URL gate script changes run all regression tests",
  [
    "web/scripts/access-url-diagnostics.js",
    "web/scripts/app-store-access-url-gate.js",
    "web/scripts/app-store-access-url-summary.js",
  ],
  [...ALL_REGRESSION_TESTS, APP_STORE_ACCESS_TEST]
);

expectSelection(
  "non-array inputs fall back to all regression tests",
  null,
  [...ALL_REGRESSION_TESTS, APP_STORE_ACCESS_TEST]
);

assert.deepStrictEqual(
  splitHeavyRegressionTests(["tests/ui/site-e2e.spec.js", APP_STORE_ACCESS_TEST]),
  {standard: ["tests/ui/site-e2e.spec.js"], heavy: [APP_STORE_ACCESS_TEST]},
  "heavy app store access test is split from standard regression tests"
);

const cliInputPath = path.join(__dirname, `.select-ui-tests-${process.pid}.txt`);
const cliCombinedOutputPath = path.join(__dirname, `.select-ui-tests-${process.pid}.combined.txt`);
const cliStandardOutputPath = path.join(__dirname, `.select-ui-tests-${process.pid}.standard.txt`);
const cliHeavyOutputPath = path.join(__dirname, `.select-ui-tests-${process.pid}.heavy.txt`);
fs.writeFileSync(cliInputPath, "web/src/backend/MachineNodeDeployBackend.js\n", "utf8");
try {
  const output = execFileSync(process.execPath, [path.join(__dirname, "select-ui-tests.js"), cliInputPath], {
    encoding: "utf8",
  });
  assert.strictEqual(output, `tests/ui/worker-node.spec.js\n${APP_STORE_ACCESS_TEST}\n`, "CLI prints selected regression tests");

  execFileSync(
    process.execPath,
    [
      path.join(__dirname, "select-ui-tests.js"),
      "--split",
      cliInputPath,
      cliCombinedOutputPath,
      cliStandardOutputPath,
      cliHeavyOutputPath,
    ],
    {encoding: "utf8"}
  );
  assert.strictEqual(fs.readFileSync(cliCombinedOutputPath, "utf8"), `tests/ui/worker-node.spec.js\n${APP_STORE_ACCESS_TEST}\n`);
  assert.strictEqual(fs.readFileSync(cliStandardOutputPath, "utf8"), `tests/ui/worker-node.spec.js\n`);
  assert.strictEqual(fs.readFileSync(cliHeavyOutputPath, "utf8"), `${APP_STORE_ACCESS_TEST}\n`);
} finally {
  fs.rmSync(cliInputPath, {force: true});
  fs.rmSync(cliCombinedOutputPath, {force: true});
  fs.rmSync(cliStandardOutputPath, {force: true});
  fs.rmSync(cliHeavyOutputPath, {force: true});
}

fs.writeFileSync(cliInputPath, "web/src/backend/MachineNodeDeployBackend.js\n", "utf8");
const missingOutputDir = path.join(__dirname, `.select-ui-tests-${process.pid}-missing`);
try {
  const failedSplit = spawnSync(
    process.execPath,
    [
      path.join(__dirname, "select-ui-tests.js"),
      "--split",
      cliInputPath,
      path.join(missingOutputDir, "combined.txt"),
      cliStandardOutputPath,
      cliHeavyOutputPath,
    ],
    {encoding: "utf8"}
  );
  assert.notStrictEqual(failedSplit.status, 0, "CLI split fails when an output file cannot be written");
  assert.match(failedSplit.stderr, /Error writing selected UI test file:/, "CLI split reports write failures clearly");
  assert.doesNotMatch(failedSplit.stderr, /\s+at\s+(Object\.|node:fs)/, "CLI split does not print a Node.js stack trace");
} finally {
  fs.rmSync(cliInputPath, {force: true});
  fs.rmSync(missingOutputDir, {force: true, recursive: true});
}
