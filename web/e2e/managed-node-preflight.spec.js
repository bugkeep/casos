const {test, expect} = require("@playwright/test");

test("nodes page preflight shows ssh failure for an unreachable managed node target", async ({page}) => {
  const authResponse = await page.context().request.post("/api/test-signin");
  expect(authResponse.ok()).toBeTruthy();
  const authBody = await authResponse.json();
  expect(authBody.status).toBe("ok");

  await expect.poll(async() => {
    const [siteResponse, namespacesResponse, managedNodesResponse] = await Promise.all([
      page.context().request.get("/api/get-built-in-site"),
      page.context().request.get("/api/get-namespaces"),
      page.context().request.get("/api/get-managed-nodes"),
    ]);
    const [siteBody, namespacesBody, managedNodesBody] = await Promise.all([
      siteResponse.json(),
      namespacesResponse.json(),
      managedNodesResponse.json(),
    ]);
    const siteReady = siteBody.status === "ok";
    const namespaceReady = namespacesBody.status === "ok" &&
      Array.isArray(namespacesBody.data) &&
      namespacesBody.data.some(item => item.name === "kube-system");
    const managedReady = managedNodesBody.status === "ok";
    return siteReady && namespaceReady && managedReady;
  }, {
    message: "expected CasOS, embedded k8s, and the managed-node API to be ready before opening the page",
    timeout: 60_000,
  }).toBe(true);

  await page.goto("/nodes");

  await expect(page.getByTestId("cluster-nodes-table")).toBeVisible();
  await expect(page.getByTestId("managed-nodes-table")).toBeVisible();
  await expect(page.getByTestId("managed-node-auto-deploy-button")).toBeVisible();

  await page.getByTestId("managed-node-auto-deploy-button").click();

  const dialog = page.getByTestId("managed-node-auto-deploy-modal");
  await expect(dialog).toBeVisible();

  await dialog.getByTestId("managed-node-name-input").locator("input").fill("ci-preflight-negative");
  await dialog.getByTestId("managed-node-host-input").locator("input").fill("127.0.0.1");
  await dialog.getByTestId("managed-node-port-input").locator("input").fill("1");
  await dialog.getByTestId("managed-node-username-input").locator("input").fill("root");
  await dialog.getByTestId("managed-node-password-input").locator("input").fill("wrong-password");

  await page.getByTestId("managed-node-preflight-button").click();

  const result = dialog.getByTestId("managed-node-preflight-result");
  await expect(result).toBeVisible();
  await expect(result.getByText("ssh connection failed")).toBeVisible();
});
