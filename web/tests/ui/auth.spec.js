const {expect, request, test} = require("@playwright/test");

const backendURL = `http://127.0.0.1:${process.env.E2E_BACKEND_PORT || 9000}`;

const localUser = {
  id: "1",
  owner: "built-in",
  name: "localadmin",
  displayName: "Local Admin",
  isAdmin: true,
  provider: "local",
};

async function mockAuthStatus(page, data) {
  await page.route("**/api/auth/status", route => route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({status: "ok", msg: "", data}),
  }));
}

test.beforeEach(async({page}) => {
  await page.addInitScript(() => localStorage.setItem("language", "en"));
});

test.describe.configure({mode: "serial"});

test("enforces the local authentication boundary against the real backend @smoke", async() => {
  test.setTimeout(120 * 1000);
  const password = "standalone-admin-password";
  const newPassword = "updated-standalone-password";
  const proxied = await request.newContext({baseURL: backendURL, extraHTTPHeaders: {"X-Forwarded-For": "127.0.0.1"}});
  const first = await request.newContext({baseURL: backendURL});
  const second = await request.newContext({baseURL: backendURL});
  const stale = await request.newContext({baseURL: backendURL});
  const rateLimited = await request.newContext({baseURL: backendURL});
  const crossOrigin = await request.newContext({baseURL: backendURL, extraHTTPHeaders: {Origin: "https://example.invalid"}});

  try {
    const initialStatus = await first.get("/api/auth/status");
    expect(initialStatus.status()).toBe(200);
    expect((await initialStatus.json()).data).toMatchObject({
      provider: "local",
      initialized: false,
      authenticated: false,
      canSetup: true,
    });

    const proxiedStatus = await proxied.get("/api/auth/status");
    expect((await proxiedStatus.json()).data).toMatchObject({canSetup: false, canRecover: false});
    const rejectedProxySetup = await proxied.post("/api/auth/setup", {
      data: {username: "localadmin", password},
    });
    expect(rejectedProxySetup.status()).toBe(403);

    const unauthenticatedBusinessRequest = await second.get("/api/get-account");
    expect(unauthenticatedBusinessRequest.status()).toBe(401);

    const setupResults = await Promise.all([
      first.post("/api/auth/setup", {data: {username: "LocalAdmin", password}}),
      second.post("/api/auth/setup", {data: {username: "LocalAdmin", password}}),
    ]);
    expect(setupResults.map(response => response.status()).sort()).toEqual([200, 409]);
    const authenticated = setupResults[0].status() === 200 ? first : second;

    const authenticatedStatus = await authenticated.get("/api/auth/status");
    const authenticatedData = (await authenticatedStatus.json()).data;
    expect(authenticatedData).toMatchObject({initialized: true, authenticated: true});
    expect(authenticatedData.csrfToken).toEqual(expect.any(String));

    const noCSRF = await authenticated.post("/api/auth/password", {
      data: {currentPassword: password, newPassword},
    });
    expect(noCSRF.status()).toBe(403);

    const staleLogin = await stale.post("/api/auth/login", {
      data: {username: "localadmin", password},
    });
    expect(staleLogin.status()).toBe(200);
    const oldSessionCookie = (await authenticated.storageState()).cookies.find(cookie => cookie.name === "casos_session");

    const passwordChange = await authenticated.post("/api/auth/password", {
      headers: {"X-CSRF-Token": authenticatedData.csrfToken},
      data: {currentPassword: password, newPassword},
    });
    expect(passwordChange.status()).toBe(200);
    const newSessionCookie = (await authenticated.storageState()).cookies.find(cookie => cookie.name === "casos_session");
    expect(newSessionCookie?.value).toBeTruthy();
    expect(newSessionCookie?.value).not.toBe(oldSessionCookie?.value);

    const staleStatus = await stale.get("/api/auth/status");
    expect((await staleStatus.json()).data.authenticated).toBe(false);

    const caseInsensitiveLogin = await stale.post("/api/auth/login", {
      data: {username: "LOCALADMIN", password: newPassword},
    });
    expect(caseInsensitiveLogin.status()).toBe(200);

    for (let attempt = 0; attempt < 5; attempt += 1) {
      const failure = await rateLimited.post("/api/auth/login", {
        data: {username: "rate-limit-probe", password: "incorrect-password"},
      });
      expect(failure.status()).toBe(401);
    }
    const locked = await rateLimited.post("/api/auth/login", {
      data: {username: "rate-limit-probe", password: "incorrect-password"},
    });
    expect(locked.status()).toBe(429);

    const rejectedOrigin = await crossOrigin.get("/api/auth/status");
    expect(rejectedOrigin.headers()["access-control-allow-origin"]).toBeUndefined();
  } finally {
    await Promise.all([proxied.dispose(), first.dispose(), second.dispose(), stale.dispose(), rateLimited.dispose(), crossOrigin.dispose()]);
  }
});

test("routes an uninitialized local installation to responsive setup @smoke", async({page}) => {
  await mockAuthStatus(page, {
    provider: "local",
    initialized: false,
    authenticated: false,
    canSetup: true,
    canRecover: true,
  });

  await page.goto("/");

  await expect(page).toHaveURL(/\/setup$/);
  await expect(page.getByRole("heading", {name: "Set up CasOS"})).toBeVisible();
  await expect(page.getByLabel("Username")).toBeVisible();
  await expect(page.getByLabel("Confirm password")).toBeVisible();
  await expect(page.getByRole("button", {name: "Create administrator"})).toBeVisible();
});

test("shows local sign-in and direct-loopback recovery states @smoke", async({page}) => {
  await mockAuthStatus(page, {
    provider: "local",
    initialized: true,
    authenticated: false,
    canSetup: false,
    canRecover: true,
  });

  await page.goto("/signin");
  await expect(page.getByRole("heading", {name: "Sign in"})).toBeVisible();
  await expect(page.getByRole("link", {name: "Forgot password"})).toBeVisible();

  await page.goto("/recover");
  await expect(page.getByRole("heading", {name: "Reset password"})).toBeVisible();
  await expect(page.getByLabel("New password")).toBeVisible();
  await expect(page.getByLabel("Confirm password")).toBeVisible();
});

test("blocks recovery in the UI when the server is not loopback-only @smoke", async({page}) => {
  await mockAuthStatus(page, {
    provider: "local",
    initialized: true,
    authenticated: false,
    canSetup: false,
    canRecover: false,
  });

  await page.goto("/recover");

  await expect(page.getByText("Recovery unavailable")).toBeVisible();
  await expect(page.getByText("Password recovery is restricted to a direct loopback connection.")).toBeVisible();
  await expect(page.getByRole("button", {name: "Reset password"})).toHaveCount(0);
});

test("uses server-provided Casdoor configuration for enterprise sign-in @smoke", async({page}) => {
  await page.route("https://sso.example.test/**", route => route.fulfill({status: 200, body: "SSO"}));
  await mockAuthStatus(page, {
    provider: "casdoor",
    initialized: true,
    authenticated: false,
    canSetup: false,
    canRecover: false,
    casdoor: {
      serverUrl: "https://sso.example.test",
      clientId: "browser-client",
      organizationName: "example-org",
      appName: "casos",
      redirectPath: "/callback",
    },
  });

  await page.goto("/signin");

  await expect(page).toHaveURL(/^https:\/\/sso\.example\.test\//);
  expect(page.url()).toContain("client_id=browser-client");
});

test("offers password change only for an authenticated local administrator @smoke", async({page}) => {
  await mockAuthStatus(page, {
    provider: "local",
    initialized: true,
    authenticated: true,
    canSetup: false,
    canRecover: true,
    user: localUser,
    csrfToken: "test-csrf-token",
  });

  await page.goto("/");
  await page.getByText("Local Admin").click();
  await page.getByText("Change password", {exact: true}).click();

  await expect(page.getByRole("dialog", {name: "Change password"})).toBeVisible();
  await expect(page.getByLabel("Current password")).toBeVisible();
  await expect(page.getByLabel("New password")).toBeVisible();
  await expect(page.getByLabel("Confirm password")).toBeVisible();
});
