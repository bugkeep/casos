const fs = require("fs");
const path = require("path");

// Registry of non-smoke regression specs selectable from changed paths.
const ALL_REGRESSION_TESTS = [
  "tests/ui/site-e2e.spec.js",
  "tests/ui/worker-node.spec.js",
];

const APP_STORE_ACCESS_TEST = "tests/ui/app-store-access-url.spec.js";
const HEAVY_TESTS = [APP_STORE_ACCESS_TEST];
const ALL_TESTS = [...ALL_REGRESSION_TESTS, ...HEAVY_TESTS];

const WORKER_NODE_UI_PATTERNS = [
  /^controllers\/machine\.go$/,
  /^object\/machine\.go$/,
  /^web\/src\/Machine(ListPage|EditPage|NodeDeployPanel)\.js$/,
  /^web\/src\/backend\/MachineBackend\.js$/,
  /^web\/tests\/ui\/worker-node\.spec\.js$/,
];

const WORKER_NODE_DEPLOY_PATTERNS = [
  /^controllers\/machine_node_deploy\.go$/,
  /^object\/machine_node_deploy\.go$/,
  /^deploy\/(cni|containerd_config|executor|init|installer|key|kubeconfig|kubelet|kubeproxy|node_bootstrap|preflight|service|types)\.go$/,
  /^web\/src\/backend\/MachineNodeDeployBackend\.js$/,
];

const APP_STORE_ACCESS_PATTERNS = [
  /^controllers\/helm\.go$/,
  /^controllers\/node\.go$/,
  /^controllers\/service\.go$/,
  /^store\/helm\.go$/,
  /^web\/src\/AppStorePage\.js$/,
  /^web\/src\/Helm(InstallModal|ReleasePage)\.js$/,
  /^web\/src\/backend\/HelmBackend\.js$/,
  /^web\/src\/backend\/NodeBackend\.js$/,
  /^web\/src\/backend\/ServiceBackend\.js$/,
  /^web\/src\/DeploymentListPage\.js$/,
  /^web\/src\/ServiceListPage\.js$/,
  /^web\/tests\/ui\/app-store-access-url\.spec\.js$/,
];

const SMOKE_COVERED_PATTERNS = [
  /^web\/src\/SiteEditPage\.js$/,
];

const SITE_PATTERNS = [
  /^controllers\/site\.go$/,
  /^object\/site\.go$/,
  /^web\/src\/SiteListPage\.js$/,
  /^web\/src\/backend\/SiteBackend\.js$/,
  /^web\/tests\/ui\/site-e2e\.spec\.js$/,
];

const FULL_REGRESSION_PATTERNS = [
  /^conf\/app\.conf$/,
  /^routers\/router\.go$/,
  /^web\/package\.json$/,
  /^web\/playwright\.config\.js$/,
  /^web\/src\/(Conf|Setting)\.js$/,
  /^web\/src\/locales\//,
  /^web\/yarn\.lock$/,
];

const FULL_HEAVY_REGRESSION_PATTERNS = [
  /^\.github\/workflows\//,
  /^web\/tests\/ui\/e2e-helpers\.js$/,
  /^web\/scripts\/select-ui-tests\.js$/,
  /^web\/scripts\/select-ui-tests-check\.js$/,
];

const DOCS_ONLY_PATTERNS = [
  /(^|\/)(README|CHANGELOG|LICENSE)(\.[^/]*)?$/i,
  /\.md$/i,
  /^docs\//,
];

const CODE_ROOT_PATTERNS = [
  /^conf\//,
  /^controllers\//,
  /^main\.go$/,
  /^object\//,
  /^proxy\//,
  /^routers\//,
  /^web\/scripts\//,
  /^web\/src\//,
];

function normalizeChangedPath(filePath) {
  return String(filePath || "")
    .trim()
    .replace(/\\/g, "/")
    .replace(/^\.\//, "");
}

function matchesAny(filePath, patterns) {
  return patterns.some(pattern => pattern.test(filePath));
}

function isCodePath(filePath) {
  return matchesAny(filePath, CODE_ROOT_PATTERNS);
}

function normalizeChangedFiles(changedFiles) {
  if (!Array.isArray(changedFiles)) {
    return [];
  }
  return Array.from(new Set(
    changedFiles.map(normalizeChangedPath).filter(Boolean)
  ));
}

function selectRegressionTestsFromNormalized(normalizedFiles) {
  if (normalizedFiles.length === 0) {
    return ALL_TESTS;
  }

  const selectedTests = new Set();
  let runAllRegression = false;
  let runAllHeavyRegression = false;

  // Ordering matters: skip docs, honor heavy all-regression triggers (CI/test infra),
  // then standard all-regression triggers (config/routing), then targeted and smoke-covered matches.
  // If a file matching a standard all-regression trigger appears later in the iteration than
  // a file matching a targeted heavy pattern, the heavy test is retained via selectedTests.has().
  for (const filePath of normalizedFiles) {
    if (matchesAny(filePath, DOCS_ONLY_PATTERNS)) {
      continue;
    }
    if (matchesAny(filePath, FULL_HEAVY_REGRESSION_PATTERNS)) {
      runAllRegression = true;
      runAllHeavyRegression = true;
      continue;
    }
    if (matchesAny(filePath, FULL_REGRESSION_PATTERNS)) {
      runAllRegression = true;
      continue;
    }
    if (matchesAny(filePath, WORKER_NODE_DEPLOY_PATTERNS)) {
      selectedTests.add("tests/ui/worker-node.spec.js");
      selectedTests.add(APP_STORE_ACCESS_TEST);
      continue;
    }
    if (matchesAny(filePath, WORKER_NODE_UI_PATTERNS)) {
      selectedTests.add("tests/ui/worker-node.spec.js");
      continue;
    }
    if (matchesAny(filePath, APP_STORE_ACCESS_PATTERNS)) {
      selectedTests.add(APP_STORE_ACCESS_TEST);
      continue;
    }
    if (matchesAny(filePath, SITE_PATTERNS)) {
      selectedTests.add("tests/ui/site-e2e.spec.js");
      continue;
    }
    if (matchesAny(filePath, SMOKE_COVERED_PATTERNS)) {
      continue;
    }
    if (isCodePath(filePath)) {
      runAllRegression = true;
    }
  }

  if (runAllRegression) {
    // Include the heavy test when a full-heavy trigger was hit, or when a targeted
    // worker/app-store match already selected it before a later full-regression trigger.
    return (runAllHeavyRegression || selectedTests.has(APP_STORE_ACCESS_TEST))
      ? ALL_TESTS
      : ALL_REGRESSION_TESTS;
  }

  return ALL_TESTS.filter(testFile => selectedTests.has(testFile));
}

// Selects non-smoke UI regression specs for repository-relative changed paths.
function selectRegressionTests(changedFiles) {
  return selectRegressionTestsFromNormalized(normalizeChangedFiles(changedFiles));
}

function splitHeavyRegressionTests(testFiles) {
  const standard = [];
  const heavy = [];
  for (const testFile of testFiles) {
    if (HEAVY_TESTS.includes(testFile)) {
      heavy.push(testFile);
    } else {
      standard.push(testFile);
    }
  }
  return {standard, heavy};
}

function writeTestFiles(outputPath, testFiles) {
  try {
    // Empty output files are intentional: the workflow uses Bash `-s` to test whether any spec was selected.
    fs.writeFileSync(outputPath, testFiles.length > 0 ? `${testFiles.join("\n")}\n` : "");
  } catch (error) {
    process.stderr.write(`Error writing selected UI test file: ${outputPath}: ${error.message}\n`);
    process.exitCode = 1;
    return false;
  }
  return true;
}

function main(argv) {
  const args = argv.slice(2);
  const splitMode = args[0] === "--split";
  const usage = splitMode
    ? "Usage: node scripts/select-ui-tests.js --split <changed-files.txt> <combined-output.txt> <standard-output.txt> <heavy-output.txt>\n"
    : "Usage: node scripts/select-ui-tests.js <changed-files.txt>\n";
  if (args.length === 0 || (splitMode && args.length !== 5) || (!splitMode && args.length !== 1)) {
    process.stderr.write(usage);
    process.exitCode = 1;
    return;
  }

  const changedFilesPath = splitMode ? args[1] : args[0];
  const combinedOutputPath = splitMode ? args[2] : null;
  const standardOutputPath = splitMode ? args[3] : null;
  const heavyOutputPath = splitMode ? args[4] : null;

  const repoRoot = path.resolve(__dirname, "..", "..");
  let resolvedChangedFilesPath;
  try {
    resolvedChangedFilesPath = fs.realpathSync(path.resolve(changedFilesPath));
  } catch (error) {
    process.stderr.write(`Error reading changed files list: ${error.message}\n`);
    process.exitCode = 1;
    return;
  }

  const changedFilesPathRelative = path.relative(repoRoot, resolvedChangedFilesPath);
  if (changedFilesPathRelative.startsWith("..")) {
    process.stderr.write(`Error: changed files path is outside the repository: ${changedFilesPath}\n`);
    process.exitCode = 1;
    return;
  }

  let rawChangedFiles;
  try {
    rawChangedFiles = fs.readFileSync(resolvedChangedFilesPath, "utf8");
  } catch (error) {
    process.stderr.write(`Error reading changed files list: ${error.message}\n`);
    process.exitCode = 1;
    return;
  }

  const changedFiles = rawChangedFiles.split(/\r?\n/);
  const normalizedFiles = normalizeChangedFiles(changedFiles);
  if (normalizedFiles.length === 0) {
    process.stderr.write("Warning: no changed files detected; falling back to all regression tests.\n");
  }
  const tests = selectRegressionTestsFromNormalized(normalizedFiles);
  if (splitMode) {
    const {standard, heavy} = splitHeavyRegressionTests(tests);
    if (!writeTestFiles(combinedOutputPath, tests) ||
        !writeTestFiles(standardOutputPath, standard) ||
        !writeTestFiles(heavyOutputPath, heavy)) {
      return;
    }
    return;
  }
  process.stdout.write(tests.length > 0 ? `${tests.join("\n")}\n` : "");
}

if (require.main === module) {
  main(process.argv);
}

module.exports = {
  ALL_REGRESSION_TESTS,
  APP_STORE_ACCESS_TEST,
  HEAVY_TESTS,
  selectRegressionTests,
  splitHeavyRegressionTests,
};
