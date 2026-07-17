const {expect, test} = require("@playwright/test");
const {expectOkJson, signInAsCiUser} = require("./e2e-helpers");
const {
  INSTALLABLE_CASES,
  PRESET_REPO_URLS,
  assertCatalogSnapshot,
} = require("./app-store-catalog-cases");
const {
  installAppFromAppStore,
  installedReleasesFixture,
  makeReleaseName,
} = require("./app-store-helpers");
const {
  createMachineFromUi,
  makeMachineName,
  startWorkerNodeDeployment,
  workerNodeDialog,
  workerNodeTaskTable,
} = require("./worker-node-helpers");

const RUN_FULL_MATRIX = process.env.E2E_FULL_APP_STORE_MATRIX === "1";
const E2E_WORKER_VM_IP = process.env.E2E_WORKER_VM_IP;
const E2E_APISERVER_URL = process.env.E2E_APISERVER_URL || "https://127.0.0.1:16443";
const REPO_FILTER = process.env.E2E_APP_MATRIX_REPO;
const CHART_FILTER = process.env.E2E_APP_MATRIX_CHART;
const START_AT = process.env.E2E_APP_MATRIX_START;
const LIMIT = Number(process.env.E2E_APP_MATRIX_LIMIT || 0);
const INSTALL_TIMEOUT_MS = Number(process.env.E2E_APP_INSTALL_TIMEOUT_MS) || 15 * 60 * 1000;

function selectedCases() {
  let cases = INSTALLABLE_CASES.filter(({repo, chart}) =>
    (!REPO_FILTER || repo === REPO_FILTER) &&
    (!CHART_FILTER || chart === CHART_FILTER) &&
    (!START_AT || `${repo}/${chart}` >= START_AT)
  );
  if (LIMIT > 0) {
    cases = cases.slice(0, LIMIT);
  }
  return cases;
}

function releasePrefix(chart) {
  return `matrix-${chart}`.slice(0, 40).replace(/-+$/, "");
}

const appMatrixTest = test.extend({installedReleases: installedReleasesFixture});
appMatrixTest.describe.configure({mode: "serial", retries: 0});

appMatrixTest.describe("full App Store install matrix", () => {
  appMatrixTest.skip(!RUN_FULL_MATRIX, "set E2E_FULL_APP_STORE_MATRIX=1 to run the full install matrix");
  appMatrixTest.skip(!E2E_WORKER_VM_IP, "E2E_WORKER_VM_IP is required for install cases");

  let machineName;
  let setupContext;

  appMatrixTest.beforeAll(async({browser}) => {
    appMatrixTest.setTimeout(12 * 60 * 1000);
    setupContext = await browser.newContext({baseURL: test.info().project.use.baseURL});
    const page = await setupContext.newPage();
    await signInAsCiUser(page);
    const currentCatalog = {};
    for (const [repo, url] of Object.entries(PRESET_REPO_URLS)) {
      const response = await setupContext.request.get("/api/get-repo-charts", {params: {url}});
      const body = await expectOkJson(response);
      currentCatalog[repo] = (body.data ?? []).map(chart => chart.name);
    }
    assertCatalogSnapshot(currentCatalog);
    machineName = makeMachineName("app-store-full-matrix");
    await createMachineFromUi(page, machineName, [], {ip: E2E_WORKER_VM_IP, username: "root"});
    const task = await startWorkerNodeDeployment(page, machineName, E2E_APISERVER_URL);
    const taskRow = workerNodeTaskTable(page, machineName).locator(`tr[data-row-key="${task.id}"]`);
    await expect(taskRow.getByRole("cell", {name: "succeeded", exact: true})).toBeVisible({timeout: 8 * 60 * 1000});
    await workerNodeDialog(page, machineName).getByRole("button", {name: "Close"}).click();
    await page.close();
  });

  appMatrixTest.afterAll(async() => {
    if (setupContext) {
      if (machineName) {
        await setupContext.request.post("/api/delete-machine", {data: {owner: "admin", name: machineName}});
      }
      await setupContext.close();
    }
  });

  appMatrixTest.beforeEach(async({page}) => {
    await signInAsCiUser(page);
  });

  for (const {repo, chart} of selectedCases()) {
    appMatrixTest(`${repo}/${chart} installs with App Store values`, async({page, installedReleases}) => {
      appMatrixTest.setTimeout(INSTALL_TIMEOUT_MS);
      await installAppFromAppStore(page, {
        repoName: repo,
        chartName: chart,
        releaseName: makeReleaseName(releasePrefix(chart)),
        installedReleases,
      });
    });
  }
});
