const {expect} = require("@playwright/test");
const {expectOkJson} = require("./e2e-helpers");
const {
  API_ADD_MACHINE,
  API_DEPLOY_MACHINE_NODE,
  API_DELETE_MACHINE,
  API_GET_MACHINE_NODE_LOGS,
  API_GET_MACHINE_NODE_TASKS,
  API_GET_NODES,
  E2E_APISERVER_URL,
  E2E_MACHINE_OWNER,
  LOG_PROGRESS_INTERVAL_MS,
  MACHINES_ROUTE,
  SSH_HOST,
  SSH_PORT,
  SSH_PRIVATE_KEY,
  SSH_USER,
  UI_NAVIGATION_TIMEOUT_MS,
  WORKER_NODE_POLL_INTERVAL_MS,
  WORKER_READY_TIMEOUT_MS,
  WORKER_TASK_POLL_INTERVAL_MS,
} = require("./app-store-access-url-config");
const {
  compactDeployLogs,
  compactTask,
  logAppStoreDiagnostic,
  logAppStoreProgress,
} = require("./app-store-access-url-log");
const {
  isRetryableInfrastructureError,
  readOkJson,
} = require("./app-store-access-url-http");
const {routeUrlPattern} = require("./app-store-access-url-routes");
const {
  collectWorkerNodeDiagnostics,
  resetWorkerDiagnosticsLog,
} = require("./app-store-access-url-worker-diagnostics");

const WORKER_DISPLAY_NAME = "App Store Access Worker";

async function createMachineFromUi(page, machineName) {
  await page.goto(MACHINES_ROUTE, {
    waitUntil: "domcontentloaded",
    timeout: UI_NAVIGATION_TIMEOUT_MS,
  });
  await expect(page).toHaveURL(routeUrlPattern(MACHINES_ROUTE));
  await expect(machineTableTitle(page)).toBeVisible({timeout: UI_NAVIGATION_TIMEOUT_MS});
  const addButton = machineTableTitle(page).getByRole("button", {name: "Add"});
  await expect(addButton).toBeVisible({timeout: UI_NAVIGATION_TIMEOUT_MS});
  logAppStoreDiagnostic("create-machine-route-ready", {machineName});

  await addButton.click();
  const addDialog = page.getByRole("dialog", {name: "Add Machine"});
  await expect(addDialog).toBeVisible({timeout: UI_NAVIGATION_TIMEOUT_MS});
  logAppStoreDiagnostic("create-machine-dialog-ready", {machineName});
  await addDialog.getByPlaceholder("my-machine").fill(machineName);
  await addDialog.getByPlaceholder("My Machine").fill(WORKER_DISPLAY_NAME);
  await addDialog.getByPlaceholder("192.168.1.10").fill(SSH_HOST);
  await addDialog.getByLabel(/SSH port/).fill(String(SSH_PORT));
  await addDialog.getByPlaceholder("root").fill(SSH_USER);
  logAppStoreDiagnostic("create-machine-fields-filled", {machineName});
  await selectPrivateKeyAuthType(page, addDialog, machineName);
  const privateKeyField = addDialog.getByLabel(/Private key/);
  await privateKeyField.fill(SSH_PRIVATE_KEY);
  logAppStoreDiagnostic("create-machine-private-key-filled", {machineName});

  const addMachine = page.waitForResponse(response =>
    response.url().includes(API_ADD_MACHINE) && response.request().method() === "POST"
  );
  logAppStoreDiagnostic("create-machine-submit-ready", {machineName});
  await addDialog.getByRole("button", {name: "Add"}).click();
  logAppStoreDiagnostic("create-machine-submit-clicked", {machineName});
  await expectOkJson(await addMachine);
  logAppStoreDiagnostic("create-machine-api-created", {machineName});
  await expect(addDialog).toBeHidden();
  await findMachineRow(page, machineName);
  logAppStoreDiagnostic("create-machine-row-visible", {machineName});
}

async function selectPrivateKeyAuthType(page, addDialog, machineName) {
  const authTypeSelect = addDialog.getByRole("combobox", {name: "Auth type"});
  await authTypeSelect.click({timeout: UI_NAVIGATION_TIMEOUT_MS});
  logAppStoreDiagnostic("create-machine-auth-dropdown-open", {machineName});
  // Ant Design exposes the dropdown option as a hidden role=option node in CI, so use the focused combobox keyboard path.
  await authTypeSelect.press("ArrowDown");
  await authTypeSelect.press("Enter");
  await expect(addDialog.getByLabel(/Private key/)).toBeVisible({timeout: UI_NAVIGATION_TIMEOUT_MS});
  logAppStoreDiagnostic("create-machine-auth-private-key-selected", {machineName});
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
  let lastLogs = [];
  let lastRetryableError = "";
  let lastProgressLogAt = 0;
  while (Date.now() - started < WORKER_READY_TIMEOUT_MS) {
    try {
      const tasks = await getMachineNodeTasks(page, nodeName);
      const matchedTask = tasks.find(task => task.id === taskId);
      if (!matchedTask) {
        lastTask = {
          id: taskId,
          status: "pending",
          phase: "waiting-for-task",
          errorMsg: `expected task not found among ${tasks.length} tasks`,
        };
        lastRetryableError = "";
      } else {
        lastTask = matchedTask;
      }
      if (lastTask?.status === "failed") {
        throw new Error(`worker node deployment failed during ${lastTask.phase}: ${lastTask.errorMsg || "worker task did not include errorMsg"}`);
      }
      if (lastTask?.status === "succeeded" && lastTask.phase === "ready") {
        const remainingMs = Math.max(1000, WORKER_READY_TIMEOUT_MS - (Date.now() - started));
        return waitForNodeSummary(page, nodeName, remainingMs);
      }
      lastRetryableError = "";
    } catch (error) {
      if (!isRetryableInfrastructureError(error)) {
        throw error;
      }
      lastTask = {
        id: taskId,
        status: "pending",
        phase: "task-poll-retry",
        errorMsg: error.message,
      };
      lastRetryableError = error.message;
    }
    const now = Date.now();
    if (now - lastProgressLogAt >= LOG_PROGRESS_INTERVAL_MS) {
      lastProgressLogAt = now;
      if (lastTask?.id) {
        try {
          lastLogs = await getMachineNodeLogs(page, lastTask.id);
        } catch (error) {
          if (!isRetryableInfrastructureError(error)) {
            throw error;
          }
          lastRetryableError = error.message;
        }
      }
      logAppStoreProgress("wait-worker-ready", started, {
        task: compactTask(lastTask),
        logs: compactDeployLogs(lastLogs, 5),
        lastRetryableError,
      });
    }
    await page.waitForTimeout(WORKER_TASK_POLL_INTERVAL_MS);
  }
  if (lastTask?.id) {
    try {
      lastLogs = await getMachineNodeLogs(page, lastTask.id);
    } catch (error) {
      lastRetryableError = error.message || String(error);
    }
  }
  const diagnosticsPath = await collectWorkerNodeDiagnostics("worker-ready-timeout");
  throw new Error(`timed out waiting for worker node deployment; last task: ${JSON.stringify(lastTask || null)}; last logs: ${JSON.stringify(compactDeployLogs(lastLogs, 12))}${lastRetryableError ? `; last retryable error: ${lastRetryableError}` : ""}${diagnosticsPath ? `; worker diagnostics: ${diagnosticsPath}` : ""}`);
}

async function getMachineNodeTasks(page, machineName) {
  const response = await page.context().request.get(
    `${API_GET_MACHINE_NODE_TASKS}?owner=${encodeURIComponent(E2E_MACHINE_OWNER)}&machineName=${encodeURIComponent(machineName)}`
  );
  const body = await readOkJson(response, "get-machine-node-tasks");
  return body.data || [];
}

async function getMachineNodeLogs(page, taskId) {
  const response = await page.context().request.get(`${API_GET_MACHINE_NODE_LOGS}?taskId=${encodeURIComponent(taskId)}`);
  const body = await readOkJson(response, "get-machine-node-logs");
  return body.data || [];
}

async function getNodeSummaries(page) {
  const response = await page.context().request.get(API_GET_NODES);
  const body = await readOkJson(response, "get-nodes");
  return body.data || [];
}

async function waitForNodeSummary(page, nodeName, timeoutMs = 60 * 1000) {
  const started = Date.now();
  let lastNodes = [];
  let lastRetryableError = "";
  while (Date.now() - started < timeoutMs) {
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
    await page.waitForTimeout(WORKER_NODE_POLL_INTERVAL_MS);
  }
  throw new Error(`worker node ${nodeName} did not become visible with an IP; nodes: ${JSON.stringify(lastNodes)}${lastRetryableError ? `; last retryable error: ${lastRetryableError}` : ""}`);
}

async function deleteMachine(page, machineName) {
  const response = await page.context().request.post(API_DELETE_MACHINE, {
    data: {owner: E2E_MACHINE_OWNER, name: machineName},
  });
  await expectOkJson(response);
}

module.exports = {
  createMachineFromUi,
  deleteMachine,
  resetWorkerDiagnosticsLog,
  startWorkerNodeDeployment,
  waitForWorkerNodeReady,
};
