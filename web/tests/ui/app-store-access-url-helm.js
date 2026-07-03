const {expect} = require("@playwright/test");
const {expectOkJson} = require("./e2e-helpers");
const {
  API_ADD_HELM_REPO,
  API_DELETE_HELM_REPO,
  API_GET_HELM_REPOS,
  API_GET_HELM_RELEASES,
  API_GET_REPO_CHARTS,
  API_INSTALL_HELM_CHART_STREAM,
  API_UNINSTALL_HELM_RELEASE,
  APP_STORE_ROUTE,
  APP_STORE_SOURCE,
  HELM_READY_TIMEOUT_MS,
  LOG_PROGRESS_INTERVAL_MS,
  NAMESPACE,
} = require("./app-store-access-url-config");
const {
  compactRelease,
  logAppStoreProgress,
} = require("./app-store-access-url-log");
const {
  RetryableInfrastructureError,
  isIgnorableCleanupMessage,
  isRetryableHttpStatus,
  isRetryableInfrastructureError,
  readOkJson,
  responseErrorMessage,
} = require("./app-store-access-url-http");
const {
  escapeRegExp,
  routeUrlPattern,
} = require("./app-store-access-url-routes");

const HELM_RELEASE_POLL_INTERVAL_MS = 3000;
const HELM_CLEANUP_TIMEOUT_MS = 15 * 1000;
const HELM_CLEANUP_POLL_INTERVAL_MS = 2000;

async function getHelmReleases(page) {
  const response = await page.context().request.get(`${API_GET_HELM_RELEASES}?namespace=${NAMESPACE}`);
  const body = await readOkJson(response, "get-helm-releases");
  return body.data || [];
}

async function addRepoFromAppStore(page, repoName, repoURL, expectedCharts = []) {
  await page.goto(APP_STORE_ROUTE);
  await page.waitForLoadState("networkidle");
  await expect(page).toHaveURL(routeUrlPattern(APP_STORE_ROUTE));
  await page.getByRole("button", {name: "Add Repo"}).click();
  const dialog = page.getByRole("dialog", {name: "Add Helm Repo"});
  await expect(dialog).toBeVisible();
  await dialog.getByLabel(/Repo name/).fill(repoName);
  await dialog.getByLabel(/Repo URL/).fill(repoURL);

  const addRepo = page.waitForResponse(response =>
    response.url().includes(API_ADD_HELM_REPO) && response.request().method() === "POST"
  );
  await dialog.getByRole("button", {name: "OK"}).click();
  const body = await expectOkJson(await addRepo);
  await expect(dialog).toBeHidden();

  const repos = await page.context().request.get(API_GET_HELM_REPOS);
  const reposBody = await expectOkJson(repos);
  const repo = (reposBody.data || []).find(item => item.name === repoName);
  expect(repo, `repo ${repoName} should be persisted`).toBeTruthy();
  await expect(page.getByText(repoName, {exact: true})).toBeVisible();
  const charts = page.waitForResponse(
    response => response.url().includes(API_GET_REPO_CHARTS) && response.request().method() === "GET",
    {timeout: HELM_READY_TIMEOUT_MS}
  );
  await page.getByText(repoName, {exact: true}).click();
  const chartsBody = await expectOkJson(await charts);
  const listedCharts = expectedCharts.map(expectedChart => {
    const chart = (chartsBody.data || []).find(item =>
      item.name === expectedChart.chartName &&
      (!expectedChart.chartVersion || item.version === expectedChart.chartVersion)
    );
    const chartLabel = expectedChart.chartVersion
      ? `${expectedChart.chartName}@${expectedChart.chartVersion}`
      : expectedChart.chartName;
    expect(
      chart,
      `${chartLabel} must be listed in the app store repo before installation`
    ).toBeTruthy();
    return {
      ...expectedChart,
      source: APP_STORE_SOURCE,
      repoId: repo.id || body.data?.id || null,
      repoName,
      repoURL,
      chartName: chart.name,
      chartVersion: chart.version,
      namespace: NAMESPACE,
    };
  });
  return {
    repoId: repo.id || body.data?.id || null,
    repoName,
    repoURL,
    charts: listedCharts,
  };
}

async function installChartFromAppStore(page, releaseName, appStoreChart) {
  expect(appStoreChart.source).toBe(APP_STORE_SOURCE);
  expect(appStoreChart.chartName).toBeTruthy();
  expect(appStoreChart.chartVersion).toBeTruthy();
  const chartCard = page.locator(".ant-card-hoverable")
    .filter({hasText: appStoreChart.chartName})
    .filter({hasText: appStoreChart.chartVersion})
    .filter({has: page.getByRole("button", {name: "Install"})});
  await expect(chartCard, "the chart must be uniquely selected from the App Store chart list").toHaveCount(1);
  await expect(chartCard, "the chart must be selected from the App Store chart list").toBeVisible();
  await chartCard.getByRole("button", {name: "Install"}).click();

  const dialog = page.getByRole("dialog", {name: new RegExp(`Install chart\\s+${escapeRegExp(appStoreChart.chartName)}`)});
  await expect(dialog).toBeVisible();
  await expect(dialog.getByText(appStoreChart.repoURL, {exact: true})).toBeVisible();
  await dialog.getByLabel("Release name").fill(releaseName);
  const valuesTextarea = dialog.locator("textarea");
  await expect(valuesTextarea).toBeVisible({timeout: HELM_READY_TIMEOUT_MS});
  const valuesYAML = resolveChartValuesYAML(appStoreChart, releaseName);
  if (valuesYAML) {
    await valuesTextarea.fill(valuesYAML);
  }

  const installResponsePromise = page.waitForResponse(response =>
    response.url().includes(API_INSTALL_HELM_CHART_STREAM) && response.request().method() === "POST"
  );
  await dialog.getByRole("button", {name: "Install"}).click();
  const installResponse = await installResponsePromise;
  expect(installResponse.ok()).toBeTruthy();
  await expect(dialog).toBeHidden({timeout: HELM_READY_TIMEOUT_MS});
  return {...appStoreChart, releaseName};
}

function resolveChartValuesYAML(appStoreChart, releaseName) {
  if (typeof appStoreChart.valuesYAML === "function") {
    return appStoreChart.valuesYAML({releaseName, appStoreChart});
  }
  return appStoreChart.valuesYAML;
}

async function waitForHelmRelease(page, releaseContext) {
  const started = Date.now();
  let lastReleases = [];
  let lastRetryableError = "";
  let lastProgressLogAt = 0;
  while (Date.now() - started < HELM_READY_TIMEOUT_MS) {
    try {
      lastReleases = await getHelmReleases(page);
      const release = lastReleases.find(item => item.name === releaseContext.releaseName);
      if (release?.status === "deployed") {
        expect(release).toMatchObject({
          namespace: releaseContext.namespace,
          chart: `${releaseContext.chartName}-${releaseContext.chartVersion}`,
        });
        return release;
      }
      lastRetryableError = "";
    } catch (error) {
      if (!isRetryableInfrastructureError(error)) {
        throw error;
      }
      lastRetryableError = error.message;
    }
    const now = Date.now();
    if (now - lastProgressLogAt >= LOG_PROGRESS_INTERVAL_MS) {
      lastProgressLogAt = now;
      logAppStoreProgress("wait-helm-release", started, {
        release: compactRelease(lastReleases.find(item => item.name === releaseContext.releaseName)),
        lastRetryableError,
      });
    }
    await page.waitForTimeout(HELM_RELEASE_POLL_INTERVAL_MS);
  }
  throw new Error(`release ${releaseContext.namespace}/${releaseContext.releaseName} from ${releaseContext.source} chart ${releaseContext.chartName}@${releaseContext.chartVersion} was not deployed; releases: ${JSON.stringify(lastReleases)}${lastRetryableError ? `; last retryable error: ${lastRetryableError}` : ""}`);
}

async function cleanupHelmRelease(page, releaseName) {
  const started = Date.now();
  let lastError = "";
  while (Date.now() - started < HELM_CLEANUP_TIMEOUT_MS) {
    try {
      const response = await page.context().request.post(API_UNINSTALL_HELM_RELEASE, {
        data: {releaseName, namespace: NAMESPACE},
      });
      if (!response.ok()) {
        const message = responseErrorMessage(`uninstall release ${releaseName}`, response);
        if (response.status() === 404) {
          return;
        }
        if (isRetryableHttpStatus(response.status())) {
          throw new RetryableInfrastructureError(message);
        }
        throw new Error(message);
      }
      try {
        await readOkJson(response, `uninstall release ${releaseName}`);
        return;
      } catch (error) {
        if (isIgnorableCleanupMessage(error.message)) {
          return;
        }
        if (isRetryableInfrastructureError(error)) {
          throw error;
        }
        throw error;
      }
    } catch (error) {
      if (!isRetryableInfrastructureError(error)) {
        if (isIgnorableCleanupMessage(error.message)) {
          return;
        }
        throw error;
      }
      lastError = error.message;
      await page.waitForTimeout(HELM_CLEANUP_POLL_INTERVAL_MS);
    }
  }
  if (lastError) {
    console.warn(`Skipping Helm release cleanup for ${releaseName}: ${lastError}`);
  }
}

async function deleteHelmRepo(page, repoId) {
  if (!repoId) {
    return;
  }
  const response = await page.context().request.post(`${API_DELETE_HELM_REPO}?id=${repoId}`);
  await expectOkJson(response);
}

module.exports = {
  addRepoFromAppStore,
  cleanupHelmRelease,
  deleteHelmRepo,
  installChartFromAppStore,
  waitForHelmRelease,
};
