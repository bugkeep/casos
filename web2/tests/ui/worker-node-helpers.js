import {randomUUID} from "crypto";
import {expect} from "@playwright/test";
import {dataTable, e2eSshPassword, expectOkJson, expectTableIdle, expectToast, tableRow} from "./e2e-helpers.js";

const API_ADD_MACHINE = "/api/add-machine";
const API_DELETE_MACHINE = "/api/delete-machine";
const API_DEPLOY_MACHINE_NODE = "/api/deploy-machine-node";
const API_GET_MACHINE_NODE_TASKS = "/api/get-machine-node-tasks";
const API_REPAIR_MACHINE_NODE = "/api/repair-machine-node";
// MachineListPage currently submits new machines with owner "admin".
const E2E_MACHINE_OWNER = process.env.E2E_MACHINE_OWNER || "admin";

const MACHINES_TABLE = "machines-table";
const WORKER_NODE_TASKS_TABLE = "worker-node-tasks";

const createdMachinesFixture = async({page}, use) => {
  const createdMachines = [];
  await use(createdMachines);

  const cleanupErrors = [];
  for (const machine of [...createdMachines].reverse()) {
    try {
      const deleteMachine = await page.context().request.post(API_DELETE_MACHINE, {
        data: machine,
      });
      await expectOkJson(deleteMachine);
    } catch (error) {
      cleanupErrors.push(`${machine.name}: ${error.message}`);
    }
  }

  expect(cleanupErrors).toEqual([]);
};

function makeMachineName(prefix) {
  return `${prefix}-${randomUUID().slice(0, 8)}`;
}

function machineTable(page) {
  return dataTable(page, MACHINES_TABLE);
}

/**
 * Locates a machine's row by filtering the table rather than paging to it.
 *
 * The Ant Design version of this helper walked the pagination controls across
 * up to fifty pages, because that table had no filter. web2's DataTable
 * searches every loaded row regardless of which page is showing, so the match
 * is one keystroke away and the page-walking machinery is gone with it.
 */
async function findMachineRow(page, machineName) {
  const table = machineTable(page);
  await expect(table).toBeVisible();
  await expectTableIdle(page, MACHINES_TABLE);

  await table.getByPlaceholder("Search...").fill(machineName);

  const machineRow = tableRow(table, machineName);
  await expect(machineRow).toBeVisible();
  return machineRow;
}

async function getMachineNodeTasks(page, machineName) {
  const tasks = await page.context().request.get(
    `${API_GET_MACHINE_NODE_TASKS}?owner=${encodeURIComponent(E2E_MACHINE_OWNER)}&machineName=${encodeURIComponent(machineName)}`
  );
  return expectOkJson(tasks);
}

// The pane's heading is "Worker Node — <machine>" with an em dash, so it is
// matched loosely rather than reproducing the punctuation in every call site.
function workerNodeDialog(page, machineName) {
  return page.getByRole("dialog", {name: new RegExp(`Worker Node.*${machineName}`)});
}

function workerNodeTaskTable(page, machineName) {
  return workerNodeDialog(page, machineName).getByTestId(WORKER_NODE_TASKS_TABLE);
}

async function expectWorkerNodeTaskVisible(page, machineName, task) {
  const taskRow = tableRow(workerNodeTaskTable(page, machineName), task.id);
  await expect(taskRow.getByRole("cell", {name: String(task.id), exact: true})).toBeVisible();
  await expect(taskRow.getByRole("cell", {name: task.nodeName || machineName, exact: true})).toBeVisible();
}

async function expectWorkerNodeLogVisible(page, machineName, message) {
  await expect(workerNodeDialog(page, machineName).getByText(message)).toBeVisible();
}

async function createMachineFromUi(page, machineName, createdMachines, options = {}) {
  await page.goto("/machines");
  await expect(page).toHaveURL(/\/machines$/);
  await expectTableIdle(page, MACHINES_TABLE);

  // "Add Local WSL" also starts with Add, so the name has to match exactly.
  await machineTable(page).getByRole("button", {name: "Add", exact: true}).click();
  const addDialog = page.getByRole("dialog", {name: "Add Machine"});
  await expect(addDialog).toBeVisible();
  await addDialog.getByPlaceholder("my-machine").fill(machineName);
  await addDialog.getByPlaceholder("My Machine").fill(options.displayName || "E2E Worker Node");
  await addDialog.getByPlaceholder("192.168.1.10").fill(options.ip || "127.0.0.1");
  await addDialog.getByPlaceholder("root").fill(options.username || "root");
  // Matched by role: the field's reveal toggle is labelled "Show password", so
  // a plain getByLabel("Password") would resolve to both the input and a button.
  await addDialog.getByRole("textbox", {name: "Password"}).fill(options.password || e2eSshPassword);

  const addMachine = page.waitForResponse(response =>
    response.url().includes(API_ADD_MACHINE) && response.request().method() === "POST"
  );
  await addDialog.getByRole("button", {name: "Add", exact: true}).click();
  const addMachineResponse = await addMachine;
  try {
    await expectOkJson(addMachineResponse);
  } catch (error) {
    if (addMachineResponse.ok()) {
      createdMachines.push({owner: E2E_MACHINE_OWNER, name: machineName});
    }
    throw error;
  }
  createdMachines.push({owner: E2E_MACHINE_OWNER, name: machineName});
  await expect(addDialog).toBeHidden();

  await findMachineRow(page, machineName);
}

async function openWorkerNodePanel(page, machineName) {
  const machineRow = await findMachineRow(page, machineName);

  const loadTasks = page.waitForResponse(response =>
    response.url().includes(API_GET_MACHINE_NODE_TASKS) && response.request().method() === "GET"
  );
  await machineRow.getByRole("button", {name: "Deploy worker node"}).click();
  await expect(workerNodeDialog(page, machineName)).toBeVisible();
  return expectOkJson(await loadTasks);
}

async function submitWorkerNodeAction(page, machineName, buttonName, apiPath) {
  const request = page.waitForResponse(response =>
    response.url().includes(apiPath) && response.request().method() === "POST"
  );
  await workerNodeDialog(page, machineName).getByRole("button", {name: buttonName}).click();
  return request;
}

async function startWorkerNodeDeployment(page, machineName, apiserverUrl) {
  await openWorkerNodePanel(page, machineName);
  const dialog = workerNodeDialog(page, machineName);
  await expect(dialog.getByLabel("Node name")).toHaveValue(machineName);
  await dialog.getByLabel("Apiserver URL").fill(apiserverUrl);

  const deployMachineNode = submitWorkerNodeAction(page, machineName, "Deploy Node", API_DEPLOY_MACHINE_NODE);
  const deployBody = await expectOkJson(await deployMachineNode);
  expect(deployBody.data).toMatchObject({
    machineName,
    nodeName: machineName,
    apiserverUrl,
    status: "pending",
    phase: "queued",
  });

  await expectToast(page, "Node deployment started");
  await expectWorkerNodeTaskVisible(page, machineName, deployBody.data);
  await expectWorkerNodeLogVisible(page, machineName, "Node deployment task created");
  return deployBody.data;
}

async function startWorkerNodeRepair(page, machineName, apiserverUrl) {
  await openWorkerNodePanel(page, machineName);
  const dialog = workerNodeDialog(page, machineName);
  await expect(dialog.getByLabel("Node name")).toHaveValue(machineName);
  await dialog.getByLabel("Apiserver URL").fill(apiserverUrl);

  const repairMachineNode = submitWorkerNodeAction(page, machineName, "Repair Node", API_REPAIR_MACHINE_NODE);
  const repairBody = await expectOkJson(await repairMachineNode);
  expect(repairBody.data).toMatchObject({
    machineName,
    nodeName: machineName,
    apiserverUrl,
    status: "pending",
    phase: "queued",
  });

  await expectToast(page, "Node repair started");
  await expectWorkerNodeTaskVisible(page, machineName, repairBody.data);
  await expectWorkerNodeLogVisible(page, machineName, "Node deployment task created");
  return repairBody.data;
}

export {
  API_ADD_MACHINE,
  API_DELETE_MACHINE,
  API_DEPLOY_MACHINE_NODE,
  API_GET_MACHINE_NODE_TASKS,
  API_REPAIR_MACHINE_NODE,
  E2E_MACHINE_OWNER,
  MACHINES_TABLE,
  WORKER_NODE_TASKS_TABLE,
  createdMachinesFixture,
  createMachineFromUi,
  findMachineRow,
  getMachineNodeTasks,
  machineTable,
  makeMachineName,
  openWorkerNodePanel,
  startWorkerNodeDeployment,
  startWorkerNodeRepair,
  submitWorkerNodeAction,
  workerNodeDialog,
  workerNodeTaskTable,
  expectWorkerNodeTaskVisible,
  expectWorkerNodeLogVisible,
};
