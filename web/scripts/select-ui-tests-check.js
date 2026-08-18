import assert from "assert";
import {execFileSync} from "child_process";
import fs from "fs";
import path from "path";
import {fileURLToPath} from "url";
import {ALL_REGRESSION_TESTS, selectRegressionTests} from "./select-ui-tests.js";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));

function expectSelection(name, changedFiles, expectedTests) {
  assert.deepStrictEqual(selectRegressionTests(changedFiles), expectedTests, name);
}

expectSelection(
  "worker node UI changes select worker node regression",
  ["web/src/components/shared/machine-node-deploy-sheet.jsx"],
  ["tests/ui/worker-node.spec.js", "tests/ui/worker-node-ready.spec.js"]
);

expectSelection(
  "worker node backend changes select worker node regression once",
  ["controllers/machine.go", "object/machine_node_deploy.go", "web/src/pages/MachineListPage.jsx"],
  ["tests/ui/worker-node.spec.js", "tests/ui/worker-node-ready.spec.js"]
);

expectSelection(
  "platform bootstrap changes select worker readiness regression",
  ["server/storage_bootstrap.go", "server/flannel_bootstrap.go", "deploy/node_bootstrap.go"],
  ["tests/ui/worker-node.spec.js", "tests/ui/worker-node-ready.spec.js"]
);

expectSelection(
  "site changes rely on fixed smoke coverage",
  ["web/src/pages/SiteEditPage.jsx"],
  []
);

expectSelection(
  "site list and backend changes select site regression",
  ["web/src/pages/SiteListPage.jsx", "web/src/backend/SiteBackend.js", "object/site.go"],
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
  ALL_REGRESSION_TESTS
);

expectSelection(
  "app store UI and access URL changes select app store regression",
  [
    "web/src/pages/AppStorePage.jsx",
    "web/src/pages/DeploymentListPage.jsx",
    "web/src/pages/ServiceListPage.jsx",
    "controllers/helm.go",
  ],
  ["tests/ui/app-store.spec.js"]
);

expectSelection(
  "helm install dialog changes select app store regression",
  ["web/src/components/shared/helm-install-dialog.jsx"],
  ["tests/ui/app-store.spec.js"]
);

expectSelection(
  "unknown frontend code changes run all regression tests",
  ["web/src/pages/PodListPage.jsx"],
  ALL_REGRESSION_TESTS
);

// The shared table, dialogs and primitives are what every screen is assembled
// from, so a change there cannot be scoped to one feature's specs.
expectSelection(
  "shared DataTable changes run all regression tests",
  ["web/src/components/shared/data-table.jsx"],
  ALL_REGRESSION_TESTS
);

expectSelection(
  "shadcn primitive changes run all regression tests",
  ["web/src/components/ui/button.jsx"],
  ALL_REGRESSION_TESTS
);

expectSelection(
  "route table changes run all regression tests",
  ["web/src/routes.jsx"],
  ALL_REGRESSION_TESTS
);

expectSelection(
  "static asset serving changes run all regression tests",
  ["routers/static_filter.go"],
  ALL_REGRESSION_TESTS
);

expectSelection(
  "UI selector script changes run all regression tests",
  ["web/scripts/select-ui-tests.js"],
  ALL_REGRESSION_TESTS
);

expectSelection(
  "non-array inputs fall back to all regression tests",
  null,
  ALL_REGRESSION_TESTS
);

const cliInputPath = path.join(scriptDir, `.select-ui-tests-${process.pid}.txt`);
fs.writeFileSync(cliInputPath, "web/src/components/shared/machine-node-deploy-sheet.jsx\n", "utf8");
try {
  const output = execFileSync(process.execPath, [path.join(scriptDir, "select-ui-tests.js"), cliInputPath], {
    encoding: "utf8",
  });
  assert.strictEqual(output, "tests/ui/worker-node.spec.js\ntests/ui/worker-node-ready.spec.js\n", "CLI prints selected regression tests");
} finally {
  fs.rmSync(cliInputPath, {force: true});
}

process.stdout.write(`select-ui-tests: all ${ALL_REGRESSION_TESTS.length} regression specs registered; selection checks passed\n`);
