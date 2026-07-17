const assert = require("node:assert/strict");
const test = require("node:test");
const {
  ALL_CATALOG_CASES,
  INSTALLABLE_CASES,
  assertCatalogSnapshot,
  UNSUPPORTED_CASES,
  validateCatalogPartition,
} = require("../ui/app-store-catalog-cases");

test("catalog snapshot covers every bounded preset repository chart", () => {
  assert.equal(ALL_CATALOG_CASES.length, 194);
  assert.equal(new Set(ALL_CATALOG_CASES.map(item => `${item.repo}/${item.chart}`)).size, 194);
});

test("compatibility results partition the complete catalog", () => {
  assert.equal(INSTALLABLE_CASES.length, 130);
  assert.equal(UNSUPPORTED_CASES.length, 64);
  assert.equal(INSTALLABLE_CASES.length + UNSUPPORTED_CASES.length, ALL_CATALOG_CASES.length);
  assert.ok(INSTALLABLE_CASES.some(item => item.repo === "Bitnami" && item.chart === "airflow"));
  assert.ok(UNSUPPORTED_CASES.some(item => item.repo === "ingress-nginx" && item.chart === "ingress-nginx"));
});

test("catalog validation rejects mismatched repository keys", () => {
  assert.throws(
    () => validateCatalogPartition({Bitnami: "nginx"}, {}),
    /repository keys do not match/
  );
});

test("catalog validation rejects unsupported charts absent from the catalog", () => {
  assert.throws(
    () => validateCatalogPartition({Bitnami: "nginx"}, {Bitnami: "missing-chart"}),
    /missing-chart.*is not present/
  );
});

test("catalog validation rejects duplicate unsupported charts", () => {
  assert.throws(
    () => validateCatalogPartition({Bitnami: "nginx"}, {Bitnami: "nginx nginx"}),
    /unsupported list.*duplicate/
  );
});

test("catalog snapshot comparison rejects charts missing from the current repository", () => {
  const currentCatalog = Object.fromEntries(
    ["Bitnami", "Rancher", "ingress-nginx"].map(repo => [
      repo,
      ALL_CATALOG_CASES.filter(item => item.repo === repo).map(item => item.chart),
    ])
  );
  currentCatalog.Bitnami = currentCatalog.Bitnami.filter(chart => chart !== "nginx");

  assert.throws(() => assertCatalogSnapshot(currentCatalog), /Bitnami catalog snapshot differs/);
});
