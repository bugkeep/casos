const fs = require("fs");
const {expect} = require("@playwright/test");

const API_ADD_MACHINE = "/api/add-machine";
const API_DELETE_MACHINE = "/api/delete-machine";
const API_DEPLOY_MACHINE_NODE = "/api/deploy-machine-node";
const API_GET_MACHINE_NODE_LOGS = "/api/get-machine-node-logs";
const API_GET_MACHINE_NODE_TASKS = "/api/get-machine-node-tasks";
const API_GET_METRICS = "/api/get-metrics";
const API_GET_NODES = "/api/get-nodes";

const E2E_APISERVER_URL = process.env.E2E_APISERVER_URL || "https://127.0.0.1:16443";
const E2E_MACHINE_OWNER = process.env.E2E_MACHINE_OWNER || "admin";

function toPositiveInt(value, defaultValue) {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : defaultValue;
}

const REAL_WORKER_TIMEOUT_MS = toPositiveInt(process.env.E2E_REAL_WORKER_TIMEOUT_MS, 10 * 60 * 1000);
const REAL_WORKER_METRICS_TIMEOUT_MS = toPositiveInt(process.env.E2E_REAL_WORKER_METRICS_TIMEOUT_MS, 90 * 1000);
const REAL_WORKER_POLL_MS = toPositiveInt(process.env.E2E_REAL_WORKER_POLL_MS, 5000);

function readPrivateKey() {
  if (process.env.E2E_REAL_WORKER_SSH_PRIVATE_KEY) {
    return process.env.E2E_REAL_WORKER_SSH_PRIVATE_KEY.replace(/\\n/g, "\n");
  }
  if (process.env.E2E_REAL_WORKER_SSH_KEY_PATH) {
    return fs.readFileSync(process.env.E2E_REAL_WORKER_SSH_KEY_PATH, "utf8");
  }
  return "";
}

function readRealWorkerConfig({required = false} = {}) {
  const config = {
    host: process.env.E2E_REAL_WORKER_SSH_HOST || "",
    port: toPositiveInt(process.env.E2E_REAL_WORKER_SSH_PORT, 22),
    username: process.env.E2E_REAL_WORKER_SSH_USER || "root",
    privateKey: "",
  };

  try {
    config.privateKey = readPrivateKey();
  } catch (error) {
    if (required) {
      throw new Error(`Could not read E2E real worker SSH key: ${error.message}`);
    }
    console.warn(`[real-worker] Could not read SSH key: ${error.message}`);
    return null;
  }

  const missing = [];
  if (!config.host) {
    missing.push("E2E_REAL_WORKER_SSH_HOST");
  }
  if (!config.username) {
    missing.push("E2E_REAL_WORKER_SSH_USER");
  }
  if (!config.privateKey) {
    missing.push("E2E_REAL_WORKER_SSH_PRIVATE_KEY or E2E_REAL_WORKER_SSH_KEY_PATH");
  }
  if (missing.length > 0) {
    if (required) {
      throw new Error(`Missing real worker SSH configuration: ${missing.join(", ")}`);
    }
    return null;
  }
  return config;
}

function hasRealWorkerConfig() {
  return Boolean(readRealWorkerConfig());
}

async function readOkJson(response, label) {
  if (!response.ok()) {
    throw new Error(`${label} returned HTTP ${response.status()}`);
  }
  const body = await response.json();
  if (body.status !== "ok") {
    throw new Error(`${label} returned ${body.status}: ${body.msg || "<empty message>"}`);
  }
  return body;
}

async function createRealWorkerMachine(page, machineName) {
  const config = readRealWorkerConfig({required: true});
  const response = await page.context().request.post(API_ADD_MACHINE, {
    data: {
      owner: E2E_MACHINE_OWNER,
      name: machineName,
      displayName: "Real Worker E2E",
      ip: config.host,
      port: config.port,
      username: config.username,
      authType: "privateKey",
      password: "",
      privateKey: config.privateKey,
      status: "Unknown",
      role: "worker",
    },
  });
  await readOkJson(response, "add real worker machine");
}

async function deleteMachine(page, machineName) {
  const response = await page.context().request.post(API_DELETE_MACHINE, {
    data: {owner: E2E_MACHINE_OWNER, name: machineName},
  });
  await readOkJson(response, "delete real worker machine");
}

function machineTable(page) {
  return page.locator(".ant-table-wrapper").filter({hasText: "Machines"});
}

function workerNodeDialog(page, machineName) {
  return page.getByRole("dialog", {name: `Worker Node - ${machineName}`});
}

async function deployWorkerNodeFromUi(page, machineName) {
  await page.goto("/machines");
  await page.waitForLoadState("networkidle");

  const machineRow = machineTable(page).locator(`tr[data-row-key="${machineName}"]`);
  await expect(machineRow).toBeVisible({timeout: 15 * 1000});
  await machineRow.getByRole("button", {name: "Deploy worker node"}).click();

  const dialog = workerNodeDialog(page, machineName);
  await expect(dialog).toBeVisible();
  await expect(dialog.getByLabel("Node name")).toHaveValue(machineName);
  await dialog.getByLabel("Apiserver URL").fill(E2E_APISERVER_URL);

  const deployMachineNode = page.waitForResponse(response =>
    response.url().includes(API_DEPLOY_MACHINE_NODE) && response.request().method() === "POST"
  );
  await dialog.getByRole("button", {name: "Deploy Node"}).click();
  const body = await readOkJson(await deployMachineNode, "deploy real worker node");
  expect(body.data).toMatchObject({
    machineName,
    nodeName: machineName,
    apiserverUrl: E2E_APISERVER_URL,
  });
  return body.data;
}

async function getMachineNodeTasks(page, machineName) {
  const response = await page.context().request.get(
    `${API_GET_MACHINE_NODE_TASKS}?owner=${encodeURIComponent(E2E_MACHINE_OWNER)}&machineName=${encodeURIComponent(machineName)}`
  );
  const body = await readOkJson(response, "get real worker node tasks");
  return body.data || [];
}

async function getMachineNodeLogs(page, taskId) {
  const response = await page.context().request.get(`${API_GET_MACHINE_NODE_LOGS}?taskId=${encodeURIComponent(taskId)}`);
  const body = await readOkJson(response, "get real worker node logs");
  return body.data || [];
}

async function getNodes(page) {
  const response = await page.context().request.get(API_GET_NODES);
  const body = await readOkJson(response, "get nodes");
  return body.data || [];
}

async function getMetrics(page) {
  const response = await page.context().request.get(API_GET_METRICS);
  const body = await readOkJson(response, "get worker kubelet metrics");
  return body.data || {nodes: [], pods: []};
}

function formatWorkerLogs(logs) {
  return logs.map(log => `${log.createdAt || ""} ${log.level || ""} ${log.message || ""}`.trim()).join("\n");
}

async function waitForWorkerNodeReady(page, machineName, taskId, timeoutMs = REAL_WORKER_TIMEOUT_MS) {
  if (!taskId) {
    throw new Error(`taskId is required for waitForWorkerNodeReady (machineName=${machineName})`);
  }

  const startedAt = Date.now();
  let lastTask = null;
  let lastNodes = [];
  let lastLogs = [];
  let nextProgressAt = startedAt;

  while (Date.now() - startedAt < timeoutMs) {
    const tasks = await getMachineNodeTasks(page, machineName);
    lastTask = tasks.find(task => task.id === taskId) || null;
    if (lastTask?.status === "failed") {
      lastLogs = await getMachineNodeLogs(page, taskId);
      throw new Error(`real worker deployment failed during ${lastTask.phase}: ${lastTask.errorMsg || "no error message"}\n${formatWorkerLogs(lastLogs)}`);
    }

    lastNodes = await getNodes(page);
    const node = lastNodes.find(item => item.name === machineName);
    if (node?.status === "Ready" && (node.internalIP || node.externalIP)) {
      return {node, task: lastTask};
    }

    if (Date.now() >= nextProgressAt) {
      nextProgressAt = Date.now() + 30 * 1000;
      lastLogs = await getMachineNodeLogs(page, taskId).catch(() => lastLogs);
      console.log(
        `[real-worker] waiting for ${machineName}; task=${JSON.stringify(lastTask)} nodes=${JSON.stringify(lastNodes)} logs=\n${formatWorkerLogs(lastLogs.slice(-6))}`
      );
    }
    await page.waitForTimeout(REAL_WORKER_POLL_MS);
  }

  if (taskId) {
    lastLogs = await getMachineNodeLogs(page, taskId).catch(() => []);
  }
  throw new Error(
    [
      `timed out waiting for real worker node ${machineName} to become Ready`,
      `last task: ${JSON.stringify(lastTask)}`,
      `last nodes: ${JSON.stringify(lastNodes)}`,
      `logs:\n${formatWorkerLogs(lastLogs)}`,
    ].join("\n")
  );
}

async function expectWorkerNodeReadyInUi(page, nodeName) {
  await page.goto("/nodes");
  await page.waitForLoadState("networkidle");
  const nodesTable = page.locator(".ant-table-wrapper").filter({hasText: "Nodes"});
  const nodeRow = nodesTable.locator(`tr[data-row-key="${nodeName}"]`);
  await expect(nodeRow).toBeVisible({timeout: 15 * 1000});
  await expect(nodeRow).toContainText("Ready");
}

async function expectWorkerKubeletMetricsAvailable(page, nodeName, timeoutMs = REAL_WORKER_METRICS_TIMEOUT_MS) {
  const startedAt = Date.now();
  let lastMetrics = null;

  while (Date.now() - startedAt < timeoutMs) {
    lastMetrics = await getMetrics(page);
    const nodeMetric = (lastMetrics.nodes || []).find(item => item.name === nodeName);
    if (nodeMetric && Number(nodeMetric.memUsedMi) > 0) {
      console.log(`[real-worker] kubelet metrics are available for ${nodeName}: ${JSON.stringify(nodeMetric)}`);
      return lastMetrics;
    }
    await page.waitForTimeout(REAL_WORKER_POLL_MS);
  }

  throw new Error(
    [
      `timed out waiting for worker kubelet metrics from ${nodeName}`,
      "expected /api/get-metrics to read kubelet /stats/summary and report non-zero memUsedMi",
      `last metrics: ${JSON.stringify(lastMetrics)}`,
    ].join("\n")
  );
}

async function attachWorkerNodeLogs(page, taskId, testInfo) {
  if (!taskId) {
    return;
  }
  const logs = await getMachineNodeLogs(page, taskId).catch(error => ([{
    createdAt: "",
    level: "error",
    message: `Failed to read worker node logs: ${error.message}`,
  }]));
  await testInfo.attach("worker-node-deploy-logs.txt", {
    body: formatWorkerLogs(logs) || "No worker node deployment logs were returned.",
    contentType: "text/plain",
  });
}

module.exports = {
  attachWorkerNodeLogs,
  createRealWorkerMachine,
  deleteMachine,
  deployWorkerNodeFromUi,
  expectWorkerKubeletMetricsAvailable,
  expectWorkerNodeReadyInUi,
  hasRealWorkerConfig,
  readRealWorkerConfig,
  waitForWorkerNodeReady,
};
