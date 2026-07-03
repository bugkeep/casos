const {expect} = require("@playwright/test");
const {
  ACCESS_URL_TYPES,
  classifyAccessUrlFailure,
} = require("./access-url-diagnostics");
const {
  API_GET_DEPLOYMENTS,
  API_GET_INGRESSES,
  API_GET_NODES,
  API_GET_SERVICES,
  DEPLOYMENTS_ROUTE,
  NAMESPACE,
} = require("./app-store-access-url-config");
const {readOkJson} = require("./app-store-access-url-http");
const {
  cssAttributeValue,
  readNamespacedList,
} = require("./app-store-access-url-probe-utils");
const {
  markDeploymentAccessUrlClick,
} = require("./app-store-access-url-click-evidence");

const RESOURCE_ROW_VISIBLE_TIMEOUT_MS = 60 * 1000;

async function findDeploymentAccessUrl(page, releaseContext) {
  let evidence;
  try {
    evidence = await readDeploymentAccessEvidence(page, releaseContext);
  } catch (error) {
    return {
      ...emptyDeploymentAccessTarget(),
      classification: classifyAccessUrlFailure({
        detail: `failed to read Deployment Access URL evidence: ${error.message}`,
      }),
    };
  }

  const expectedTargets = findExpectedDeploymentAccessTargets({...evidence, releaseContext});
  if (expectedTargets.length === 0) {
    return {
      ...emptyDeploymentAccessTarget(),
      classification: classifyMissingDeploymentAccessTarget(evidence, releaseContext),
    };
  }
  let selectedTarget = null;
  if (releaseContext.expectedAccessUrlType) {
    selectedTarget = expectedTargets.find(target => target.accessUrlType === releaseContext.expectedAccessUrlType) || null;
    if (!selectedTarget) {
      return {
        ...emptyDeploymentAccessTarget(),
        accessUrlType: releaseContext.expectedAccessUrlType,
        classification: classifyAccessUrlFailure({
          serviceUrlMissing: true,
          accessUrlType: releaseContext.expectedAccessUrlType,
          detail: `release ${releaseContext.namespace || NAMESPACE}/${releaseContext.releaseName} expected a ${releaseContext.expectedAccessUrlType} Deployment Access URL, but only discovered target types: ${formatDiscoveredAccessUrlTypes(expectedTargets)}`,
        }),
      };
    }
  }
  selectedTarget = selectedTarget || expectedTargets.find(target => target.nodePort === releaseContext.nodePort) || expectedTargets[0];

  try {
    await page.goto(DEPLOYMENTS_ROUTE);
    await page.waitForLoadState("networkidle");
  } catch (error) {
    return {
      ...selectedTarget,
      classification: classifyAccessUrlFailure({
        detail: `navigation to deployments route failed: ${error.message}`,
      }),
    };
  }

  for (const target of expectedTargets) {
    const rowKey = `${target.namespace}/${target.deploymentName}`;
    const row = page.locator(`tr[data-row-key="${cssAttributeValue(rowKey)}"]`);
    try {
      await expect(row).toBeVisible({timeout: RESOURCE_ROW_VISIBLE_TIMEOUT_MS});
      const accessLink = row.locator(`a[href="${cssAttributeValue(target.accessUrl)}"]`);
      await expect(accessLink, `Deployment row ${rowKey} should render Access URL ${target.accessUrl}`).toBeVisible();
    } catch (error) {
      return {
        ...target,
        classification: classifyAccessUrlFailure({
          serviceUrlMissing: true,
          accessUrlType: target.accessUrlType,
          serviceType: target.serviceType,
          detail: `Deployment page row ${rowKey} did not render ${target.accessUrlType} Access URL ${target.accessUrl} for service ${target.namespace}/${target.serviceName}: ${error.message}`,
        }),
      };
    }
  }

  return selectedTarget;
}

async function openDeploymentAccessUrlPage(page, target, {timeoutMs = 5000} = {}) {
  if (!target?.deploymentName || !target?.namespace || !target?.accessUrl) {
    throw new Error("Deployment Access URL click needs deploymentName, namespace, and accessUrl");
  }
  await page.goto(DEPLOYMENTS_ROUTE);
  await page.waitForLoadState("networkidle");
  const rowKey = `${target.namespace}/${target.deploymentName}`;
  const row = page.locator(`tr[data-row-key="${cssAttributeValue(rowKey)}"]`);
  const accessLink = row.locator(`a[href="${cssAttributeValue(target.accessUrl)}"]`);
  await expect(row).toBeVisible({timeout: RESOURCE_ROW_VISIBLE_TIMEOUT_MS});
  await expect(accessLink, `Deployment row ${rowKey} should expose clickable Access URL ${target.accessUrl}`).toBeVisible();
  await accessLink.scrollIntoViewIfNeeded({timeout: Math.min(timeoutMs, RESOURCE_ROW_VISIBLE_TIMEOUT_MS)})
    .catch(error => {
      console.warn(`Failed to scroll Deployment Access URL ${rowKey} into view: ${error.message}`);
    });
  await markDeploymentAccessUrlClick(page, {
    rowKey,
    accessUrl: target.accessUrl,
    accessUrlType: target.accessUrlType,
  }).catch(error => {
    console.warn(`Failed to mark Deployment Access URL click ${rowKey}: ${error.message}`);
  });
  await page.waitForTimeout(250);
  const popupPromise = page.waitForEvent("popup", {timeout: timeoutMs}).catch(() => null);
  await accessLink.click({timeout: timeoutMs});
  const accessPage = await popupPromise;
  if (accessPage) {
    await accessPage.bringToFront().catch(() => {});
  }
  return accessPage;
}

function emptyDeploymentAccessTarget() {
  return {
    deploymentName: "",
    serviceName: "",
    serviceType: "",
    accessUrlType: "",
    ingressName: "",
    ingressHost: "",
  };
}

function formatDiscoveredAccessUrlTypes(targets) {
  const types = [...new Set((targets || []).map(target => target.accessUrlType || "type-not-recorded"))];
  return types.length > 0 ? types.join(",") : "none";
}

async function readDeploymentAccessEvidence(page, releaseContext) {
  const namespace = releaseContext.namespace || NAMESPACE;
  const [deployments, services, ingresses, nodeIP] = await Promise.all([
    readNamespacedList(page, API_GET_DEPLOYMENTS, namespace, "get-deployments"),
    readNamespacedList(page, API_GET_SERVICES, namespace, "get-services"),
    readNamespacedList(page, API_GET_INGRESSES, namespace, "get-ingresses"),
    readNodeIP(page),
  ]);
  return {deployments, services, ingresses, nodeIP};
}

function findExpectedDeploymentAccessTargets({deployments = [], services = [], ingresses = [], nodeIP, releaseContext = {}}) {
  const namespace = releaseContext.namespace || NAMESPACE;
  const targets = [];
  const seen = new Set();
  collectNodePortDeploymentAccessTargets({deployments, services, nodeIP, namespace, releaseContext, targets, seen});
  collectDomainDeploymentAccessTargets({deployments, services, ingresses, namespace, releaseContext, targets, seen});
  return targets.sort(compareDeploymentAccessTargets);
}

function collectNodePortDeploymentAccessTargets({deployments, services, nodeIP, namespace, releaseContext, targets, seen}) {
  if (!nodeIP) {
    return;
  }
  for (const service of services) {
    if (!isNodePortServiceForRelease(service, namespace, releaseContext)) {
      continue;
    }
    const matchingDeployments = deployments.filter(deployment =>
      deployment.namespace === namespace &&
      serviceSelectorMatchesDeployment(service.selector, deployment.selector)
    );
    for (const deployment of matchingDeployments) {
      for (const port of service.ports || []) {
        if (!port.nodePort) {
          continue;
        }
        const accessUrl = `http://${nodeIP}:${port.nodePort}`;
        const key = `${namespace}/${deployment.name}/${service.name}/${port.nodePort}`;
        if (seen.has(key)) {
          continue;
        }
        seen.add(key);
        targets.push({
          deploymentName: deployment.name,
          namespace,
          serviceName: service.name,
          serviceType: service.type,
          accessUrlType: ACCESS_URL_TYPES.NODEPORT,
          accessUrl,
          nodePort: port.nodePort,
        });
      }
    }
  }
}

function collectDomainDeploymentAccessTargets({deployments, services, ingresses, namespace, releaseContext, targets, seen}) {
  const serviceByName = new Map((services || [])
    .filter(service => service.namespace === namespace)
    .map(service => [service.name, service]));

  for (const deployment of deployments || []) {
    if (deployment.namespace !== namespace || !isReleaseRelatedDeployment(deployment, releaseContext)) {
      continue;
    }
    // Mirror DeploymentListPage.getAccessUrls: domain links are rendered from rule.serviceName.
    const deployServiceNames = new Set((services || [])
      .filter(service => service.namespace === namespace && service.selector?.app === deployment.name)
      .map(service => service.name));
    deployServiceNames.add(deployment.name);

    for (const ingress of ingresses || []) {
      if (ingress.namespace !== namespace || !isReleaseRelatedIngress(ingress, releaseContext)) {
        continue;
      }
      for (const rule of ingress.rules || []) {
        if (!rule.host || !rule.serviceName || !deployServiceNames.has(rule.serviceName)) {
          continue;
        }
        const service = serviceByName.get(rule.serviceName);
        const path = rule.path && rule.path !== "/" ? rule.path : "";
        const accessUrl = `http://${rule.host}${path}`;
        const key = `${namespace}/${deployment.name}/${ingress.name}/${rule.serviceName}/${accessUrl}`;
        if (seen.has(key)) {
          continue;
        }
        seen.add(key);
        targets.push({
          deploymentName: deployment.name,
          namespace,
          serviceName: rule.serviceName,
          serviceType: service?.type || "",
          accessUrlType: ACCESS_URL_TYPES.DOMAIN,
          accessUrl,
          ingressName: ingress.name,
          ingressHost: rule.host,
        });
      }
    }
  }
}

function compareDeploymentAccessTargets(left, right) {
  return compareString(left.deploymentName, right.deploymentName) ||
    compareString(left.serviceName, right.serviceName) ||
    accessUrlTypeRank(left.accessUrlType) - accessUrlTypeRank(right.accessUrlType) ||
    compareString(left.accessUrl, right.accessUrl) ||
    (left.nodePort || 0) - (right.nodePort || 0);
}

function accessUrlTypeRank(accessUrlType) {
  if (accessUrlType === ACCESS_URL_TYPES.NODEPORT) {
    return 0;
  }
  if (accessUrlType === ACCESS_URL_TYPES.DOMAIN) {
    return 1;
  }
  return 2;
}

function compareString(left, right) {
  const leftText = String(left || "");
  const rightText = String(right || "");
  if (leftText === rightText) {
    return 0;
  }
  return leftText < rightText ? -1 : 1;
}

function isNodePortServiceForRelease(service, namespace, releaseContext) {
  return service?.namespace === namespace &&
    service.type === "NodePort" &&
    Array.isArray(service.ports) &&
    service.ports.some(port => port.nodePort) &&
    isReleaseRelatedService(service, releaseContext);
}

function isReleaseRelatedService(service, releaseContext = {}) {
  const releaseName = releaseContext.releaseName || "";
  if (!releaseName) {
    return true;
  }
  if (service.name === releaseName || service.name.startsWith(`${releaseName}-`)) {
    return true;
  }
  if (releaseContext.serviceName && service.name === releaseContext.serviceName) {
    return true;
  }
  return Object.values(service.selector || {}).some(value => value === releaseName);
}

function isReleaseRelatedDeployment(deployment, releaseContext = {}) {
  const releaseName = releaseContext.releaseName || "";
  if (!releaseName) {
    return true;
  }
  if (deployment.name === releaseName || deployment.name.startsWith(`${releaseName}-`)) {
    return true;
  }
  return Object.values(deployment.selector || {}).some(value => value === releaseName);
}

function isReleaseRelatedIngress(ingress, releaseContext = {}) {
  const releaseName = releaseContext.releaseName || "";
  if (!releaseName) {
    return true;
  }
  if (ingress.name === releaseName || ingress.name.startsWith(`${releaseName}-`)) {
    return true;
  }
  return (ingress.rules || []).some(rule =>
    rule.serviceName === releaseName ||
    String(rule.serviceName || "").startsWith(`${releaseName}-`)
  );
}

function serviceSelectorMatchesDeployment(serviceSelector = {}, deploymentSelector = {}) {
  const entries = Object.entries(serviceSelector || {});
  if (entries.length === 0) {
    return false;
  }
  return entries.every(([key, value]) => deploymentSelector?.[key] === value);
}

function classifyMissingDeploymentAccessTarget(evidence, releaseContext) {
  const namespace = releaseContext.namespace || NAMESPACE;
  if (!evidence.nodeIP) {
    return classifyAccessUrlFailure({
      detail: "Deployment Access URL could not be evaluated because CasOS did not report a worker node IP",
    });
  }

  const relatedServices = (evidence.services || []).filter(service =>
    service.namespace === namespace && isReleaseRelatedService(service, releaseContext)
  );
  if (relatedServices.length === 0) {
    return classifyAccessUrlFailure({
      appWorkloadDiagnostic: true,
      detail: `release ${namespace}/${releaseContext.releaseName} did not create a Service that can be associated with its Deployments`,
    });
  }

  const nodePortServices = relatedServices.filter(service => service.type === "NodePort");
  if (nodePortServices.length === 0) {
    const domainIngresses = (evidence.ingresses || []).filter(ingress =>
      ingress.namespace === namespace &&
      (ingress.rules || []).some(rule => rule.host && relatedServices.some(service => service.name === rule.serviceName))
    );
    if (domainIngresses.length > 0) {
      return classifyAccessUrlFailure({
        appWorkloadDiagnostic: true,
        detail: `release ${namespace}/${releaseContext.releaseName} created Ingress domain rules, but none matched a listed Deployment Access URL row by rule serviceName`,
      });
    }
    return classifyAccessUrlFailure({
      appWorkloadDiagnostic: true,
      detail: `release ${namespace}/${releaseContext.releaseName} did not create a NodePort Service for Deployment Access URL rendering; service types: ${relatedServices.map(service => `${service.name}:${service.type}`).join(",")}`,
    });
  }

  return classifyAccessUrlFailure({
    appWorkloadDiagnostic: true,
    detail: `release ${namespace}/${releaseContext.releaseName} created NodePort Services, but none selected a listed Deployment by selector`,
  });
}

async function readNodeIP(page) {
  const response = await page.context().request.get(API_GET_NODES);
  const body = await readOkJson(response, "get-nodes");
  const nodes = Array.isArray(body.data) ? body.data : [];
  const nodeWithExternalIP = nodes.find(node => node.externalIP);
  if (nodeWithExternalIP) {
    return nodeWithExternalIP.externalIP;
  }
  const nodeWithInternalIP = nodes.find(node => node.internalIP);
  return nodeWithInternalIP?.internalIP || "";
}

module.exports = {
  findDeploymentAccessUrl,
  findExpectedDeploymentAccessTargets,
  openDeploymentAccessUrlPage,
};
