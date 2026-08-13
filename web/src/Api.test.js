/* eslint-env jest */
import {apiFetch, setAuthStatus, setServerUrl} from "./Api";

describe("apiFetch", () => {
  beforeEach(() => {
    global.fetch = jest.fn(() => Promise.resolve(new Response("{}", {headers: {"Content-Type": "application/json"}})));
    setServerUrl("http://localhost:9000");
    setAuthStatus({csrfToken: "csrf-value"});
  });

  afterEach(() => {
    jest.restoreAllMocks();
    setAuthStatus(null);
  });

  test("adds credentials and CSRF to mutating CasOS requests", async() => {
    await apiFetch("/api/example", {method: "POST"});
    const [, options] = global.fetch.mock.calls[0];
    expect(options.credentials).toBe("include");
    expect(options.headers.get("X-CSRF-Token")).toBe("csrf-value");
  });

  test("does not add CSRF to read requests", async() => {
    await apiFetch("/api/example");
    const [, options] = global.fetch.mock.calls[0];
    expect(options.headers.has("X-CSRF-Token")).toBe(false);
  });

  test("does not attach CasOS credentials or CSRF to third-party requests", async() => {
    await apiFetch("https://registry.example.test/v2/search", {method: "POST"});
    const [, options] = global.fetch.mock.calls[0];
    expect(options.credentials).toBeUndefined();
    expect(options.headers.has("X-CSRF-Token")).toBe(false);
  });
});
