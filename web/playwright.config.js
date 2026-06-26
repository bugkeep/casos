// Copyright 2026 The Casos Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

const os = require("os");
const path = require("path");
const crypto = require("crypto");
const {defineConfig, devices} = require("@playwright/test");

function safePort(value, defaultPort) {
  const port = Number(value || defaultPort);
  return Number.isInteger(port) && port > 0 && port <= 65535 ? port : defaultPort;
}

const localE2ETestToken = crypto.randomBytes(32).toString("hex");
const backendPort = safePort(process.env.E2E_BACKEND_PORT, 9000);
const frontendPort = safePort(process.env.E2E_FRONTEND_PORT, 8001);
const baseURL = `http://127.0.0.1:${frontendPort}`;
const backendURL = `http://127.0.0.1:${backendPort}`;
const backendHealthPath = process.env.E2E_HEALTH_CHECK_PATH || "/api/get-built-in-site";
const e2eToken = process.env.E2E_TEST_TOKEN || localE2ETestToken;
// Keep local runs convenient while giving specs and the backend the same token.
process.env.E2E_TEST_TOKEN = e2eToken;
const e2eDataDir = process.env.E2E_DATA_DIR || path.join(os.tmpdir(), `casos-e2e-${process.pid}`);
const backendDir = path.resolve(__dirname, "..");

module.exports = defineConfig({
  testDir: "./tests/ui",
  outputDir: "test-results",
  timeout: 30 * 1000,
  expect: {
    timeout: 10 * 1000,
  },
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? [["github"], ["html", {open: "never"}]] : "list",
  use: {
    baseURL,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
    video: "retain-on-failure",
    viewport: {width: 1280, height: 720},
  },
  projects: [
    {
      name: "chromium",
      use: {...devices["Desktop Chrome"]},
    },
  ],
  webServer: [
    {
      command: "go run main.go -createDatabase=true",
      cwd: backendDir,
      url: `${backendURL}${backendHealthPath}`,
      reuseExistingServer: !process.env.CI,
      timeout: 180 * 1000,
      env: {
        ...process.env,
        httpport: String(backendPort),
        dataDir: e2eDataDir,
        apiserverPort: process.env.E2E_APISERVER_PORT || "16443",
        webhookPort: process.env.E2E_WEBHOOK_PORT || "19443",
        socks5Proxy: process.env.socks5Proxy || "",
        e2eTestMode: "true",
        e2eTestToken: e2eToken,
        e2eTestOwner: "admin",
        e2eTestAdmin: "true",
      },
    },
    {
      command: "yarn start",
      url: baseURL,
      reuseExistingServer: !process.env.CI,
      timeout: 120 * 1000,
      env: {
        ...process.env,
        BROWSER: "none",
        CI: "false",
        PORT: String(frontendPort),
      },
    },
  ],
});
