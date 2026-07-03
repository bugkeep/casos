const {expect} = require("@playwright/test");
const {
  ACCESS_URL_TYPES,
  classifyAccessUrlFailure,
  formatAccessUrlFailure,
} = require("./access-url-diagnostics");
const {
  ACCESS_READY_TIMEOUT_MS,
  API_GET_PODS,
  API_GET_SERVICES,
  NAMESPACE,
  SERVICES_ROUTE,
} = require("./app-store-access-url-config");
const {
  findDeploymentAccessUrl,
  openDeploymentAccessUrlPage,
} = require("./app-store-access-url-deployment-probe");
const {
  logAppStoreProgress,
} = require("./app-store-access-url-log");
const {
  cssAttributeValue,
  readNamespacedList,
} = require("./app-store-access-url-probe-utils");

const MAX_NAVIGATION_TIMEOUT_MS = 10000;
const MIN_NAVIGATION_TIMEOUT_MS = 1000;
const BACKOFF_CAP_MS = 3000;
const BACKOFF_BASE_MS = 500;
const BACKOFF_EXPONENT_LIMIT = 3;
const RESOURCE_ROW_VISIBLE_TIMEOUT_MS = 60 * 1000;
const ACCESS_URL_CLICK_TIMEOUT_MS = 5000;

async function readServiceAccessUrl(page, releaseContext) {
  const result = await findServiceAccessUrl(page, releaseContext);
  if (result.classification) {
    throw new Error(formatAccessUrlFailure({...releaseContext, classification: result.classification}));
  }
  return result.accessUrl;
}

async function findServiceAccessUrl(page, releaseContext) {
  try {
    await page.goto(SERVICES_ROUTE);
    await page.waitForLoadState("networkidle");
  } catch (error) {
    const classification = classifyAccessUrlFailure({
      detail: `navigation to services route failed: ${error.message}`,
    });
    return {classification};
  }
  const serviceEvidence = await readServiceEvidence(page, releaseContext).catch(error => ({
    service: null,
    error: error.message,
  }));
  const namespace = releaseContext.namespace || NAMESPACE;
  const serviceName = serviceEvidence.service?.name || releaseContext.serviceName || releaseContext.releaseName;
  if (!serviceName) {
    return {
      classification: classifyAccessUrlFailure({
        testHarnessDiagnostic: true,
        detail: `service name is missing from service evidence and release context for namespace ${namespace}`,
      }),
    };
  }
  const rowKey = `${namespace}/${serviceName}`;
  const row = page.locator(`tr[data-row-key="${cssAttributeValue(rowKey)}"]`);
  const expectedNodePort = releaseContext.nodePort || serviceEvidence.nodePort;
  try {
    await expect(row).toBeVisible({timeout: RESOURCE_ROW_VISIBLE_TIMEOUT_MS});
    const rowText = await row.textContent().catch(() => "");
    const serviceType = serviceEvidence.service?.type || detectServiceType(rowText);
    const accessLink = expectedNodePort
      ? row.getByRole("link", {name: new RegExp(`:${expectedNodePort}$`)})
      : row.getByRole("link").first();
    await expect(accessLink).toBeVisible();
    const accessUrl = await accessLink.getAttribute("href");
    expect(accessUrl, "the Services page Access URL should have an href").toBeTruthy();
    if (expectedNodePort) {
      expect(accessUrl).toContain(`:${expectedNodePort}`);
    }
    if (serviceType && serviceType !== "NodePort") {
      const classification = classifyAccessUrlFailure({
        serviceUrlMissing: true,
        serviceType,
        detail: `service ${namespace}/${serviceName} has type ${serviceType} but exposed ${accessUrl}`,
      });
      return {classification, serviceName, serviceType};
    }
    return {accessUrl, serviceName, serviceType};
  } catch (error) {
    const serviceType = serviceEvidence.service?.type || "";
    const serviceDetail = serviceType
      ? `service ${namespace}/${serviceName} has type ${serviceType} and did not expose a matching NodePort Access URL`
      : `service ${namespace}/${serviceName} did not expose a matching Access URL: ${error.message}`;
    const classification = classifyAccessUrlFailure({
      serviceUrlMissing: true,
      serviceType,
      detail: serviceDetail,
    });
    return {classification, serviceName, serviceType, serviceEvidenceError: serviceEvidence.error};
  }
}

async function waitForAccessUrl(page, accessUrl, releaseContext) {
  const result = await probeAccessUrl(page, accessUrl, releaseContext, {timeoutMs: ACCESS_READY_TIMEOUT_MS});
  if (result.reachable) {
    return;
  }
  throw new Error(formatAccessUrlFailure(result));
}

async function probeReleaseAccessUrl(page, releaseContext, {timeoutMs}) {
  const deploymentResult = await findDeploymentAccessUrl(page, releaseContext);
  if (deploymentResult.classification) {
    return {
      ...releaseContext,
      deploymentName: deploymentResult.deploymentName,
      serviceName: deploymentResult.serviceName,
      serviceType: deploymentResult.serviceType,
      accessUrlType: deploymentResult.accessUrlType,
      ingressName: deploymentResult.ingressName,
      ingressHost: deploymentResult.ingressHost,
      reachable: false,
      attemptCount: 0,
      classification: deploymentResult.classification,
    };
  }
  let accessPage = null;
  let accessClickError = null;
  const accessClickStartedAt = Date.now();
  try {
    accessPage = await openDeploymentAccessUrlPage(page, deploymentResult, {
      timeoutMs: Math.min(ACCESS_URL_CLICK_TIMEOUT_MS, Math.max(MIN_NAVIGATION_TIMEOUT_MS, timeoutMs)),
    });
  } catch (error) {
    accessClickError = error;
    logAppStoreProgress("click-deployment-access-url", accessClickStartedAt, {
      releaseName: releaseContext.releaseName,
      deploymentName: deploymentResult.deploymentName,
      accessUrl: deploymentResult.accessUrl,
      error: error.message,
    });
  }
  return probeAccessUrl(page, deploymentResult.accessUrl, {
    ...releaseContext,
    deploymentName: deploymentResult.deploymentName,
    serviceName: deploymentResult.serviceName,
    serviceType: deploymentResult.serviceType,
    accessUrlType: deploymentResult.accessUrlType,
    ingressName: deploymentResult.ingressName,
    ingressHost: deploymentResult.ingressHost,
    accessClickError: accessClickError?.message || "",
  }, {timeoutMs, accessPage});
}

async function probeAccessUrl(page, accessUrl, releaseContext, {timeoutMs, accessPage: clickedAccessPage = null}) {
  const started = Date.now();
  let lastFailure = null;
  let attemptCount = 0;
  const maxNavigationTimeoutMs = Math.min(MAX_NAVIGATION_TIMEOUT_MS, Math.max(MIN_NAVIGATION_TIMEOUT_MS, timeoutMs));
  let accessPage = clickedAccessPage;
  try {
    if (!accessPage) {
      accessPage = await page.context().newPage();
    }
    while (Date.now() - started < timeoutMs) {
      attemptCount += 1;
      const remainingMs = timeoutMs - (Date.now() - started);
      const navigationTimeoutMs = Math.min(maxNavigationTimeoutMs, Math.max(MIN_NAVIGATION_TIMEOUT_MS, remainingMs));
      try {
        const response = await accessPage.goto(accessUrl, {timeout: navigationTimeoutMs, waitUntil: "domcontentloaded"});
        const body = (await accessPage.locator("body").textContent()) || "";
        const successStatus = Boolean(response) && response.status() >= 200 && response.status() < 300;
        if (successStatus && body.trim().length > 0) {
          return {
            ...releaseContext,
            accessUrl,
            reachable: true,
            attemptCount,
          };
        }
        lastFailure = {responseStatus: response?.status() ?? null, body};
      } catch (error) {
        lastFailure = {error};
      }
      const classification = classifyAccessUrlFailure({
        ...(lastFailure || {detail: "access attempt failed"}),
        accessUrlType: releaseContext.accessUrlType,
      });
      logAppStoreProgress("wait-access-url", started, {
        attemptCount,
        category: classification.category,
        scope: classification.scope,
        reportable: classification.reportable,
        detail: classification.detail,
      });
      const backoffMs = Math.min(
        BACKOFF_CAP_MS,
        BACKOFF_BASE_MS * (2 ** Math.min(attemptCount - 1, BACKOFF_EXPONENT_LIMIT))
      );
      if (Date.now() - started + backoffMs < timeoutMs) {
        await delay(backoffMs);
      }
    }
  } finally {
    if (accessPage) {
      await accessPage.close().catch(() => {});
    }
  }
  const classification = await classifyAccessProbeFailure(page, {
    ...releaseContext,
    accessUrl,
  }, lastFailure || {
    detail: "access probe did not start; check timeoutMs configuration",
  });
  return {
    ...releaseContext,
    accessUrl,
    reachable: false,
    attemptCount,
    classification,
  };
}

async function classifyAccessProbeFailure(page, releaseContext, failureInput) {
  const baseClassification = classifyAccessUrlFailure({
    ...failureInput,
    accessUrlType: releaseContext.accessUrlType,
  });
  const workloadEvidence = await readWorkloadEvidence(page, releaseContext).catch(error => ({
    error: error.message,
  }));
  if (workloadEvidence.error || !workloadEvidence.service) {
    if (releaseContext.accessUrlType === ACCESS_URL_TYPES.DOMAIN) {
      return classifyAccessUrlFailure({
        domainAccessUrlDiagnostic: true,
        detail: `domain Access URL ${releaseContext.accessUrl || ""} could not be opened and service evidence was unavailable: ${workloadEvidence.error || baseClassification.detail}`,
      });
    }
    return baseClassification;
  }

  const service = workloadEvidence.service;
  if (service.type && service.type !== "NodePort" && releaseContext.accessUrlType !== ACCESS_URL_TYPES.DOMAIN) {
    return classifyAccessUrlFailure({
      appWorkloadDiagnostic: true,
      detail: `service ${service.namespace}/${service.name} has type ${service.type}, so the chart is not exposing a NodePort workload`,
    });
  }
  if (workloadEvidence.runningPods.length === 0) {
    return classifyAccessUrlFailure({
      appWorkloadDiagnostic: true,
      detail: `${service.type || "Service"} service ${service.namespace}/${service.name} has no running pods matching selector ${formatSelector(service.selector)}`,
    });
  }
  if (releaseContext.accessUrlType === ACCESS_URL_TYPES.DOMAIN) {
    return classifyAccessUrlFailure({
      domainAccessUrlDiagnostic: true,
      detail: `domain Access URL ${releaseContext.accessUrl} for Ingress ${releaseContext.namespace || NAMESPACE}/${releaseContext.ingressName || "ingress-not-recorded"} could not be opened: ${baseClassification.detail}`,
    });
  }
  return baseClassification;
}

async function readWorkloadEvidence(page, releaseContext) {
  const serviceEvidence = await readServiceEvidence(page, releaseContext);
  const pods = await readNamespacedList(page, API_GET_PODS, releaseContext.namespace || NAMESPACE, "get-pods");
  const matchingPods = serviceEvidence.service?.selector
    ? pods.filter(pod => labelsMatchSelector(pod.labels, serviceEvidence.service.selector))
    : [];
  return {
    ...serviceEvidence,
    matchingPods,
    runningPods: matchingPods.filter(pod => pod.phase === "Running"),
  };
}

async function readServiceEvidence(page, releaseContext) {
  const namespace = releaseContext.namespace || NAMESPACE;
  const serviceName = releaseContext.serviceName || releaseContext.releaseName;
  const services = await readNamespacedList(page, API_GET_SERVICES, namespace, "get-services");
  const service = services.find(item => item.namespace === namespace && item.name === serviceName) || null;
  const nodePort = service?.ports?.find(port => port.nodePort)?.nodePort;
  return {service, nodePort};
}

function labelsMatchSelector(labels = {}, selector = {}) {
  return Object.entries(selector).every(([key, value]) => labels[key] === value);
}

function formatSelector(selector = {}) {
  const entries = Object.entries(selector);
  return entries.length > 0
    ? entries.map(([key, value]) => `${key}=${value}`).join(",")
    : "empty selector";
}

function detectServiceType(text) {
  const match = String(text || "").match(/\b(ClusterIP|NodePort|LoadBalancer|ExternalName)\b/);
  return match ? match[1] : "";
}

function delay(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

module.exports = {
  probeReleaseAccessUrl,
  readServiceAccessUrl,
  waitForAccessUrl,
};
