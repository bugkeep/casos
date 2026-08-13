const {expect, test} = require("@playwright/test");

test.skip(!process.env.E2E_STANDALONE_URL, "requires an embedded standalone server");

test("completes browser setup, sign-out, and local sign-in @standalone", async({page}) => {
  const username = "browseradmin";
  const password = "standalone-browser-password";

  await page.setViewportSize({width: 390, height: 844});
  await page.addInitScript(() => localStorage.setItem("language", "en"));
  await page.goto("/");

  await expect(page).toHaveURL(/\/setup$/);
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Password", {exact: true}).fill(password);
  await page.getByLabel("Confirm password").fill(password);
  await page.getByRole("button", {name: "Create administrator"}).click();

  await expect(page).toHaveURL(/\/$/);
  const accountMenu = page.getByLabel(`Account menu for ${username}`);
  await expect(accountMenu).toBeVisible();
  await expect(page.locator("html")).toHaveJSProperty("scrollWidth", 390);
  const accountMenuBox = await accountMenu.boundingBox();
  expect(accountMenuBox.x).toBeGreaterThanOrEqual(0);
  expect(accountMenuBox.x + accountMenuBox.width).toBeLessThanOrEqual(390);
  await accountMenu.click();
  await page.getByText("Sign Out", {exact: true}).click();

  await expect(page).toHaveURL(/\/signin$/);
  await page.getByLabel("Username").fill(username.toUpperCase());
  await page.getByLabel("Password", {exact: true}).fill(password);
  await page.getByRole("button", {name: "Sign in"}).click();

  await expect(page).toHaveURL(/\/$/);
  await expect(accountMenu).toBeVisible();
  await expect(page.locator("html")).toHaveJSProperty("scrollWidth", 390);
});
