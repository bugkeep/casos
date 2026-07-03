const {test} = require("@playwright/test");
const {signInAsCiUser} = require("./e2e-helpers");
const {
  ACCESS_URL_FAILURE_CATEGORIES,
  formatAccessUrlFailure,
  shouldFailAccessUrlOutcome,
} = require("./access-url-diagnostics");
const {
  ACCESS_EXPERIMENT_TIMEOUT_MS,
  ACCESS_READY_TIMEOUT_MS,
  E2E_APISERVER_URL,
  HELM_READY_TIMEOUT_MS,
  MACHINE_CREATE_TIMEOUT_MS,
  SSH_HOST,
  SSH_PRIVATE_KEY,
  SSH_USER,
  WORKER_READY_TIMEOUT_MS,
  hasWorkerSshConfig,
} = require("./app-store-access-url-config");
const {
  REPRESENTATIVE_APP_STORE_REPOS,
  flattenRepresentativeCharts,
} = require("./app-store-access-url-samples");
const {
  compactAccessOutcome,
  compactAppStoreChart,
  createAppStoreStageTracker,
  makeResourceSuffix,
} = require("./app-store-access-url-log");
const {
  addRepoFromAppStore,
  cleanupHelmRelease,
  deleteHelmRepo,
  installChartFromAppStore,
  waitForHelmRelease,
} = require("./app-store-access-url-helm");
const {probeReleaseAccessUrl} = require("./app-store-access-url-probe");
const {
  attachAnnotatedAccessUrlFailureScreenshot,
  showAccessUrlFailureOverlay,
} = require("./app-store-access-url-visual-evidence");
const {
  diagnosticOutcome,
  writeAccessUrlSummary,
} = require("../../scripts/app-store-access-url-summary");
const {
  createMachineFromUi,
  deleteMachine,
  resetWorkerDiagnosticsLog,
  startWorkerNodeDeployment,
  waitForWorkerNodeReady,
} = require("./app-store-access-url-worker");

test.skip(!hasWorkerSshConfig && !process.env.CI, "requires E2E_APP_STORE_SSH_USER and SSH private key for a real worker node target");

test.beforeEach(async({page}) => {
  await signInAsCiUser(page);
});

test("samples app-store charts and classifies Access URL reachability", async({page}) => {
  const selectedCharts = flattenRepresentativeCharts();
  const expectedFailureCases = selectedCharts.filter(chart => !chart.expectReachable).length;
  const expectedReachableCases = selectedCharts.filter(chart => chart.expectReachable).length;
  test.setTimeout(
    WORKER_READY_TIMEOUT_MS +
    MACHINE_CREATE_TIMEOUT_MS +
    HELM_READY_TIMEOUT_MS * selectedCharts.length +
    ACCESS_READY_TIMEOUT_MS * expectedReachableCases +
    ACCESS_EXPERIMENT_TIMEOUT_MS * expectedFailureCases +
    4 * 60 * 1000
  );
  const suffix = makeResourceSuffix();
  const machineName = `astore-${suffix}`;
  const repoSpecs = REPRESENTATIVE_APP_STORE_REPOS.map(repo => ({
    ...repo,
    repoName: `${repo.repoNamePrefix}-${suffix}`,
  }));
  const releaseNames = selectedCharts.map(chart => `${chart.releasePrefix}-${suffix}`);
  const repoIds = [];
  let createdMachine = false;
  const stages = createAppStoreStageTracker({machineName});
  const releaseContexts = [];
  const outcomes = [];
  let status = "completed";
  let failedAccessUrlOutcomes = [];
  await resetWorkerDiagnosticsLog();

  try {
    stages.log("test-start", {sshHost: SSH_HOST, apiserverUrl: E2E_APISERVER_URL});
    if (!hasWorkerSshConfig || !SSH_USER || !SSH_PRIVATE_KEY) {
      status = "diagnostic-precondition";
      outcomes.push(diagnosticOutcome(
        ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC,
        "CI did not provide SSH credentials for the App Store Access worker precondition",
        {source: "app-store-access-precondition"}
      ));
      return;
    }
    stages.log("app-store-representative-samples-selected", {
      repos: repoSpecs.map(repo => ({
        key: repo.key,
        repoName: repo.repoName,
        repoURL: repo.repoURL,
        charts: repo.charts.map(compactAppStoreChart),
      })),
    });

    await stages.run("pre-cleanup-helm-releases", async() => {
      for (const releaseName of releaseNames) {
        await cleanupHelmRelease(page, releaseName);
      }
    }, {releaseNames});
    await stages.run("create-machine", async() => {
      await createMachineFromUi(page, machineName);
    }, {}, {timeoutMs: MACHINE_CREATE_TIMEOUT_MS});
    createdMachine = true;

    const task = await stages.run("start-worker-deployment", () => startWorkerNodeDeployment(page, machineName));
    await stages.run("wait-worker-ready", () => waitForWorkerNodeReady(page, machineName, task.id));

    for (const repoSpec of repoSpecs) {
      const appStoreRepo = await stages.run(
        "add-repo-from-app-store",
        () => addRepoFromAppStore(page, repoSpec.repoName, repoSpec.repoURL, repoSpec.charts),
        {repoName: repoSpec.repoName, repoURL: repoSpec.repoURL}
      );
      if (appStoreRepo.repoId) {
        repoIds.push(appStoreRepo.repoId);
      }
      stages.log("app-store-charts-selected", {
        repoName: repoSpec.repoName,
        charts: appStoreRepo.charts.map(compactAppStoreChart),
      });

      for (const appStoreChart of appStoreRepo.charts) {
        const releaseName = `${appStoreChart.releasePrefix}-${suffix}`;
        const releaseContext = await stages.run(
          "install-chart-from-app-store",
          () => installChartFromAppStore(page, releaseName, appStoreChart),
          {repoName: repoSpec.repoName, releaseName, chartName: appStoreChart.chartName}
        );
        releaseContexts.push(releaseContext);
        await stages.run("wait-helm-release", () => waitForHelmRelease(page, releaseContext), {
          releaseName,
          chartName: appStoreChart.chartName,
        });
      }
    }

    for (const releaseContext of releaseContexts) {
      const timeoutMs = releaseContext.expectReachable ? ACCESS_READY_TIMEOUT_MS : ACCESS_EXPERIMENT_TIMEOUT_MS;
      const outcome = await stages.run(
        "probe-access-url",
        () => probeReleaseAccessUrl(page, releaseContext, {timeoutMs}),
        {releaseName: releaseContext.releaseName, chartName: releaseContext.chartName, timeoutMs}
      );
      outcomes.push(outcome);
      stages.log("access-url-case-result", compactAccessOutcome(outcome));
    }

  } catch (error) {
    status = "diagnostic-error";
    stages.log("access-url-experiment-diagnostic-error", {error: error.message || String(error)});
    outcomes.push(diagnosticOutcome(
      ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC,
      `App Store Access URL experiment stopped before product gate: ${error.message || String(error)}`,
      {source: "app-store-access-experiment"}
    ));
  } finally {
    await writeAccessUrlSummary(outcomes, {status})
      .then(payload => {
        stages.log("access-url-classification-summary", payload.summary);
      })
      .catch(error => {
        stages.log("access-url-summary-write-fail", {error: error.message});
      });
    failedAccessUrlOutcomes = outcomes.filter(shouldFailAccessUrlOutcome);
    if (failedAccessUrlOutcomes.length > 0) {
      await attachAnnotatedAccessUrlFailureScreenshot(page, failedAccessUrlOutcomes, test.info())
        .catch(error => {
          stages.log("access-url-failure-annotation-fail", {error: error.message});
        });
    }
    await stages.run("cleanup-helm-releases", async() => {
      for (const releaseName of [...releaseNames].reverse()) {
        try {
          await cleanupHelmRelease(page, releaseName);
        } catch (error) {
          stages.log("cleanup-helm-release-fail", {releaseName, error: error.message});
        }
      }
    }).catch(() => {});
    for (const repoId of [...repoIds].reverse()) {
      await stages.run("delete-helm-repo", () => deleteHelmRepo(page, repoId), {repoId}).catch(() => {});
    }
    if (createdMachine) {
      await stages.run("delete-machine", () => deleteMachine(page, machineName)).catch(() => {});
    }
    if (failedAccessUrlOutcomes.length > 0) {
      await showAccessUrlFailureOverlay(page, failedAccessUrlOutcomes)
        .catch(error => {
          stages.log("access-url-final-overlay-fail", {error: error.message});
        });
    }
    stages.finish();
  }

  if (failedAccessUrlOutcomes.length > 0) {
    throw new Error(
      [
        `${failedAccessUrlOutcomes.length} rendered App Store Access URL sample(s) could not be opened`,
        ...failedAccessUrlOutcomes.map(outcome => formatAccessUrlFailure(outcome)),
      ].join("\n")
    );
  }
});
