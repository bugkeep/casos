const {randomUUID} = require("crypto");
const {test} = require("@playwright/test");
const {signInAsCiUser} = require("./e2e-helpers");
const {
  attachWorkerNodeLogs,
  createRealWorkerMachine,
  deleteMachine,
  deployWorkerNodeFromUi,
  expectWorkerKubeletMetricsAvailable,
  expectWorkerNodeReadyInUi,
  hasRealWorkerConfig,
  readRealWorkerConfig,
  waitForWorkerNodeReady,
} = require("./real-worker-helpers");

test.skip(!hasRealWorkerConfig() && !process.env.CI, "requires E2E_REAL_WORKER_SSH_* to deploy a real worker node");

test.beforeEach(async({page}) => {
  await signInAsCiUser(page);
});

test("deploys a temporary VM as a real worker node @real-worker", async({page}, testInfo) => {
  test.setTimeout(Number(process.env.E2E_REAL_WORKER_TEST_TIMEOUT_MS || 12 * 60 * 1000));
  readRealWorkerConfig({required: true});

  const machineName = `real-worker-${randomUUID().slice(0, 8)}`;
  let createdMachine = false;
  let taskId = null;

  try {
    await createRealWorkerMachine(page, machineName);
    createdMachine = true;

    const task = await deployWorkerNodeFromUi(page, machineName);
    taskId = task.id;

    await waitForWorkerNodeReady(page, machineName, taskId);
    await expectWorkerNodeReadyInUi(page, machineName);
    const metrics = await expectWorkerKubeletMetricsAvailable(page, machineName);
    await testInfo.attach("worker-kubelet-metrics.json", {
      body: JSON.stringify(metrics, null, 2),
      contentType: "application/json",
    });
  } finally {
    await attachWorkerNodeLogs(page, taskId, testInfo);
    if (createdMachine) {
      await deleteMachine(page, machineName).catch(error => {
        console.warn(`[real-worker] cleanup failed for machine ${machineName}: ${error.message}`);
      });
    }
  }
});
