const {randomUUID} = require("crypto");
const http = require("http");
const fs = require("fs");
const zlib = require("zlib");
const {expect, test} = require("@playwright/test");
const {expectOkJson, signInAsCiUser} = require("./e2e-helpers");

const API_ADD_HELM_REPO = "/api/add-helm-repo";
const API_DELETE_HELM_REPO = "/api/delete-helm-repo";
const API_DEPLOY_MACHINE_NODE = "/api/deploy-machine-node";
const API_DELETE_MACHINE = "/api/delete-machine";
const API_GET_HELM_REPOS = "/api/get-helm-repos";
const API_GET_HELM_RELEASES = "/api/get-helm-releases";
const API_GET_MACHINE_NODE_TASKS = "/api/get-machine-node-tasks";
const API_GET_NODES = "/api/get-nodes";
const API_UNINSTALL_HELM_RELEASE = "/api/uninstall-helm-release";
const CHART_NAME = "casos-access-url-fixture";
const CHART_VERSION = "0.1.0";
const NODE_PORT = Number(process.env.E2E_APP_STORE_NODE_PORT || 31080);
const NAMESPACE = "default";
const WORKER_READY_TIMEOUT_MS = Number(process.env.E2E_APP_STORE_WORKER_TIMEOUT_MS || 20 * 60 * 1000);
const HELM_READY_TIMEOUT_MS = Number(process.env.E2E_APP_STORE_HELM_TIMEOUT_MS || 3 * 60 * 1000);
const ACCESS_READY_TIMEOUT_MS = Number(process.env.E2E_APP_STORE_ACCESS_TIMEOUT_MS || 2 * 60 * 1000);
const E2E_APISERVER_URL = process.env.E2E_APISERVER_URL || "https://127.0.0.1:16443";
const E2E_MACHINE_OWNER = process.env.E2E_MACHINE_OWNER || "admin";
const SSH_HOST = process.env.E2E_APP_STORE_SSH_HOST || "127.0.0.1";
const SSH_PORT = Number(process.env.E2E_APP_STORE_SSH_PORT || 22);
const SSH_USER = process.env.E2E_APP_STORE_SSH_USER || "";
const SSH_PRIVATE_KEY = process.env.E2E_APP_STORE_SSH_PRIVATE_KEY ||
  (process.env.E2E_APP_STORE_SSH_KEY_PATH ? fs.readFileSync(process.env.E2E_APP_STORE_SSH_KEY_PATH, "utf8") : "");
const hasWorkerSshConfig = Boolean(SSH_USER && SSH_PRIVATE_KEY);
const RETRYABLE_INFRASTRUCTURE_PATTERNS = [
  /connection refused/i,
  /invalid connection/i,
  /dial tcp .*3306/i,
  /cluster not ready/i,
  /temporarily unavailable/i,
];

test.skip(!hasWorkerSshConfig && !process.env.CI, "requires E2E_APP_STORE_SSH_USER and SSH private key for a real worker node target");

test.beforeEach(async({page}) => {
  await signInAsCiUser(page);
});

test("installs a store chart and verifies the Access URL is reachable", async({page}) => {
  test.setTimeout(WORKER_READY_TIMEOUT_MS + HELM_READY_TIMEOUT_MS + ACCESS_READY_TIMEOUT_MS + 3 * 60 * 1000);
  expect(hasWorkerSshConfig, "CI must prepare SSH credentials for the app-store access test").toBeTruthy();
  expect(SSH_USER, "E2E_APP_STORE_SSH_USER must be set").toBeTruthy();
  expect(SSH_PRIVATE_KEY, "E2E_APP_STORE_SSH_PRIVATE_KEY or E2E_APP_STORE_SSH_KEY_PATH must be set").toBeTruthy();

  const suffix = makeResourceSuffix();
  const machineName = `astore-${suffix}`;
  const releaseName = `as-${suffix}`;
  const repoName = `repo-${suffix}-${randomUUID().slice(0, 6)}`;
  let repoServer;
  let repoId = null;
  let createdMachine = false;

  try {
    repoServer = await startHelmRepoServer();

    await cleanupHelmRelease(page, releaseName);
    await createMachineFromUi(page, machineName);
    createdMachine = true;

    const task = await startWorkerNodeDeployment(page, machineName);
    await waitForWorkerNodeReady(page, machineName, task.id);

    repoId = await addRepoFromAppStore(page, repoName, repoServer.url);
    await installChartFromAppStore(page, releaseName);
    await waitForHelmRelease(page, releaseName);

    const accessUrl = await readServiceAccessUrl(page, releaseName);
    expect(accessUrl).toContain(`:${NODE_PORT}`);
    await waitForAccessUrl(page, accessUrl);
  } finally {
    await cleanupHelmRelease(page, releaseName).catch(() => {});
    if (repoId) {
      await deleteHelmRepo(page, repoId).catch(() => {});
    }
    if (createdMachine) {
      await deleteMachine(page, machineName).catch(() => {});
    }
    await closeServer(repoServer?.server);
  }
});

function makeResourceSuffix() {
  const raw = process.env.GITHUB_RUN_ID || process.env.E2E_TEST_TOKEN || String(process.pid);
  const runPart = String(raw).replace(/[^a-z0-9-]/gi, "").toLowerCase().slice(-8) || "local";
  return `${runPart}-${randomUUID().slice(0, 8)}`;
}

async function createMachineFromUi(page, machineName) {
  await page.goto("/machines");
  await page.waitForLoadState("networkidle");
  await expect(page).toHaveURL(/\/machines$/);

  await machineTableTitle(page).getByRole("button", {name: "Add"}).click();
  const addDialog = page.getByRole("dialog", {name: "Add Machine"});
  await expect(addDialog).toBeVisible();
  await addDialog.getByPlaceholder("my-machine").fill(machineName);
  await addDialog.getByPlaceholder("My Machine").fill("App Store Access Worker");
  await addDialog.getByPlaceholder("192.168.1.10").fill(SSH_HOST);
  await addDialog.getByLabel(/SSH port/).fill(String(SSH_PORT));
  await addDialog.getByPlaceholder("root").fill(SSH_USER);
  const authTypeSelect = addDialog.getByRole("combobox", {name: "Auth type"});
  await authTypeSelect.click();
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  const privateKeyField = addDialog.getByLabel(/Private key/);
  await expect(privateKeyField).toBeVisible();
  await privateKeyField.fill(SSH_PRIVATE_KEY);

  const addMachine = page.waitForResponse(response =>
    response.url().includes("/api/add-machine") && response.request().method() === "POST"
  );
  await addDialog.getByRole("button", {name: "Add"}).click();
  await expectOkJson(await addMachine);
  await expect(addDialog).toBeHidden();
  await findMachineRow(page, machineName);
}

class RetryableInfrastructureError extends Error {
  constructor(message) {
    super(message);
    this.name = "RetryableInfrastructureError";
  }
}

function isRetryableInfrastructureMessage(message) {
  const text = String(message || "");
  return RETRYABLE_INFRASTRUCTURE_PATTERNS.some(pattern => pattern.test(text));
}

function isRetryableInfrastructureError(error) {
  return error instanceof RetryableInfrastructureError || isRetryableInfrastructureMessage(error?.message || error);
}

function isIgnorableCleanupMessage(message) {
  const text = String(message || "").toLowerCase();
  return text.includes("not found") || text.includes("not loaded") || text.includes("cluster not ready");
}

function responseErrorMessage(context, response) {
  return `${context}: HTTP ${response.status()}`;
}

async function readOkJson(response, context) {
  if (!response.ok()) {
    const message = responseErrorMessage(context, response);
    if (response.status() >= 500 || response.status() === 429 || response.status() === 408) {
      throw new RetryableInfrastructureError(message);
    }
    throw new Error(message);
  }

  let body;
  try {
    body = await response.json();
  } catch (error) {
    const message = `${context}: failed to parse JSON response: ${error.message}`;
    if (isRetryableInfrastructureError(error)) {
      throw new RetryableInfrastructureError(message);
    }
    throw new Error(message);
  }

  if (body.status !== "ok") {
    const msg = String(body.msg || "");
    const message = msg ? `${context}: ${msg}` : `${context}: unexpected response`;
    if (isRetryableInfrastructureError(message)) {
      throw new RetryableInfrastructureError(message);
    }
    throw new Error(message);
  }
  return body;
}

function machineTable(page) {
  return page.locator(".ant-table-wrapper").filter({hasText: "Machines"});
}

function machineTableTitle(page) {
  return machineTable(page).locator(".ant-table-title");
}

async function findMachineRow(page, machineName) {
  const row = machineTable(page).locator(`tr[data-row-key="${machineName}"]`);
  await expect(row).toBeVisible();
  return row;
}

function workerNodeDialog(page, machineName) {
  return page.getByRole("dialog", {name: `Worker Node - ${machineName}`});
}

async function startWorkerNodeDeployment(page, machineName) {
  const machineRow = await findMachineRow(page, machineName);
  await machineRow.getByRole("button", {name: "Deploy worker node"}).click();
  const dialog = workerNodeDialog(page, machineName);
  await expect(dialog).toBeVisible();
  await expect(dialog.getByLabel("Node name")).toHaveValue(machineName);
  await dialog.getByLabel("Apiserver URL").fill(E2E_APISERVER_URL);

  const deployMachineNode = page.waitForResponse(response =>
    response.url().includes(API_DEPLOY_MACHINE_NODE) && response.request().method() === "POST"
  );
  await dialog.getByRole("button", {name: "Deploy Node"}).click();
  const body = await expectOkJson(await deployMachineNode);
  expect(body.data).toMatchObject({
    machineName,
    nodeName: machineName,
    apiserverUrl: E2E_APISERVER_URL,
    status: "pending",
    phase: "queued",
  });
  await expect(page.locator(".ant-message").getByText("Node deployment started", {exact: true})).toBeVisible();
  return body.data;
}

async function waitForWorkerNodeReady(page, nodeName, taskId) {
  const started = Date.now();
  let lastTask;
  let lastRetryableError = "";
  while (Date.now() - started < WORKER_READY_TIMEOUT_MS) {
    try {
      const tasks = await getMachineNodeTasks(page, nodeName);
      lastTask = tasks.find(task => task.id === taskId) || tasks[0];
      if (lastTask?.status === "failed") {
        throw new Error(`worker node deployment failed during ${lastTask.phase}: ${lastTask.errorMsg || "unknown error"}`);
      }
      if (lastTask?.status === "succeeded" && lastTask.phase === "ready") {
        return waitForNodeSummary(page, nodeName);
      }
      lastRetryableError = "";
    } catch (error) {
      if (!isRetryableInfrastructureError(error)) {
        throw error;
      }
      lastRetryableError = error.message;
    }
    await page.waitForTimeout(5000);
  }
  throw new Error(`timed out waiting for worker node deployment; last task: ${JSON.stringify(lastTask || null)}${lastRetryableError ? `; last retryable error: ${lastRetryableError}` : ""}`);
}

async function getMachineNodeTasks(page, machineName) {
  const response = await page.context().request.get(
    `${API_GET_MACHINE_NODE_TASKS}?owner=${encodeURIComponent(E2E_MACHINE_OWNER)}&machineName=${encodeURIComponent(machineName)}`
  );
  const body = await readOkJson(response, "get-machine-node-tasks");
  return body.data || [];
}

async function getHelmReleases(page) {
  const response = await page.context().request.get(`${API_GET_HELM_RELEASES}?namespace=${NAMESPACE}`);
  const body = await readOkJson(response, "get-helm-releases");
  return body.data || [];
}

async function getNodeSummaries(page) {
  const response = await page.context().request.get(API_GET_NODES);
  const body = await readOkJson(response, "get-nodes");
  return body.data || [];
}

async function waitForNodeSummary(page, nodeName) {
  const started = Date.now();
  let lastNodes = [];
  let lastRetryableError = "";
  while (Date.now() - started < 60 * 1000) {
    try {
      lastNodes = await getNodeSummaries(page);
      const node = lastNodes.find(item => item.name === nodeName);
      if (node?.status === "Ready" && (node.internalIP || node.externalIP)) {
        return node;
      }
      lastRetryableError = "";
    } catch (error) {
      if (!isRetryableInfrastructureError(error)) {
        throw error;
      }
      lastRetryableError = error.message;
    }
    await page.waitForTimeout(3000);
  }
  throw new Error(`worker node ${nodeName} did not become visible with an IP; nodes: ${JSON.stringify(lastNodes)}${lastRetryableError ? `; last retryable error: ${lastRetryableError}` : ""}`);
}

async function addRepoFromAppStore(page, repoName, repoURL) {
  await page.goto("/app-store");
  await page.waitForLoadState("networkidle");
  await expect(page).toHaveURL(/\/app-store$/);
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
  const charts = page.waitForResponse(response =>
    response.url().includes("/api/get-repo-charts") && response.request().method() === "GET"
  );
  await page.getByText(repoName, {exact: true}).click();
  await expectOkJson(await charts);
  return repo.id || body.data?.id || null;
}

async function installChartFromAppStore(page, releaseName) {
  await expect(page.getByText(CHART_NAME, {exact: true})).toBeVisible();
  await page.getByRole("button", {name: "Install"}).click();

  const dialog = page.getByRole("dialog", {name: new RegExp(`Install chart\\s+${CHART_NAME}`)});
  await expect(dialog).toBeVisible();
  await dialog.getByLabel("Release name").fill(releaseName);
  await expect(dialog.locator("textarea")).toBeVisible({timeout: HELM_READY_TIMEOUT_MS});

  const installResponsePromise = page.waitForResponse(response =>
    response.url().includes("/api/install-helm-chart-stream") && response.request().method() === "POST"
  );
  await dialog.getByRole("button", {name: "Install"}).click();
  const installResponse = await installResponsePromise;
  expect(installResponse.ok()).toBeTruthy();
  await expect(dialog).toBeHidden({timeout: HELM_READY_TIMEOUT_MS});
}

async function waitForHelmRelease(page, releaseName) {
  const started = Date.now();
  let lastReleases = [];
  let lastRetryableError = "";
  while (Date.now() - started < HELM_READY_TIMEOUT_MS) {
    try {
      lastReleases = await getHelmReleases(page);
      const release = lastReleases.find(item => item.name === releaseName);
      if (release?.status === "deployed") {
        return release;
      }
      lastRetryableError = "";
    } catch (error) {
      if (!isRetryableInfrastructureError(error)) {
        throw error;
      }
      lastRetryableError = error.message;
    }
    await page.waitForTimeout(3000);
  }
  throw new Error(`release ${releaseName} was not deployed; releases: ${JSON.stringify(lastReleases)}${lastRetryableError ? `; last retryable error: ${lastRetryableError}` : ""}`);
}

async function readServiceAccessUrl(page, releaseName) {
  await page.goto("/services");
  await page.waitForLoadState("networkidle");
  const row = page.locator(`tr[data-row-key="${NAMESPACE}/${releaseName}"]`);
  await expect(row).toBeVisible({timeout: 60 * 1000});
  const accessLink = row.getByRole("link", {name: new RegExp(`:${NODE_PORT}$`)});
  await expect(accessLink).toBeVisible();
  return accessLink.getAttribute("href");
}

async function waitForAccessUrl(page, accessUrl) {
  const started = Date.now();
  let lastError = "";
  while (Date.now() - started < ACCESS_READY_TIMEOUT_MS) {
    try {
      const response = await page.goto(accessUrl, {timeout: 10000, waitUntil: "domcontentloaded"});
      const body = (await page.locator("body").textContent()) || "";
      if (response && response.status() === 200 && body.trim().length > 0) {
        return;
      }
      lastError = `HTTP ${response?.status() || "no response"}: ${body.slice(0, 120)}`;
    } catch (error) {
      lastError = error.message;
    }
    await page.waitForTimeout(3000);
  }
  throw new Error(`Access URL ${accessUrl} was not reachable: ${lastError}`);
}

async function cleanupHelmRelease(page, releaseName) {
  const started = Date.now();
  let lastError = "";
  while (Date.now() - started < 15 * 1000) {
    try {
      const response = await page.context().request.post(API_UNINSTALL_HELM_RELEASE, {
        data: {releaseName, namespace: NAMESPACE},
      });
      if (!response.ok()) {
        const message = responseErrorMessage(`uninstall release ${releaseName}`, response);
        if (response.status() >= 500 || response.status() === 429 || response.status() === 408) {
          throw new RetryableInfrastructureError(message);
        }
        console.warn(`Skipping Helm release cleanup for ${releaseName}: ${message}`);
        return;
      }
      const body = await response.json();
      const msg = String(body.msg || "").toLowerCase();
      if (body.status === "ok" || isIgnorableCleanupMessage(msg)) {
        return;
      }
      const message = `failed to uninstall release ${releaseName}: ${body.msg}`;
      if (isRetryableInfrastructureError(message)) {
        throw new RetryableInfrastructureError(message);
      }
      console.warn(`Skipping Helm release cleanup for ${releaseName}: ${message}`);
      return;
    } catch (error) {
      if (!isRetryableInfrastructureError(error)) {
        console.warn(`Skipping Helm release cleanup for ${releaseName}: ${error.message}`);
        return;
      }
      lastError = error.message;
      await page.waitForTimeout(2000);
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

async function deleteMachine(page, machineName) {
  const response = await page.context().request.post(API_DELETE_MACHINE, {
    data: {owner: E2E_MACHINE_OWNER, name: machineName},
  });
  await expectOkJson(response);
}

async function startHelmRepoServer() {
  const chartFile = `${CHART_NAME}-${CHART_VERSION}.tgz`;
  let chartURL = "";
  const chartArchive = createChartArchive();
  const server = http.createServer((req, res) => {
    if (req.url === "/index.yaml") {
      res.writeHead(200, {"Content-Type": "application/x-yaml"});
      res.end(createIndexYaml(chartURL));
      return;
    }
    if (req.url === `/${chartFile}`) {
      res.writeHead(200, {"Content-Type": "application/gzip"});
      res.end(chartArchive);
      return;
    }
    res.writeHead(404, {"Content-Type": "text/plain"});
    res.end("not found");
  });
  await listen(server, "127.0.0.1", 0);
  const url = `http://127.0.0.1:${server.address().port}`;
  chartURL = `${url}/${chartFile}`;
  return {server, url};
}

function listen(server, host, port) {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(port, host, () => {
      server.off("error", reject);
      resolve();
    });
  });
}

function closeServer(server) {
  if (!server) {
    return Promise.resolve();
  }
  return new Promise(resolve => server.close(resolve));
}

function createIndexYaml(chartURL) {
  return [
    "apiVersion: v1",
    "entries:",
    `  ${CHART_NAME}:`,
    "    - apiVersion: v2",
    `      name: ${CHART_NAME}`,
    `      version: ${CHART_VERSION}`,
    "      appVersion: \"1.0.0\"",
    "      description: CasOS app store Access URL fixture",
    "      type: application",
    "      urls:",
    `        - ${chartURL}`,
    "generated: \"2026-01-01T00:00:00Z\"",
    "",
  ].join("\n");
}

function createChartArchive() {
  const root = `${CHART_NAME}/`;
  return zlib.gzipSync(createTarArchive({
    [`${root}Chart.yaml`]: [
      "apiVersion: v2",
      `name: ${CHART_NAME}`,
      "description: CasOS app store Access URL fixture",
      "type: application",
      `version: ${CHART_VERSION}`,
      "appVersion: \"1.0.0\"",
      "",
    ].join("\n"),
    [`${root}values.yaml`]: [
      "image: registry.k8s.io/e2e-test-images/agnhost:2.40",
      "containerPort: 8080",
      `nodePort: ${NODE_PORT}`,
      "",
    ].join("\n"),
    [`${root}templates/deployment.yaml`]: [
      "apiVersion: apps/v1",
      "kind: Deployment",
      "metadata:",
      "  name: {{ .Release.Name }}",
      "  labels:",
      "    app: {{ .Release.Name }}",
      "spec:",
      "  replicas: 1",
      "  selector:",
      "    matchLabels:",
      "      app: {{ .Release.Name }}",
      "  template:",
      "    metadata:",
      "      labels:",
      "        app: {{ .Release.Name }}",
      "    spec:",
      "      containers:",
      "        - name: access-fixture",
      "          image: {{ .Values.image }}",
      "          command:",
      "            - /agnhost",
      "          args:",
      "            - netexec",
      "            - --http-port={{ .Values.containerPort }}",
      "            - --udp-port=-1",
      "          ports:",
      "            - name: http",
      "              containerPort: {{ .Values.containerPort }}",
      "",
    ].join("\n"),
    [`${root}templates/service.yaml`]: [
      "apiVersion: v1",
      "kind: Service",
      "metadata:",
      "  name: {{ .Release.Name }}",
      "spec:",
      "  type: NodePort",
      "  selector:",
      "    app: {{ .Release.Name }}",
      "  ports:",
      "    - name: http",
      "      protocol: TCP",
      "      port: 80",
      "      targetPort: {{ .Values.containerPort }}",
      "      nodePort: {{ .Values.nodePort }}",
      "",
    ].join("\n"),
  }));
}

function createTarArchive(files) {
  const chunks = [];
  for (const [name, content] of Object.entries(files)) {
    const data = Buffer.from(content, "utf8");
    chunks.push(createTarHeader(name, data.length));
    chunks.push(data);
    const padding = (512 - (data.length % 512)) % 512;
    if (padding > 0) {
      chunks.push(Buffer.alloc(padding));
    }
  }
  chunks.push(Buffer.alloc(1024));
  return Buffer.concat(chunks);
}

function createTarHeader(name, size) {
  const header = Buffer.alloc(512, 0);
  writeTarString(header, 0, name, 100);
  writeTarOctal(header, 100, 0o644, 8);
  writeTarOctal(header, 108, 0, 8);
  writeTarOctal(header, 116, 0, 8);
  writeTarOctal(header, 124, size, 12);
  writeTarOctal(header, 136, 0, 12);
  header.fill(0x20, 148, 156);
  header[156] = "0".charCodeAt(0);
  writeTarString(header, 257, "ustar", 6);
  writeTarString(header, 263, "00", 2);
  let checksum = 0;
  for (const byte of header) {
    checksum += byte;
  }
  writeTarString(header, 148, checksum.toString(8).padStart(6, "0") + "\0 ", 8);
  return header;
}

function writeTarString(header, offset, value, length) {
  Buffer.from(value).copy(header, offset, 0, Math.min(Buffer.byteLength(value), length));
}

function writeTarOctal(header, offset, value, length) {
  writeTarString(header, offset, value.toString(8).padStart(length - 1, "0") + "\0", length);
}
