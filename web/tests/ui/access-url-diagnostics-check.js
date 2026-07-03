const assert = require("assert");
const {
  ACCESS_URL_FAILURE_CATEGORIES,
  ACCESS_URL_FAILURE_SCOPES,
  ACCESS_URL_TYPES,
  classifyAccessUrlFailure,
  formatAccessUrlFailure,
  isReportableAccessUrlCategory,
  shouldFailAccessUrlOutcome,
  summarizeAccessUrlOutcomes,
} = require("./access-url-diagnostics");

function expectCategory(name, input, expectedCategory) {
  const actualCategory = classifyAccessUrlFailure(input).category;
  assert.strictEqual(
    actualCategory,
    expectedCategory,
    `${name}: expected ${expectedCategory}, got ${actualCategory}`
  );
}

assert.ok(
  !Object.values(ACCESS_URL_FAILURE_CATEGORIES).includes("legacy-category"),
  "ACCESS_URL_FAILURE_CATEGORIES must not contain a catch-all fallback category"
);
assert.deepStrictEqual(
  Object.values(ACCESS_URL_FAILURE_CATEGORIES).sort(),
  [
    ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC,
    ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC,
    ACCESS_URL_FAILURE_CATEGORIES.DOMAIN_ACCESS_URL_UNREACHABLE,
    ACCESS_URL_FAILURE_CATEGORIES.NODE_PORT_UNREACHABLE,
    ACCESS_URL_FAILURE_CATEGORIES.SERVICE_ACCESS_URL_NOT_RENDERED,
    ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC,
  ].sort(),
  "the bottom-level classifier should stay focused on six broad, actionable buckets"
);

expectCategory(
  "connection refused is a node port listener/routing problem",
  {error: new Error("net::ERR_CONNECTION_REFUSED at http://192.168.250.2:31080")},
  ACCESS_URL_FAILURE_CATEGORIES.NODE_PORT_UNREACHABLE
);

expectCategory(
  "connection refused wins when a message also mentions timeout",
  {error: new Error("request timeout after connection refused at http://192.168.250.2:31080")},
  ACCESS_URL_FAILURE_CATEGORIES.NODE_PORT_UNREACHABLE
);

expectCategory(
  "connection refused wins when a message also mentions DNS",
  {error: new Error("connection refused while resolving via DNS server at http://192.168.250.2:31080")},
  ACCESS_URL_FAILURE_CATEGORIES.NODE_PORT_UNREACHABLE
);

expectCategory(
  "timeout is a network path problem",
  {error: new Error("page.goto: Timeout 10000ms exceeded")},
  ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC
);

expectCategory(
  "browser DNS failure is grouped with CI/network diagnostics",
  {error: new Error("net::ERR_NAME_NOT_RESOLVED")},
  ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC
);

expectCategory(
  "domain Access URL DNS failure is grouped with domain/ingress reachability",
  {accessUrlType: ACCESS_URL_TYPES.DOMAIN, error: new Error("net::ERR_NAME_NOT_RESOLVED")},
  ACCESS_URL_FAILURE_CATEGORIES.DOMAIN_ACCESS_URL_UNREACHABLE
);

expectCategory(
  "domain Access URL connection refused is grouped with domain/ingress reachability",
  {accessUrlType: ACCESS_URL_TYPES.DOMAIN, error: new Error("net::ERR_CONNECTION_REFUSED")},
  ACCESS_URL_FAILURE_CATEGORIES.DOMAIN_ACCESS_URL_UNREACHABLE
);

expectCategory(
  "domain Access URL missing from Deployments is a rendered URL candidate",
  {
    serviceUrlMissing: true,
    accessUrlType: ACCESS_URL_TYPES.DOMAIN,
    serviceType: "ClusterIP",
    detail: "Deployment page row did not render Ingress domain Access URL",
  },
  ACCESS_URL_FAILURE_CATEGORIES.SERVICE_ACCESS_URL_NOT_RENDERED
);

expectCategory(
  "DNS wins when a message also mentions timeout",
  {error: new Error("timeout while resolving via DNS server")},
  ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC
);

expectCategory(
  "empty successful response is an application workload problem",
  {responseStatus: 200, body: "   "},
  ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC
);

expectCategory(
  "http 503 is an application workload problem",
  {responseStatus: 503, body: "service unavailable"},
  ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC
);

expectCategory(
  "missing response is grouped with CI/network diagnostics",
  {responseStatus: null, body: ""},
  ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC
);

expectCategory(
  "null input is treated as no HTTP response instead of throwing",
  null,
  ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC
);

expectCategory(
  "status zero is treated as no HTTP response",
  {responseStatus: 0, body: ""},
  ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC
);

expectCategory(
  "empty string status is treated as no HTTP response",
  {responseStatus: "", body: ""},
  ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC
);

expectCategory(
  "string status zero is treated as no HTTP response",
  {responseStatus: "0", body: ""},
  ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC
);

expectCategory(
  "NodePort service URL not rendered is categorized separately",
  {serviceUrlMissing: true, serviceType: "NodePort", detail: "service row did not expose a matching Access URL"},
  ACCESS_URL_FAILURE_CATEGORIES.SERVICE_ACCESS_URL_NOT_RENDERED
);

expectCategory(
  "ClusterIP service without an Access URL is an app workload diagnostic",
  {serviceUrlMissing: true, serviceType: "ClusterIP", detail: "service type ClusterIP does not expose a NodePort Access URL"},
  ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC
);

expectCategory(
  "workload evidence can override a browser connection error",
  {appWorkloadDiagnostic: true, detail: "NodePort service has no running pods matching its selector"},
  ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC
);

expectCategory(
  "test harness evidence stays out of app-store reportable categories",
  {testHarnessDiagnostic: true, detail: "release context is missing service and release names"},
  ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC
);

expectCategory(
  "unmatched browser navigation errors stay in a defined diagnostic class",
  {error: new Error("something unexpected")},
  ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC
);

expectCategory(
  "successful response that reaches failure handling is a validation mismatch",
  {responseStatus: 200, body: "sample is ready"},
  ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC
);

expectCategory(
  "string status 200 follows the same validation path as numeric 200",
  {responseStatus: "200", body: "sample is ready"},
  ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC
);

expectCategory(
  "non-numeric status with detail is a browser navigation error",
  {responseStatus: "NetworkError", detail: "browser aborted before receiving a status"},
  ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC
);

const objectErrorClassification = classifyAccessUrlFailure({error: {code: "ERR_FAILED", reason: "browser aborted"}});
assert.strictEqual(objectErrorClassification.category, ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC);
assert.match(objectErrorClassification.detail, /ERR_FAILED/);
assert.doesNotMatch(objectErrorClassification.detail, /\[object Object\]/);

const errorLikeClassification = classifyAccessUrlFailure({error: {message: "net::ERR_CONNECTION_REFUSED"}});
assert.strictEqual(errorLikeClassification.category, ACCESS_URL_FAILURE_CATEGORIES.NODE_PORT_UNREACHABLE);
assert.match(errorLikeClassification.detail, /ERR_CONNECTION_REFUSED/);

const stackOnlyError = new Error("");
stackOnlyError.stack = "stack-only browser failure";
const stackOnlyClassification = classifyAccessUrlFailure({error: stackOnlyError});
assert.strictEqual(stackOnlyClassification.category, ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC);
assert.match(stackOnlyClassification.detail, /stack-only browser failure/);

const emptyError = new Error("");
emptyError.stack = "";
const emptyErrorClassification = classifyAccessUrlFailure({error: emptyError});
assert.strictEqual(emptyErrorClassification.category, ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC);
assert.match(emptyErrorClassification.detail, /Error/);

const circularError = {code: "ERR_CIRCULAR"};
circularError.self = circularError;
const circularClassification = classifyAccessUrlFailure({error: circularError});
assert.strictEqual(circularClassification.category, ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC);
assert.match(circularClassification.detail, /ERR_CIRCULAR/);
assert.match(circularClassification.detail, /\[Circular\]/);

const sharedCause = {code: "ERR_SHARED_CAUSE"};
const sharedReferenceClassification = classifyAccessUrlFailure({error: {left: sharedCause, right: sharedCause}});
assert.strictEqual(sharedReferenceClassification.category, ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC);
assert.match(sharedReferenceClassification.detail, /ERR_SHARED_CAUSE/);
assert.doesNotMatch(sharedReferenceClassification.detail, /\[Circular\]/);

const unstringifiableError = {
  toJSON() {
    throw new Error("json failed");
  },
  toString() {
    throw new Error("string failed");
  },
};
const unstringifiableClassification = classifyAccessUrlFailure({error: unstringifiableError});
assert.strictEqual(unstringifiableClassification.category, ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC);
assert.match(unstringifiableClassification.detail, /\[Unstringifiable value\]/);

const emptyObjectErrorClassification = classifyAccessUrlFailure({error: {}});
assert.strictEqual(emptyObjectErrorClassification.category, ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC);
assert.doesNotMatch(emptyObjectErrorClassification.detail, /\[object Object\]/);

const formatted = formatAccessUrlFailure({
  accessUrl: "http://192.168.250.2:31080",
  source: "app-store",
  repoName: "podinfo",
  chartName: "podinfo",
  chartVersion: "6.14.0",
  releaseName: "podinfo-ok-demo",
  deploymentName: "podinfo",
  serviceName: "podinfo",
  serviceType: "NodePort",
  attemptCount: 7,
  classification: classifyAccessUrlFailure({error: new Error("net::ERR_CONNECTION_REFUSED")}),
});

assert.match(formatted, /category=node-port-unreachable/);
assert.match(formatted, /source=app-store/);
assert.match(formatted, /repo=podinfo/);
assert.match(formatted, /chart=podinfo@6\.14\.0/);
assert.match(formatted, /release=default\/podinfo-ok-demo/);
assert.match(formatted, /deployment=default\/podinfo/);
assert.match(formatted, /service=default\/podinfo:NodePort/);
assert.match(formatted, /url=http:\/\/192\.168\.250\.2:31080/);
assert.match(formatted, /attempts=7/);
assert.match(formatted, /detail=net::ERR_CONNECTION_REFUSED/);
assert.match(formatted, /hint=.+/);

const formattedEscapedDetail = formatAccessUrlFailure({
  classification: {
    category: ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC,
    detail: "connection refused; phase=browser path=C:\\temp\nnext",
    hint: "check trace; retry=false",
  },
});
assert.match(formattedEscapedDetail, /detail=connection refused\\x3b phase\\x3dbrowser path\\x3dC:\\x5ctemp next/);
assert.match(formattedEscapedDetail, /hint=check trace\\x3b retry\\x3dfalse/);

const formattedEscapedFields = formatAccessUrlFailure({
  accessUrl: "http://host/?a=1;b=2",
  source: "app=store",
  repoName: "repo;one",
  chartName: "chart=name",
  chartVersion: "0;1",
  releaseName: "rel=1",
  namespace: "ns;1",
  classification: classifyAccessUrlFailure({responseStatus: 503, body: "nope"}),
});
assert.match(formattedEscapedFields, /source=app\\x3dstore/);
assert.match(formattedEscapedFields, /repo=repo\\x3bone/);
assert.match(formattedEscapedFields, /chart=chart\\x3dname@0\\x3b1/);
assert.match(formattedEscapedFields, /release=ns\\x3b1\/rel\\x3d1/);
assert.match(formattedEscapedFields, /url=http:\/\/host\/\?a\\x3d1\\x3bb\\x3d2/);

const formattedWithoutClassification = formatAccessUrlFailure({detail: "browser crashed before classification"});
assert.match(formattedWithoutClassification, /category=test-harness-diagnostic/);
assert.match(formattedWithoutClassification, /detail=browser crashed before classification/);
assert.doesNotMatch(formattedWithoutClassification, /category=legacy-category/);

const formattedKnownCategoryWithoutDetail = formatAccessUrlFailure({
  detail: "fallback detail stays visible",
  classification: {
    category: ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC,
  },
});
assert.match(formattedKnownCategoryWithoutDetail, /category=ci-or-network-diagnostic/);
assert.match(formattedKnownCategoryWithoutDetail, /detail=fallback detail stays visible/);

const formattedLegacyClassification = formatAccessUrlFailure({
  classification: {
    category: "legacy-category",
    detail: "legacy caller returned a removed category",
  },
});
assert.match(formattedLegacyClassification, /category=test-harness-diagnostic/);
assert.doesNotMatch(formattedLegacyClassification, /category=legacy-category/);

const formattedZeroAttempts = formatAccessUrlFailure({
  classification: classifyAccessUrlFailure({responseStatus: null}),
  attemptCount: 0,
});
assert.match(formattedZeroAttempts, /attempts=0/);

const emptySummary = {
  totalCount: 0,
  reachableCount: 0,
  failureCount: 0,
  reportableFailureCount: 0,
  diagnosticFailureCount: 0,
  categories: [],
  diagnosticCategories: [],
};
assert.deepStrictEqual(summarizeAccessUrlOutcomes(), emptySummary);
assert.deepStrictEqual(summarizeAccessUrlOutcomes(null), emptySummary);
assert.deepStrictEqual(summarizeAccessUrlOutcomes([]), emptySummary);

const reportableCategories = Object.values(ACCESS_URL_FAILURE_CATEGORIES)
  .filter(category => isReportableAccessUrlCategory(category))
  .sort();
assert.deepStrictEqual(
  reportableCategories,
  [
    ACCESS_URL_FAILURE_CATEGORIES.NODE_PORT_UNREACHABLE,
    ACCESS_URL_FAILURE_CATEGORIES.SERVICE_ACCESS_URL_NOT_RENDERED,
  ].sort(),
  "only stable app-store/CasOS candidates should be reportable categories"
);

assert.strictEqual(
  shouldFailAccessUrlOutcome({
    reachable: false,
    accessUrl: "http://demo-release.casos.invalid",
    classification: classifyAccessUrlFailure({
      accessUrlType: ACCESS_URL_TYPES.DOMAIN,
      error: new Error("net::ERR_NAME_NOT_RESOLVED"),
    }),
  }),
  true,
  "a rendered Access URL that cannot be opened must fail the UI gate even when its category is diagnostic"
);
assert.strictEqual(
  shouldFailAccessUrlOutcome({
    reachable: false,
    classification: classifyAccessUrlFailure({
      appWorkloadDiagnostic: true,
      detail: "release did not create a Service that can be associated with its Deployments",
    }),
  }),
  false,
  "diagnostics without a rendered Access URL should not pretend to be a failed click/open result"
);

assert.strictEqual(
  classifyAccessUrlFailure({error: new Error("page.goto: Timeout 10000ms exceeded")}).scope,
  ACCESS_URL_FAILURE_SCOPES.CI_OR_ENVIRONMENT
);
assert.strictEqual(
  classifyAccessUrlFailure({responseStatus: 503, body: "service unavailable"}).scope,
  ACCESS_URL_FAILURE_SCOPES.APP_WORKLOAD
);
assert.strictEqual(
  classifyAccessUrlFailure({responseStatus: 200, body: "sample is ready"}).scope,
  ACCESS_URL_FAILURE_SCOPES.TEST_HARNESS
);

const allReachableSummary = summarizeAccessUrlOutcomes([
  {reachable: true, chartName: "podinfo", releaseName: "podinfo-ok-one"},
  {reachable: true, chartName: "podinfo", releaseName: "podinfo-ok-two"},
]);
assert.strictEqual(allReachableSummary.totalCount, 2);
assert.strictEqual(allReachableSummary.reachableCount, 2);
assert.strictEqual(allReachableSummary.failureCount, 0);
assert.deepStrictEqual(allReachableSummary.categories, []);
assert.deepStrictEqual(allReachableSummary.diagnosticCategories, []);

const observedOnlySummary = summarizeAccessUrlOutcomes([
  {
    reachable: true,
    chartName: "podinfo",
    chartVersion: "6.14.0",
    releaseName: "podinfo-ok-demo",
    namespace: "default",
    accessUrl: "http://192.168.250.2:31080",
  },
  {
    reachable: false,
    chartName: "casos-access-url-clusterip",
    chartVersion: "0.1.0",
    releaseName: "as-ci-demo",
    namespace: "default",
    classification: classifyAccessUrlFailure({
      serviceUrlMissing: true,
      serviceType: "ClusterIP",
      detail: "service row did not expose a matching Access URL",
    }),
  },
  {
    reachable: false,
    chartName: "casos-access-url-nodeport-missing",
    chartVersion: "0.1.0",
    releaseName: "as-missing-demo",
    namespace: "default",
    deploymentName: "as-missing-demo",
    serviceName: "as-missing-demo",
    serviceType: "NodePort",
    classification: classifyAccessUrlFailure({
      serviceUrlMissing: true,
      serviceType: "NodePort",
      detail: "NodePort service row did not expose a matching Access URL",
    }),
  },
  {
    reachable: false,
    chartName: "casos-access-url-no-endpoints",
    chartVersion: "0.1.0",
    releaseName: "as-ne-demo",
    namespace: "default",
    deploymentName: "as-ne-demo",
    serviceName: "as-ne-demo",
    serviceType: "NodePort",
    accessUrl: "http://192.168.250.2:31081",
    attemptCount: 2,
    classification: classifyAccessUrlFailure({
      error: new Error("net::ERR_CONNECTION_REFUSED at http://192.168.250.2:31081"),
    }),
  },
]);

assert.deepStrictEqual(
  observedOnlySummary.categories.map(item => item.category).sort(),
  [
    ACCESS_URL_FAILURE_CATEGORIES.NODE_PORT_UNREACHABLE,
    ACCESS_URL_FAILURE_CATEGORIES.SERVICE_ACCESS_URL_NOT_RENDERED,
  ].sort()
);
assert.deepStrictEqual(
  observedOnlySummary.diagnosticCategories.map(item => item.category),
  [ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC]
);
assert.strictEqual(observedOnlySummary.failureCount, 3);
assert.strictEqual(observedOnlySummary.reachableCount, 1);
assert.strictEqual(observedOnlySummary.reportableFailureCount, 2);
assert.strictEqual(observedOnlySummary.diagnosticFailureCount, 1);
assert.ok(
  observedOnlySummary.categories.every(item => item.examples.length > 0),
  "every reported category must include at least one observed example"
);
assert.ok(
  observedOnlySummary.diagnosticCategories.every(item => item.examples.length > 0),
  "every diagnostic category must include at least one observed example"
);
assert.ok(
  observedOnlySummary.categories.every(item => item.category !== ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC),
  "unobserved categories must not be listed"
);
assert.ok(
  observedOnlySummary.categories
    .flatMap(item => item.examples)
    .every(example => example.chartName && example.releaseName),
  "summary examples should identify the app-store chart and release"
);
assert.ok(
  observedOnlySummary.categories
    .flatMap(item => item.examples)
    .every(example => example.deploymentName && example.serviceName),
  "reportable summary examples should identify the Deployment row and backing Service"
);

const repeatedFailureSummary = summarizeAccessUrlOutcomes(Array.from({length: 7}, (_, index) => ({
  reachable: false,
  chartName: `casos-access-url-no-endpoints-${index}`,
  releaseName: `as-ne-${index}`,
  classification: classifyAccessUrlFailure({error: new Error("net::ERR_CONNECTION_REFUSED")}),
})));
assert.strictEqual(repeatedFailureSummary.failureCount, 7);
assert.strictEqual(repeatedFailureSummary.categories.length, 1);
assert.strictEqual(repeatedFailureSummary.categories[0].category, ACCESS_URL_FAILURE_CATEGORIES.NODE_PORT_UNREACHABLE);
assert.strictEqual(repeatedFailureSummary.categories[0].count, 7);
assert.strictEqual(repeatedFailureSummary.categories[0].examples.length, 5);

const diagnosticOnlySummary = summarizeAccessUrlOutcomes([
  {
    reachable: false,
    chartName: "casos-probe-budget-demo",
    releaseName: "as-probe-demo",
    classification: classifyAccessUrlFailure({error: new Error("page.goto: Timeout 10000ms exceeded")}),
  },
  {
    reachable: false,
    chartName: "casos-access-url-http-503",
    releaseName: "as-http-demo",
    classification: classifyAccessUrlFailure({responseStatus: 503, body: "service unavailable"}),
  },
  {
    reachable: false,
    chartName: "casos-access-url-empty",
    releaseName: "as-empty-demo",
    classification: classifyAccessUrlFailure({responseStatus: 200, body: "   "}),
  },
]);
assert.strictEqual(diagnosticOnlySummary.failureCount, 3);
assert.strictEqual(diagnosticOnlySummary.reportableFailureCount, 0);
assert.deepStrictEqual(diagnosticOnlySummary.categories, []);
assert.deepStrictEqual(
  diagnosticOnlySummary.diagnosticCategories.map(item => item.category).sort(),
  [
    ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC,
    ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC,
  ].sort()
);
assert.ok(
  diagnosticOnlySummary.diagnosticCategories.every(item => item.examples.length > 0 && item.scope),
  "diagnostic-only categories should retain examples and scope for debugging without reporting them as CasOS issues"
);

const legacyCategorySummary = summarizeAccessUrlOutcomes([
  {
    reachable: false,
    chartName: "legacy-chart",
    releaseName: "legacy-release",
    classification: {
      category: "legacy-category",
      detail: "legacy caller returned a removed category",
    },
  },
]);
assert.deepStrictEqual(legacyCategorySummary.categories, []);
assert.deepStrictEqual(
  legacyCategorySummary.diagnosticCategories.map(item => item.category),
  [ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC]
);
assert.ok(
  legacyCategorySummary.diagnosticCategories
    .flatMap(item => item.examples)
    .every(example => example.detail.includes("legacy caller")),
  "legacy classifications should keep their detail while being mapped to a defined category"
);

console.log("access URL diagnostics checks passed");
