import fs from "fs";
import path from "path";
import {fileURLToPath, pathToFileURL} from "url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));

// Registry of non-smoke regression specs selectable from changed paths.
const ALL_REGRESSION_TESTS = [
  "tests/ui/site-e2e.spec.js",
  "tests/ui/worker-node.spec.js",
  "tests/ui/worker-node-ready.spec.js",
  "tests/ui/app-store.spec.js",
];

// Routed screens live in src/pages and the drawers and dialogs they open live
// in src/components/shared, so a feature's patterns have to name both.
const WORKER_NODE_PATTERNS = [
  /^controllers\/machine(_node_deploy)?\.go$/,
  /^controllers\/node\.go$/,
  /^object\/machine(_node_deploy)?\.go$/,
  /^object\/node\.go$/,
  /^web\/src\/pages\/Machine(ListPage|EditPage)\.jsx$/,
  /^web\/src\/pages\/NodeListPage\.jsx$/,
  /^web\/src\/components\/shared\/machine-node-deploy-sheet\.jsx$/,
  /^web\/src\/backend\/(Machine(NodeDeploy)?|Node)Backend\.js$/,
  /^web\/tests\/ui\/worker-node(-ready)?\.spec\.js$/,
  /^web\/tests\/ui\/worker-node-helpers\.js$/,
];

const PLATFORM_READINESS_PATTERNS = [
  /^deploy\//,
  /^server\/(bootstrap|.*_bootstrap|apiserver|controllermanager|scheduler)\.go$/,
];

const SMOKE_COVERED_PATTERNS = [
  /^web\/src\/pages\/SiteEditPage\.jsx$/,
];

const SITE_PATTERNS = [
  /^controllers\/site\.go$/,
  /^object\/site\.go$/,
  /^web\/src\/pages\/SiteListPage\.jsx$/,
  /^web\/src\/backend\/SiteBackend\.js$/,
  /^web\/tests\/ui\/site-e2e\.spec\.js$/,
];

const APP_STORE_PATTERNS = [
  /^controllers\/helm\.go$/,
  /^object\/helm_repo\.go$/,
  /^store\/helm\.go$/,
  /^web\/src\/pages\/(AppStore|HelmRelease|DeploymentList|ServiceList)Page\.jsx$/,
  /^web\/src\/components\/shared\/helm-(install-dialog|compatibility-alert)\.jsx$/,
  /^web\/src\/components\/shared\/deployment-(dialogs|storage-editor)\.jsx$/,
  /^web\/src\/backend\/HelmBackend\.js$/,
  /^web\/tests\/ui\/app-store\.spec\.js$/,
  /^web\/tests\/ui\/app-store-helpers\.js$/,
];

// Anything that every screen is built out of — the shared table, the app shell,
// the route table, the build config — can break any spec, so it runs the lot.
const FULL_REGRESSION_PATTERNS = [
  /^\.github\/workflows\//,
  /^conf\/app\.conf$/,
  /^routers\/router\.go$/,
  /^routers\/static_filter\.go$/,
  /^web\/package\.json$/,
  /^web\/playwright\.config\.js$/,
  /^web\/vite\.config\.js$/,
  /^web\/src\/(App|Setting|nav|routes)\.(js|jsx)$/,
  /^web\/src\/components\/shared\/(data-table|form-dialog|confirm-dialog|resource-sheet)\.jsx$/,
  /^web\/src\/components\/ui\//,
  /^web\/src\/hooks\//,
  /^web\/src\/locales\//,
  /^web\/tests\/ui\/(e2e-helpers|routes-render\.spec)\.js$/,
  /^web\/yarn\.lock$/,
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
    return [...ALL_REGRESSION_TESTS];
  }

  const selectedTests = new Set();
  let runAllRegression = false;

  // Ordering matters: skip docs, honor all-regression triggers, then apply targeted and smoke-covered matches.
  for (const filePath of normalizedFiles) {
    if (matchesAny(filePath, DOCS_ONLY_PATTERNS)) {
      continue;
    }
    if (matchesAny(filePath, FULL_REGRESSION_PATTERNS)) {
      runAllRegression = true;
      continue;
    }
    if (matchesAny(filePath, PLATFORM_READINESS_PATTERNS)) {
      selectedTests.add("tests/ui/worker-node.spec.js");
      selectedTests.add("tests/ui/worker-node-ready.spec.js");
      continue;
    }
    if (matchesAny(filePath, WORKER_NODE_PATTERNS)) {
      selectedTests.add("tests/ui/worker-node.spec.js");
      selectedTests.add("tests/ui/worker-node-ready.spec.js");
      continue;
    }
    if (matchesAny(filePath, SITE_PATTERNS)) {
      selectedTests.add("tests/ui/site-e2e.spec.js");
      continue;
    }
    if (matchesAny(filePath, APP_STORE_PATTERNS)) {
      selectedTests.add("tests/ui/app-store.spec.js");
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
    return [...ALL_REGRESSION_TESTS];
  }

  return ALL_REGRESSION_TESTS.filter(testFile => selectedTests.has(testFile));
}

// Selects non-smoke UI regression specs for repository-relative changed paths.
function selectRegressionTests(changedFiles) {
  return selectRegressionTestsFromNormalized(normalizeChangedFiles(changedFiles));
}

function main(argv) {
  const changedFilesPath = argv[2];
  if (!changedFilesPath) {
    process.stderr.write("Usage: node scripts/select-ui-tests.js <changed-files.txt>\n");
    process.exitCode = 1;
    return;
  }

  const repoRoot = path.resolve(scriptDir, "..", "..");
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
  process.stdout.write(tests.length > 0 ? `${tests.join("\n")}\n` : "");
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  main(process.argv);
}

export {
  ALL_REGRESSION_TESTS,
  selectRegressionTests,
};
