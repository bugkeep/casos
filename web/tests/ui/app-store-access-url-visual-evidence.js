const {
  ACCESS_URL_FAILURE_CATEGORIES,
  ACCESS_URL_TYPES,
  formatAccessUrlFailure,
  summarizeAccessUrlOutcomes,
} = require("./access-url-diagnostics");
const {
  DEPLOYMENTS_ROUTE,
  NAMESPACE,
} = require("./app-store-access-url-config");
const {
  cssAttributeValue,
} = require("./app-store-access-url-probe-utils");

const VISUAL_SUMMARY_MAX_LENGTH = 320;
const VISUAL_DETAIL_MAX_LENGTH = 180;
const VISUAL_EVIDENCE_TIMEOUT_MS = 5000;
const MAX_OVERLAY_ITEMS = 4;
const ACCESS_SUMMARY_ARTIFACT_NAME = "ui-app-store-access-summary.json";
const VISUAL_EVIDENCE_FALLBACK_DETAIL = "Access URL visual evidence could not summarize the failed outcome";

function buildAccessUrlFailureAnnotations(outcomes) {
  return (outcomes || [])
    .map((outcome, index) => buildAccessUrlFailureAnnotation(outcome, index))
    .filter(Boolean);
}

async function attachAnnotatedAccessUrlFailureScreenshot(page, outcomes, testInfo) {
  const annotations = buildAccessUrlFailureAnnotations(outcomes);
  if (annotations.length === 0) {
    return;
  }
  await renderAccessUrlFailureOverlay(page, annotations, {navigateToDeployments: true, highlightRows: true});
  if (!testInfo || typeof testInfo.outputPath !== "function") {
    return;
  }
  const screenshotPath = testInfo.outputPath("access-url-failures-annotated.png");
  await page.screenshot({path: screenshotPath, fullPage: true});
  await testInfo.attach("access-url-failures-annotated", {
    path: screenshotPath,
    contentType: "image/png",
  });
}

async function showAccessUrlFailureOverlay(page, outcomes) {
  const annotations = buildAccessUrlFailureAnnotations(outcomes);
  if (annotations.length === 0) {
    return;
  }
  await renderAccessUrlFailureOverlay(page, annotations, {navigateToDeployments: false, highlightRows: false});
  await page.waitForSelector("#casos-access-url-failure-overlay", {
    state: "visible",
    timeout: VISUAL_EVIDENCE_TIMEOUT_MS,
  })
    .catch(error => {
      console.warn(`Access URL failure overlay did not become visible: ${error.message}`);
    });
}

async function renderAccessUrlFailureOverlay(page, annotations, options = {}) {
  if (options.navigateToDeployments) {
    await page.goto(DEPLOYMENTS_ROUTE).catch(error => {
      console.warn(`Failed to navigate to Deployments for Access URL annotation: ${error.message}`);
    });
    await page.waitForLoadState("networkidle").catch(error => {
      console.warn(`Failed to wait for Deployments network idle before Access URL annotation: ${error.message}`);
    });
    await scrollFirstFailureIntoView(page, annotations).catch(error => {
      console.warn(`Failed to scroll Access URL failure into view: ${error.message}`);
    });
  }
  await page.evaluate(({annotations: visualAnnotations, highlightRows, maxOverlayItems, artifactName}) => {
    const previousOverlay = document.getElementById("casos-access-url-failure-overlay");
    if (previousOverlay) {
      previousOverlay.remove();
    }
    const previousStyle = document.getElementById("casos-access-url-failure-style");
    if (previousStyle) {
      previousStyle.remove();
    }
    document
      .querySelectorAll(".casos-access-url-failure-row,.casos-access-url-failure-link")
      .forEach(element => {
        element.classList.remove("casos-access-url-failure-row", "casos-access-url-failure-link");
      });

    const style = document.createElement("style");
    style.id = "casos-access-url-failure-style";
    style.textContent = `
      .casos-access-url-failure-row {
        box-shadow: inset 0 0 0 3px #ff4d4f !important;
        background: rgba(255, 77, 79, 0.08) !important;
      }
      .casos-access-url-failure-link {
        outline: 3px solid #ff4d4f !important;
        outline-offset: 3px !important;
        border-radius: 4px !important;
        background: #fff1f0 !important;
        color: #a8071a !important;
      }
      #casos-access-url-failure-overlay {
        position: fixed;
        right: 24px;
        top: 72px;
        z-index: 2147483647;
        width: min(520px, calc(100vw - 48px));
        max-height: calc(100vh - 96px);
        overflow: auto;
        box-sizing: border-box;
        padding: 14px 16px;
        border: 2px solid #ff4d4f;
        border-radius: 8px;
        background: #fff1f0;
        color: #5c0011;
        box-shadow: 0 12px 32px rgba(92, 0, 17, 0.22);
        font-family: Arial, sans-serif;
        font-size: 13px;
        line-height: 1.45;
      }
      #casos-access-url-failure-overlay .casos-access-url-overlay-title {
        font-size: 16px;
        font-weight: 700;
        margin-bottom: 8px;
      }
      #casos-access-url-failure-overlay .casos-access-url-overlay-item {
        padding-top: 8px;
        margin-top: 8px;
        border-top: 1px solid rgba(255, 77, 79, 0.35);
        overflow-wrap: anywhere;
      }
      #casos-access-url-failure-overlay .casos-access-url-overlay-label {
        font-weight: 700;
      }
      #casos-access-url-failure-overlay .casos-access-url-overlay-code {
        font-family: Consolas, Monaco, monospace;
      }
    `;
    document.head.appendChild(style);

    if (highlightRows) {
      const rows = Array.from(document.querySelectorAll("tr[data-row-key]"));
      for (const annotation of visualAnnotations) {
        const row = rows.find(element => element.getAttribute("data-row-key") === annotation.rowKey);
        if (!row) {
          continue;
        }
        row.classList.add("casos-access-url-failure-row");
        const link = Array.from(row.querySelectorAll("a[href]"))
          .find(element => element.getAttribute("href") === annotation.accessUrl);
        if (link) {
          link.classList.add("casos-access-url-failure-link");
          link.setAttribute("data-access-url-failure-reason", annotation.summary);
          link.setAttribute("title", annotation.summary);
        }
      }
    }

    const overlay = document.createElement("div");
    overlay.id = "casos-access-url-failure-overlay";
    overlay.setAttribute("role", "note");
    const title = document.createElement("div");
    title.className = "casos-access-url-overlay-title";
    title.textContent = `${visualAnnotations.length} Access URL failure(s)`;
    overlay.appendChild(title);

    for (const annotation of visualAnnotations.slice(0, maxOverlayItems)) {
      const item = document.createElement("div");
      item.className = "casos-access-url-overlay-item";
      appendLine(item, `#${annotation.index} ${annotation.title}`, annotation.category, "casos-access-url-overlay-label");
      appendLine(item, "URL", annotation.accessUrl || "url-not-recorded", "casos-access-url-overlay-code");
      appendLine(item, "Row", annotation.rowKey || "deployment-row-not-recorded", "casos-access-url-overlay-code");
      appendLine(item, "Reason", annotation.detail || annotation.summary, "");
      overlay.appendChild(item);
    }

    if (visualAnnotations.length > maxOverlayItems) {
      const more = document.createElement("div");
      more.className = "casos-access-url-overlay-item";
      more.textContent = `${visualAnnotations.length - maxOverlayItems} more failure(s) are in ${artifactName}`;
      overlay.appendChild(more);
    }
    document.body.appendChild(overlay);

    function appendLine(parent, label, value, valueClassName) {
      const line = document.createElement("div");
      const labelElement = document.createElement("span");
      labelElement.className = "casos-access-url-overlay-label";
      labelElement.textContent = `${label}: `;
      const valueElement = document.createElement("span");
      if (valueClassName) {
        valueElement.className = valueClassName;
      }
      valueElement.textContent = value;
      line.appendChild(labelElement);
      line.appendChild(valueElement);
      parent.appendChild(line);
    }
  }, {
    annotations,
    artifactName: ACCESS_SUMMARY_ARTIFACT_NAME,
    highlightRows: Boolean(options.highlightRows),
    maxOverlayItems: MAX_OVERLAY_ITEMS,
  });
}

async function scrollFirstFailureIntoView(page, annotations) {
  const first = annotations.find(annotation => annotation.rowKey && annotation.accessUrl);
  if (!first) {
    return;
  }
  const row = page.locator(`tr[data-row-key="${cssAttributeValue(first.rowKey)}"]`);
  const link = row.locator(`a[href="${cssAttributeValue(first.accessUrl)}"]`);
  await link.scrollIntoViewIfNeeded({timeout: VISUAL_EVIDENCE_TIMEOUT_MS})
    .catch(() => row.scrollIntoViewIfNeeded({timeout: VISUAL_EVIDENCE_TIMEOUT_MS}));
}

function buildAccessUrlFailureAnnotation(outcome, index) {
  if (!outcome) {
    return null;
  }
  const classification = readVisualClassification(outcome);
  const namespace = outcome.namespace || NAMESPACE;
  const deploymentName = outcome.deploymentName || "";
  return {
    index: index + 1,
    title: accessUrlFailureTitle(outcome.accessUrlType),
    category: classification.category,
    rowKey: deploymentName ? `${namespace}/${deploymentName}` : "",
    accessUrl: outcome.accessUrl || "",
    releaseName: outcome.releaseName || "",
    deploymentName,
    serviceName: outcome.serviceName || "",
    detail: trimVisualText(classification.detail, VISUAL_DETAIL_MAX_LENGTH),
    summary: trimVisualText(formatAccessUrlFailure({
      ...outcome,
      classification: {
        category: classification.category,
        detail: classification.detail,
      },
    }), VISUAL_SUMMARY_MAX_LENGTH),
  };
}

function readVisualClassification(outcome) {
  const summary = summarizeAccessUrlOutcomes([outcome]);
  const bucket = [...summary.categories, ...summary.diagnosticCategories][0];
  const example = bucket?.examples?.[0] || {};
  if (bucket) {
    return {
      category: bucket.category,
      scope: bucket.scope,
      reportable: bucket.reportable,
      detail: example.detail || bucket.reportReason || "Access URL failed without a detailed reason",
    };
  }
  return {
    category: ACCESS_URL_FAILURE_CATEGORIES.TEST_HARNESS_DIAGNOSTIC,
    scope: "test-harness",
    reportable: false,
    detail: VISUAL_EVIDENCE_FALLBACK_DETAIL,
  };
}

function accessUrlFailureTitle(accessUrlType) {
  if (accessUrlType === ACCESS_URL_TYPES.DOMAIN) {
    return "Domain Access URL failed";
  }
  if (accessUrlType === ACCESS_URL_TYPES.NODEPORT) {
    return "NodePort Access URL failed";
  }
  return "Access URL failed";
}

function trimVisualText(value, maxLength) {
  const text = String(value || "").replace(/\s+/g, " ").trim();
  if (text.length <= maxLength) {
    return text;
  }
  return `${text.slice(0, Math.max(0, maxLength - 3))}...`;
}

module.exports = {
  attachAnnotatedAccessUrlFailureScreenshot,
  buildAccessUrlFailureAnnotations,
  showAccessUrlFailureOverlay,
  VISUAL_SUMMARY_MAX_LENGTH,
};
