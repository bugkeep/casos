const {test} = require("@playwright/test");
const {signInAsCiUser} = require("./e2e-helpers");
const {
  addCustomHelmRepo,
  addedHelmReposFixture,
  installAppFromAppStore,
  installedReleasesFixture,
  makeReleaseName,
  makeRepoName,
} = require("./app-store-helpers");

const appStoreTest = test.extend({
  installedReleases: installedReleasesFixture,
  addedHelmRepos: addedHelmReposFixture,
});

const E2E_NAMESPACE = "default";
const APP_STORE_INSTALL_TIMEOUT_MS = Number(process.env.E2E_APP_INSTALL_TIMEOUT_MS) || 3 * 60 * 1000;
// Official OCI-hosted chart for Casdoor, the identity/SSO provider this project itself
// authenticates against (see controllers/e2e.go's casdoorsdk usage).
const CASDOOR_OCI_REPO_URL = "oci://registry-1.docker.io/casbin/casdoor-helm-charts";
const CASDOOR_CHART_NAME = "casdoor-helm-charts";

function installValues(releaseName) {
  return `fullnameOverride: ${releaseName}\n`;
}

appStoreTest.beforeEach(async({page}) => {
  await signInAsCiUser(page);
});

appStoreTest("installs nginx from the App Store", async({page, installedReleases}) => {
  appStoreTest.setTimeout(APP_STORE_INSTALL_TIMEOUT_MS);
  const releaseName = makeReleaseName("e2e-nginx");

  await installAppFromAppStore(page, {
    repoName: "Bitnami",
    chartName: "nginx",
    releaseName,
    namespace: E2E_NAMESPACE,
    valuesYAML: installValues(releaseName),
    installedReleases,
  });
});

appStoreTest("installs Casdoor from an OCI App Store repo", async({page, installedReleases, addedHelmRepos}) => {
  appStoreTest.setTimeout(APP_STORE_INSTALL_TIMEOUT_MS);
  const releaseName = makeReleaseName("e2e-casdoor");
  const repoName = makeRepoName("casdoor");

  await addCustomHelmRepo(page, {
    name: repoName,
    url: CASDOOR_OCI_REPO_URL,
    addedHelmRepos,
  });

  await installAppFromAppStore(page, {
    repoName,
    chartName: CASDOOR_CHART_NAME,
    releaseName,
    namespace: E2E_NAMESPACE,
    valuesYAML: installValues(releaseName),
    installedReleases,
  });
});
