/* eslint-env jest */

import {helmCompatibilityMessageKeys, resolveHelmCompatibilityError} from "./helmCompatibilityErrors";

describe("resolveHelmCompatibilityError", () => {
  test.each([
    "INVALID_MANIFEST",
    "RESOURCE_NOT_SERVED",
    "RESOURCE_NOT_FOUND",
    "FORBIDDEN_BY_RBAC",
    "CONFLICT",
    "DISCOVERY_FAILED",
    "API_UNAVAILABLE",
    "HELM_DRY_RUN_FAILED",
  ])("maps %s to a friendly message without parsing the backend message", code => {
    const t = jest.fn(key => `translated:${key}`);
    const result = resolveHelmCompatibilityError({
      message: "opaque backend details",
      code,
      gvk: "cert-manager.io/v1, Kind=Certificate",
    }, t);

    expect(result.message).toBe(`translated:${helmCompatibilityMessageKeys[code]}`);
    expect(result.gvk).toBe("cert-manager.io/v1, Kind=Certificate");
    expect(result.detail).toBe("opaque backend details");
  });

  test("falls back to the backend message for an unknown code", () => {
    expect(resolveHelmCompatibilityError({message: "install failed", code: "UNKNOWN"}, key => key))
      .toEqual({message: "install failed", gvk: "", detail: ""});
  });
});
