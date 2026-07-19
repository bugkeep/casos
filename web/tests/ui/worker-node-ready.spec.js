const {expect, test} = require("@playwright/test");
const {e2eSshPassword, signInAsCiUser} = require("./e2e-helpers");
const {
  createdMachinesFixture,
  createMachineFromUi,
  getMachineNodeLogs,
  getMachineNodeTasks,
  makeMachineName,
  startWorkerNodeDeployment,
  workerNodeDialog,
  workerNodeTaskTable,
} = require("./worker-node-helpers");

// This test only runs in CI jobs that provisioned a real worker VM
// (see "Prepare worker node VM" in .github/workflows/build.yml). It exercises
// the full path other worker-node.spec.js tests stop short of: the deployment
// task actually finishing and the node showing up as Ready on the Nodes page.
const E2E_WORKER_VM_IP = process.env.E2E_WORKER_VM_IP;
const E2E_APISERVER_URL = process.env.E2E_APISERVER_URL || "https://127.0.0.1:16443";
const E2E_MACHINE_PREFIX = "a-worker-ready-e2e";
const E2E_WORKER_DEPLOY_TIMEOUT_MS = Number(process.env.E2E_WORKER_DEPLOY_TIMEOUT_MS) || 8 * 60 * 1000;
const E2E_WORKER_READY_TIMEOUT_MS = Number(process.env.E2E_WORKER_READY_TIMEOUT_MS) || 2 * 60 * 1000;

const workerNodeReadyTest = test.extend({
  createdMachines: createdMachinesFixture,
});
// This test deploys to a real VM and can legitimately take several minutes;
// retrying it on failure would just re-run an expensive, non-flaky wait and
// risks blowing the CI job's overall time budget.
workerNodeReadyTest.describe.configure({retries: 0});

function nodeRow(page, nodeName) {
  return page.locator(".ant-table-wrapper").filter({hasText: "Nodes"}).locator(`tr[data-row-key="${nodeName}"]`);
}

function workflowCommandData(value) {
  return String(value).replace(/%/g, "%25").replace(/\r/g, "%0D").replace(/\n/g, "%0A");
}

function emitGithubActionsError(title, message) {
  if (process.env.GITHUB_ACTIONS !== "true") {
    return;
  }
  const property = String(title)
    .replace(/%/g, "%25")
    .replace(/\r/g, "%0D")
    .replace(/\n/g, "%0A")
    .replace(/:/g, "%3A")
    .replace(/,/g, "%2C");
  console.error(`::error title=${property}::${workflowCommandData(message)}`);
}

async function deploymentDiagnostics(page, machineName, taskId) {
  const details = [];
  try {
    const taskBody = await getMachineNodeTasks(page, machineName);
    const task = (taskBody.data || []).find(item => String(item.id) === String(taskId));
    details.push(`Task: ${JSON.stringify(task || null)}`);
  } catch (error) {
    details.push(`Task lookup failed: ${error.message}`);
  }
  try {
    const logBody = await getMachineNodeLogs(page, taskId);
    const logs = (logBody.data || []).slice(-25).map(log =>
      `${log.createdAt || ""} ${log.level || ""} ${log.message || ""}`.trim()
    );
    details.push(`Recent task logs:\n${logs.join("\n") || "(none)"}`);
  } catch (error) {
    details.push(`Task log lookup failed: ${error.message}`);
  }
  return details.join("\n");
}

async function waitForDeployTaskToSucceed(page, machineName, taskId) {
  const taskRow = workerNodeTaskTable(page, machineName).locator(`tr[data-row-key="${taskId}"]`);
  const statusTag = taskRow.locator("td .ant-tag");
  try {
    await expect.poll(async() => {
      const status = (await statusTag.textContent())?.trim().toLowerCase();
      if (status === "failed") {
        throw new Error(`Deployment task ${taskId} reached failed state`);
      }
      return status || "pending";
    }, {timeout: E2E_WORKER_DEPLOY_TIMEOUT_MS}).toBe("succeeded");
  } catch (error) {
    const alert = workerNodeDialog(page, machineName).getByRole("alert");
    const detail = await alert.isVisible() ? await alert.innerText() : "no deployment error was shown";
    const diagnostics = await deploymentDiagnostics(page, machineName, taskId);
    const message = `${error.message}\nDeployment task ${taskId} failed: ${detail}\n${diagnostics}`;
    emitGithubActionsError("Worker deployment diagnostics", message);
    throw new Error(message);
  }
}

async function waitForNodeReady(page, nodeName) {
  await expect(async() => {
    await page.goto("/nodes");
    await expect(nodeRow(page, nodeName).getByText("Ready", {exact: true})).toBeVisible({timeout: 2000});
  }).toPass({timeout: E2E_WORKER_READY_TIMEOUT_MS});
}

workerNodeReadyTest.beforeEach(async({page}) => {
  test.skip(!E2E_WORKER_VM_IP, "E2E_WORKER_VM_IP is not set; no worker VM was provisioned for this run");
  await signInAsCiUser(page);
});

workerNodeReadyTest("deployed worker node becomes Ready on the Nodes page", async({page, createdMachines}) => {
  workerNodeReadyTest.setTimeout(E2E_WORKER_DEPLOY_TIMEOUT_MS + E2E_WORKER_READY_TIMEOUT_MS + 60 * 1000);

  const machineName = makeMachineName(E2E_MACHINE_PREFIX);

  await createMachineFromUi(page, machineName, createdMachines, {
    ip: E2E_WORKER_VM_IP,
    username: "root",
    password: e2eSshPassword,
  });

  const task = await startWorkerNodeDeployment(page, machineName, E2E_APISERVER_URL);
  await waitForDeployTaskToSucceed(page, machineName, task.id);
  await workerNodeDialog(page, machineName).getByRole("button", {name: "Close"}).click();

  await waitForNodeReady(page, machineName);
});
