/* eslint-env jest */

import {buildRecommendedValues} from "./helmInstallDefaults";

test("adds release name and NodePort service defaults", () => {
  const {yaml, defaultsApplied} = buildRecommendedValues("service:\n  type: ClusterIP\n  port: 80\n", "demo");

  expect(defaultsApplied).toBe(true);
  expect(yaml).toContain("fullnameOverride: demo");
  expect(yaml).toContain("type: NodePort");
  expect(yaml).toContain("port: 80");
});

test("does not create service override when chart has no service block", () => {
  const {yaml, defaultsApplied} = buildRecommendedValues("replicaCount: 1\n", "demo");

  expect(defaultsApplied).toBe(true);
  expect(yaml).toContain("fullnameOverride: demo");
  expect(yaml).toContain("replicaCount: 1");
  expect(yaml).not.toContain("service:");
});

test("keeps original yaml when it is not an object", () => {
  const {yaml, defaultsApplied} = buildRecommendedValues("- one\n- two\n", "demo");

  expect(defaultsApplied).toBe(false);
  expect(yaml).toBe("- one\n- two\n");
});

test("keeps original yaml when no defaults are applied", () => {
  const original = "replicaCount: 1 # keep comment\n";
  const {yaml, defaultsApplied} = buildRecommendedValues(original, "");

  expect(defaultsApplied).toBe(false);
  expect(yaml).toBe(original);
});

test("keeps YAML timestamps when defaults are applied", () => {
  const {yaml} = buildRecommendedValues("scheduledAt: 2024-01-01\n", "demo");

  expect(yaml).toContain("scheduledAt:");
  expect(yaml).not.toContain("scheduledAt: {}");
});

test("skips dangerous merge keys from parsed yaml", () => {
  const {yaml} = buildRecommendedValues("__proto__:\n  polluted: true\nservice:\n  type: ClusterIP\n", "demo");

  expect(yaml).toContain("fullnameOverride: demo");
  expect(yaml).toContain("type: NodePort");
  expect(yaml).not.toContain("polluted: true");
  expect({}.polluted).toBeUndefined();
});

test("sanitizes dangerous keys nested inside arrays", () => {
  const {yaml} = buildRecommendedValues("service:\n  type: ClusterIP\nextra:\n  - __proto__:\n      polluted: true\n", "demo");

  expect(yaml).toContain("extra:\n  - {}");
  expect({}.polluted).toBeUndefined();
});
