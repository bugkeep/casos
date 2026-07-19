const {randomUUID} = require("crypto");
const {expect} = require("@playwright/test");

const API_E2E_SIGNIN = "/api/e2e/signin";
const API_GET_NODES = "/api/get-nodes";
const e2eToken = process.env.E2E_TEST_TOKEN;
const e2eSshPassword = process.env.E2E_SSH_PASSWORD || randomUUID();

async function expectOkJson(response) {
  expect(response.ok(), `${response.status()} ${response.url()}`).toBeTruthy();
  const body = await response.json();
  expect(body.status, JSON.stringify(body)).toBe("ok");
  return body;
}

async function waitForApiserverReady(page) {
  await expect.poll(async () => {
    const response = await page.context().request.get(API_GET_NODES);
    if (!response.ok()) {
      return {status: `http-${response.status()}`};
    }
    return response.json();
  }, {
    message: "wait for the apiserver before starting UI workflows",
    timeout: 30_000,
  }).toMatchObject({status: "ok"});
}

async function signInAsCiUser(page) {
  expect(e2eToken).toBeTruthy();

  await page.addInitScript(() => {
    localStorage.setItem("language", "en");
  });

  const signin = await page.context().request.post(API_E2E_SIGNIN, {
    headers: {
      "X-Casos-E2E-Token": e2eToken,
    },
  });
  const signinBody = await expectOkJson(signin);
  expect(signinBody.data).toMatchObject({
    name: "ci-user",
    displayName: "CI User",
  });
  await waitForApiserverReady(page);
}

module.exports = {
  e2eSshPassword,
  expectOkJson,
  signInAsCiUser,
};
