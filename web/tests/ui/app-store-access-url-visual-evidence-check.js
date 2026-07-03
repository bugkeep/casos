const assert = require("assert");
const {
  ACCESS_URL_FAILURE_CATEGORIES,
  ACCESS_URL_TYPES,
  classifyAccessUrlFailure,
} = require("./access-url-diagnostics");
const {
  buildAccessUrlFailureAnnotations,
  VISUAL_SUMMARY_MAX_LENGTH,
} = require("./app-store-access-url-visual-evidence");

const annotations = buildAccessUrlFailureAnnotations([
  {
    reachable: false,
    namespace: "default",
    chartName: "podinfo",
    releaseName: "podinfo-domain-demo",
    deploymentName: "podinfo-domain-demo",
    serviceName: "podinfo-domain-demo",
    serviceType: "ClusterIP",
    accessUrlType: ACCESS_URL_TYPES.DOMAIN,
    accessUrl: "http://podinfo-domain-demo.casos.invalid",
    attemptCount: 2,
    classification: classifyAccessUrlFailure({
      accessUrlType: ACCESS_URL_TYPES.DOMAIN,
      error: new Error("net::ERR_NAME_NOT_RESOLVED"),
    }),
  },
  {
    reachable: false,
    namespace: "default",
    chartName: "echo-server",
    releaseName: "echo-ci-demo",
    deploymentName: "echo-ci-demo",
    serviceName: "echo-ci-demo",
    serviceType: "NodePort",
    accessUrlType: ACCESS_URL_TYPES.NODEPORT,
    accessUrl: "http://192.168.250.2:31080",
    attemptCount: 4,
    classification: classifyAccessUrlFailure({
      error: new Error("net::ERR_CONNECTION_REFUSED"),
    }),
  },
]);

assert.strictEqual(annotations.length, 2);
assert.strictEqual(annotations[0].title, "Domain Access URL failed");
assert.strictEqual(annotations[0].category, ACCESS_URL_FAILURE_CATEGORIES.DOMAIN_ACCESS_URL_UNREACHABLE);
assert.strictEqual(annotations[0].rowKey, "default/podinfo-domain-demo");
assert.match(annotations[0].detail, /ERR_NAME_NOT_RESOLVED/);
assert.match(annotations[0].summary, /deployment=default\/podinfo-domain-demo/);
assert.match(annotations[0].summary, /url=http:\/\/podinfo-domain-demo\.casos\.invalid/);
assert.ok(
  annotations[0].summary.length <= VISUAL_SUMMARY_MAX_LENGTH,
  "visual summaries should fit inside the screenshot label"
);

assert.strictEqual(annotations[1].title, "NodePort Access URL failed");
assert.strictEqual(annotations[1].category, ACCESS_URL_FAILURE_CATEGORIES.NODE_PORT_UNREACHABLE);
assert.strictEqual(annotations[1].rowKey, "default/echo-ci-demo");
assert.match(annotations[1].detail, /ERR_CONNECTION_REFUSED/);
assert.match(annotations[1].summary, /attempts=4/);

const legacyAnnotations = buildAccessUrlFailureAnnotations([
  {
    reachable: false,
    namespace: "default",
    deploymentName: "legacy-demo",
    accessUrl: "http://legacy-demo.casos.invalid",
    classification: {
      category: "legacy-category",
      detail: "legacy caller returned a removed category",
    },
  },
]);

assert.strictEqual(legacyAnnotations.length, 1);
assert.strictEqual(legacyAnnotations[0].category, ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC);
assert.doesNotMatch(legacyAnnotations[0].summary, /category=legacy-category/);
assert.match(legacyAnnotations[0].summary, /legacy caller returned a removed category/);

console.log("app store access URL visual evidence checks passed");
