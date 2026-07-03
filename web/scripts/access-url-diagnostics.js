const ACCESS_URL_FAILURE_CATEGORIES = Object.freeze({
  SERVICE_ACCESS_URL_NOT_RENDERED: "service-access-url-not-rendered",
  NODE_PORT_UNREACHABLE: "node-port-unreachable",
  DOMAIN_ACCESS_URL_UNREACHABLE: "domain-access-url-unreachable",
  CI_OR_NETWORK_DIAGNOSTIC: "ci-or-network-diagnostic",
  APP_WORKLOAD_DIAGNOSTIC: "app-workload-diagnostic",
  TEST_HARNESS_DIAGNOSTIC: "test-harness-diagnostic",
});

const ACCESS_URL_TYPES = Object.freeze({
  DOMAIN: "domain",
  NODEPORT: "nodeport",
});

const ACCESS_URL_FAILURE_SCOPES = Object.freeze({
  APP_STORE_CASOS_CANDIDATE: "app-store-casos-candidate",
  APP_WORKLOAD: "app-workload",
  CI_OR_ENVIRONMENT: "ci-or-environment",
  TEST_HARNESS: "test-harness",
});

const CATEGORY_HINTS = Object.freeze({
  [ACCESS_URL_FAILURE_CATEGORIES.SERVICE_ACCESS_URL_NOT_RENDERED]: "The app-store release did not produce a visible Access URL in the Deployments page Access URL column.",
  [ACCESS_URL_FAILURE_CATEGORIES.NODE_PORT_UNREACHABLE]: "The worker node IP was reachable, but the NodePort listener or kube-proxy routing rejected the connection.",
  [ACCESS_URL_FAILURE_CATEGORIES.DOMAIN_ACCESS_URL_UNREACHABLE]: "The Deployments page rendered a domain Access URL, but the browser could not open it; check the Ingress host, DNS, ingress controller, and routing to the worker cluster.",
  [ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC]: "The browser did not get a stable HTTP response for an environment-shaped reason; check VM/runner routing, DNS, bridge/tap, firewall, and browser navigation details.",
  [ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC]: "The installed chart did not expose a healthy workload response; check the service type, selector, pod logs, endpoints, and response body.",
  [ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC]: "The response reached the test but did not match the expected success condition; check the test expectation and access-url validation logic.",
});

const CATEGORY_METADATA = Object.freeze({
  [ACCESS_URL_FAILURE_CATEGORIES.SERVICE_ACCESS_URL_NOT_RENDERED]: {
    scope: ACCESS_URL_FAILURE_SCOPES.APP_STORE_CASOS_CANDIDATE,
    reportable: true,
    reportReason: "The app-store install completed, but CasOS did not expose an Access URL for the installed service shape in the Deployments page.",
  },
  [ACCESS_URL_FAILURE_CATEGORIES.NODE_PORT_UNREACHABLE]: {
    scope: ACCESS_URL_FAILURE_SCOPES.APP_STORE_CASOS_CANDIDATE,
    reportable: true,
    reportReason: "The node IP and port were reachable enough to reject the connection, so this is inside the deployed cluster/service path rather than a runner routing timeout.",
  },
  [ACCESS_URL_FAILURE_CATEGORIES.DOMAIN_ACCESS_URL_UNREACHABLE]: {
    scope: ACCESS_URL_FAILURE_SCOPES.APP_WORKLOAD,
    reportable: false,
    reportReason: "The URL was rendered from Ingress/domain evidence, but DNS, ingress-controller, or host routing evidence is needed before treating it as a CasOS product issue.",
  },
  [ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC]: {
    scope: ACCESS_URL_FAILURE_SCOPES.CI_OR_ENVIRONMENT,
    reportable: false,
    reportReason: "Runner, VM, browser, DNS, and generic no-response failures are too noisy to count as CasOS issues without extra evidence.",
  },
  [ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC]: {
    scope: ACCESS_URL_FAILURE_SCOPES.APP_WORKLOAD,
    reportable: false,
    reportReason: "The observed evidence belongs to the installed chart workload or service shape unless separate CasOS routing evidence exists.",
  },
  [ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC]: {
    scope: ACCESS_URL_FAILURE_SCOPES.TEST_HARNESS,
    reportable: false,
    reportReason: "This means the test expectation did not match a reachable response, so it is a test harness signal rather than an app-store issue.",
  },
});

const ACCESS_URL_FAILURE_CATEGORY_VALUES = new Set(Object.values(ACCESS_URL_FAILURE_CATEGORIES));

function classifyAccessUrlFailure(input) {
  input = input || {};
  const message = readFailureMessage(input);
  const lowerMessage = message.toLowerCase();
  const status = normalizeResponseStatus(input.responseStatus);
  const body = String(input.body || "");

  // Explicit evidence gathered from Services/Pods or the harness beats browser-level symptoms.
  if (input.appWorkloadDiagnostic) {
    return failureCategory(
      ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC,
      input.detail || "installed chart workload did not expose a healthy NodePort response"
    );
  }

  if (input.testHarnessDiagnostic) {
    return failureCategory(
      ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC,
      input.detail || "access URL test harness reported an internal diagnostic"
    );
  }

  if (input.serviceUrlMissing) {
    if (isDomainAccessUrlInput(input)) {
      return failureCategory(
        ACCESS_URL_FAILURE_CATEGORIES.SERVICE_ACCESS_URL_NOT_RENDERED,
        input.detail ?? "Ingress domain Access URL did not render in the Deployments page"
      );
    }
    const serviceType = normalizeServiceType(input.serviceType);
    if (serviceType && serviceType !== "nodeport") {
      const detail = input.detail || `service type ${input.serviceType} does not expose a NodePort Access URL`;
      return failureCategory(ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC, detail);
    }
    return failureCategory(
      ACCESS_URL_FAILURE_CATEGORIES.SERVICE_ACCESS_URL_NOT_RENDERED,
      input.detail ?? "NodePort service row did not expose a matching Access URL"
    );
  }

  // Keep specific failures before broad ones when browser messages contain multiple indicators.
  // This order is intentional and covered by diagnostics checks.
  if (input.domainAccessUrlDiagnostic) {
    return failureCategory(
      ACCESS_URL_FAILURE_CATEGORIES.DOMAIN_ACCESS_URL_UNREACHABLE,
      input.detail || message || "domain Access URL could not be opened"
    );
  }

  const domainNavigationFailure = classifyDomainAccessUrlNavigationFailure(input, lowerMessage, status, message);
  if (domainNavigationFailure) {
    return domainNavigationFailure;
  }

  if (/connection refused|err_connection_refused|connection reset|err_connection_reset|socket hang up|econnrefused|econnreset/.test(lowerMessage)) {
    return failureCategory(ACCESS_URL_FAILURE_CATEGORIES.NODE_PORT_UNREACHABLE, message);
  }

  if (/err_name_not_resolved|\bdns\b|name or service not known|invalid url|cannot navigate to invalid url/.test(lowerMessage)) {
    return failureCategory(ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC, message);
  }

  if (/timeout|timed out|err_connection_timed_out|net::err_timed_out|no route to host|host unreachable|network is unreachable/.test(lowerMessage)) {
    return failureCategory(ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC, message);
  }

  if (status === undefined || status === 0) {
    if (message) {
      return failureCategory(ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC, message);
    }
    return failureCategory(ACCESS_URL_FAILURE_CATEGORIES.CI_OR_NETWORK_DIAGNOSTIC, "no HTTP response was returned");
  }

  if (status !== 200) {
    const detail = body.trim()
      ? `HTTP ${status}: ${trimDetail(body)}`
      : `HTTP ${status}`;
    return failureCategory(ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC, detail);
  }

  if (!body.trim()) {
    return failureCategory(ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC, "HTTP 200 with an empty response body");
  }

  return failureCategory(
    ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC,
    message || `response validation reached failure handling after HTTP ${status}`
  );
}

function isKnownAccessUrlFailureCategory(category) {
  return ACCESS_URL_FAILURE_CATEGORY_VALUES.has(category);
}

function normalizeAccessUrlClassification(classification, fallbackDetail) {
  if (classification?.category && isKnownAccessUrlFailureCategory(classification.category)) {
    const metadata = getAccessUrlFailureMetadata(classification.category);
    const detail = stringifyDiagnosticValue(classification.detail)
      || stringifyDiagnosticValue(fallbackDetail)
      || "access URL failure classification reached fallback detail";
    return {
      category: classification.category,
      scope: classification.scope || metadata.scope,
      reportable: metadata.reportable,
      reportReason: classification.reportReason || metadata.reportReason,
      detail: trimDetail(detail),
      hint: classification.hint || CATEGORY_HINTS[classification.category],
    };
  }

  return failureCategory(
    ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC,
    stringifyDiagnosticValue(classification?.detail)
      || stringifyDiagnosticValue(fallbackDetail)
      || "access URL failure classification was normalized by the test harness"
  );
}

function readFailureMessage(input) {
  return stringifyDiagnosticValue(input.error?.message)
    || stringifyDiagnosticValue(input.error)
    || stringifyDiagnosticValue(input.detail);
}

function normalizeResponseStatus(value) {
  if (value === null || value === undefined || value === "") {
    return undefined;
  }
  const numberValue = Number(value);
  return Number.isNaN(numberValue) ? undefined : numberValue;
}

function normalizeServiceType(value) {
  return String(value || "").trim().toLowerCase();
}

function isDomainAccessUrlInput(input) {
  return String(input?.accessUrlType || "").toLowerCase() === ACCESS_URL_TYPES.DOMAIN;
}

function classifyDomainAccessUrlNavigationFailure(input, lowerMessage, status, message) {
  if (!isDomainAccessUrlInput(input)) {
    return null;
  }
  if (/connection refused|err_connection_refused|connection reset|err_connection_reset|socket hang up|econnrefused|econnreset/.test(lowerMessage) ||
    /err_name_not_resolved|\bdns\b|name or service not known|invalid url|cannot navigate to invalid url/.test(lowerMessage) ||
    /timeout|timed out|err_connection_timed_out|net::err_timed_out|no route to host|host unreachable|network is unreachable/.test(lowerMessage) ||
    status === undefined ||
    status === 0) {
    return failureCategory(
      ACCESS_URL_FAILURE_CATEGORIES.DOMAIN_ACCESS_URL_UNREACHABLE,
      input.detail || message || "domain Access URL did not return an HTTP response"
    );
  }
  return null;
}

function shouldFailAccessUrlOutcome(outcome) {
  if (!outcome || outcome.reachable) {
    return false;
  }
  const classification = normalizeAccessUrlClassification(
    outcome.classification,
    outcome.detail || "access URL outcome failed without a classification"
  );
  return Boolean(outcome.accessUrl) || classification.reportable;
}

function stringifyDiagnosticValue(value) {
  if (value === null || value === undefined) {
    return "";
  }
  if (typeof value === "string") {
    return value;
  }
  if (typeof value === "object") {
    if (isErrorLike(value)) {
      return value.message || value.stack || stringifyFallback(value);
    }
    try {
      const json = stringifyJson(value);
      if (json && json !== "{}") {
        return json;
      }
      return "";
    } catch (error) {
      return stringifyFallback(value);
    }
  }
  return stringifyFallback(value);
}

function isErrorLike(value) {
  return value instanceof Error || Boolean(value && typeof value.message === "string");
}

function stringifyFallback(value) {
  let fallback;
  try {
    fallback = String(value);
  } catch (error) {
    return "[Unstringifiable value]";
  }
  return fallback === "[object Object]" ? "" : fallback;
}

function failureCategory(category, detail) {
  const metadata = getAccessUrlFailureMetadata(category);
  return {
    category,
    scope: metadata.scope,
    reportable: metadata.reportable,
    reportReason: metadata.reportReason,
    detail: trimDetail(detail),
    hint: CATEGORY_HINTS[category] || CATEGORY_HINTS[ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC],
  };
}

function getAccessUrlFailureMetadata(category) {
  return CATEGORY_METADATA[category] || CATEGORY_METADATA[ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC];
}

function isReportableAccessUrlCategory(category) {
  return Boolean(getAccessUrlFailureMetadata(category).reportable);
}

function formatAccessUrlFailure(params) {
  params = params || {};
  const {
    accessUrl,
    source,
    detail,
    repoName,
    chartName,
    chartVersion,
    releaseName,
    namespace,
    deploymentName,
    serviceName,
    serviceType,
    accessUrlType,
    ingressName,
    attemptCount,
    classification,
  } = params;
  const safeClassification = normalizeAccessUrlClassification(
    classification,
    detail || "access URL failure was formatted without a classification"
  );
  const parts = [
    "Access URL was not reachable",
    `category=${safeClassification.category}`,
    `source=${formatLogValue(source || "source-unspecified")}`,
  ];
  if (repoName) {
    parts.push(`repo=${formatLogValue(repoName)}`);
  }
  if (chartName) {
    const chartDisplay = chartVersion ? `${chartName}@${chartVersion}` : chartName;
    parts.push(`chart=${formatLogValue(chartDisplay)}`);
  }
  if (releaseName) {
    parts.push(`release=${formatLogValue(namespace || "default")}/${formatLogValue(releaseName)}`);
  }
  if (deploymentName) {
    parts.push(`deployment=${formatLogValue(namespace || "default")}/${formatLogValue(deploymentName)}`);
  }
  if (serviceName) {
    const serviceDisplay = serviceType ? `${serviceName}:${serviceType}` : serviceName;
    parts.push(`service=${formatLogValue(namespace || "default")}/${formatLogValue(serviceDisplay)}`);
  }
  if (accessUrlType) {
    parts.push(`target=${formatLogValue(accessUrlType)}`);
  }
  if (ingressName) {
    parts.push(`ingress=${formatLogValue(namespace || "default")}/${formatLogValue(ingressName)}`);
  }
  if (accessUrl) {
    parts.push(`url=${formatLogValue(accessUrl)}`);
  }
  if (attemptCount !== undefined && attemptCount !== null) {
    parts.push(`attempts=${attemptCount}`);
  }
  if (safeClassification.detail) {
    parts.push(`detail=${formatLogValue(safeClassification.detail)}`);
  }
  if (safeClassification.hint) {
    parts.push(`hint=${formatLogValue(safeClassification.hint)}`);
  }
  return parts.join("; ");
}

function summarizeAccessUrlOutcomes(outcomes) {
  const summary = {
    totalCount: 0,
    reachableCount: 0,
    failureCount: 0,
    reportableFailureCount: 0,
    diagnosticFailureCount: 0,
    categories: [],
    diagnosticCategories: [],
  };
  const categoriesByName = new Map();
  const diagnosticCategoriesByName = new Map();

  for (const outcome of outcomes || []) {
    summary.totalCount += 1;
    if (outcome?.reachable) {
      summary.reachableCount += 1;
      continue;
    }

    summary.failureCount += 1;
    const classification = normalizeAccessUrlClassification(
      outcome?.classification,
      outcome?.detail || "access URL outcome failed without a classification"
    );
    const category = classification.category;
    const reportable = isReportableAccessUrlCategory(category);
    if (reportable) {
      summary.reportableFailureCount += 1;
    } else {
      summary.diagnosticFailureCount += 1;
    }

    const target = reportable ? categoriesByName : diagnosticCategoriesByName;
    if (!target.has(category)) {
      target.set(category, {
        category,
        scope: classification.scope,
        reportable,
        reportReason: classification.reportReason,
        hint: classification.hint,
        count: 0,
        examples: [],
      });
    }

    const bucket = target.get(category);
    bucket.count += 1;
    if (bucket.examples.length < 5) {
      bucket.examples.push(compactAccessUrlExample(outcome, classification));
    }
  }

  summary.categories = Array.from(categoriesByName.values())
    .sort((left, right) => left.category.localeCompare(right.category));
  summary.diagnosticCategories = Array.from(diagnosticCategoriesByName.values())
    .sort((left, right) => left.category.localeCompare(right.category));
  return summary;
}

function compactAccessUrlExample(outcome, classification) {
  outcome = outcome || {};
  return {
    source: outcome.source,
    repoName: outcome.repoName,
    chartName: outcome.chartName,
    chartVersion: outcome.chartVersion,
    releaseName: outcome.releaseName,
    namespace: outcome.namespace,
    deploymentName: outcome.deploymentName,
    serviceName: outcome.serviceName,
    serviceType: outcome.serviceType,
    accessUrlType: outcome.accessUrlType,
    ingressName: outcome.ingressName,
    ingressHost: outcome.ingressHost,
    accessUrl: outcome.accessUrl,
    attemptCount: outcome.attemptCount,
    scope: classification.scope,
    reportable: classification.reportable,
    detail: classification.detail,
  };
}

function formatLogValue(value) {
  return String(value).replace(/[\\;=\x00-\x1f\x7f-\x9f]/g, char => (
    `\\x${char.charCodeAt(0).toString(16).padStart(2, "0")}`
  ));
}

function stringifyJson(value) {
  const ancestors = [];
  return JSON.stringify(value, function(_key, nestedValue) {
    if (nestedValue && typeof nestedValue === "object") {
      while (ancestors.length > 0 && ancestors[ancestors.length - 1] !== this) {
        ancestors.pop();
      }
      if (ancestors.includes(nestedValue)) {
        return "[Circular]";
      }
      ancestors.push(nestedValue);
    }
    return nestedValue;
  });
}

function trimDetail(value) {
  return Array.from(String(value || "").replace(/\s+/g, " ").trim()).slice(0, 300).join("");
}

module.exports = {
  ACCESS_URL_FAILURE_CATEGORIES,
  ACCESS_URL_FAILURE_SCOPES,
  ACCESS_URL_TYPES,
  classifyAccessUrlFailure,
  formatAccessUrlFailure,
  getAccessUrlFailureMetadata,
  isKnownAccessUrlFailureCategory,
  isReportableAccessUrlCategory,
  shouldFailAccessUrlOutcome,
  summarizeAccessUrlOutcomes,
};
