const {defineConfig} = require("@playwright/test");

const launchOptions = {};
if (process.env.PLAYWRIGHT_EXECUTABLE_PATH) {
  launchOptions.executablePath = process.env.PLAYWRIGHT_EXECUTABLE_PATH;
}

module.exports = defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  expect: {
    timeout: 30_000,
  },
  fullyParallel: false,
  retries: 0,
  reporter: process.env.CI ? [
    ["list"],
    ["junit", {outputFile: "test-results/playwright-junit.xml"}],
    ["html", {outputFolder: "playwright-report", open: "never"}],
  ] : [["list"]],
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL || "http://127.0.0.1:9000",
    headless: true,
    launchOptions,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: process.env.CI ? "retain-on-failure" : "off",
  },
});
