const {
  RETRYABLE_INFRASTRUCTURE_PATTERNS,
  RETRYABLE_HTTP_STATUS_CODES,
} = require("./app-store-access-url-config");

const IGNORABLE_CLEANUP_MESSAGES = Object.freeze([
  "not found",
  "not loaded",
  "cluster not ready",
]);

class RetryableInfrastructureError extends Error {
  constructor(message) {
    super(message);
    this.name = "RetryableInfrastructureError";
  }
}

function isRetryableInfrastructureMessage(message) {
  const text = String(message || "");
  return RETRYABLE_INFRASTRUCTURE_PATTERNS.some(pattern => pattern.test(text));
}

function isRetryableInfrastructureError(error) {
  return error instanceof RetryableInfrastructureError || isRetryableInfrastructureMessage(error?.message || error);
}

function isRetryableHttpStatus(status) {
  return status >= 500 || RETRYABLE_HTTP_STATUS_CODES.includes(status);
}

function isIgnorableCleanupMessage(message) {
  const text = String(message || "").toLowerCase();
  return IGNORABLE_CLEANUP_MESSAGES.some(pattern => text.includes(pattern));
}

function responseErrorMessage(context, response) {
  return `${context}: HTTP ${response.status()}`;
}

async function readOkJson(response, context) {
  if (!response.ok()) {
    const message = responseErrorMessage(context, response);
    if (isRetryableHttpStatus(response.status())) {
      throw new RetryableInfrastructureError(message);
    }
    throw new Error(message);
  }

  let body;
  try {
    body = await response.json();
  } catch (error) {
    const parseError = String(error?.message || error || "error message unavailable");
    const message = `${context}: failed to parse JSON response: ${parseError}`;
    throw new Error(message);
  }

  if (!body || typeof body !== "object" || Array.isArray(body)) {
    throw new Error(`${context}: unexpected JSON response shape`);
  }

  if (body.status !== "ok") {
    const msg = String(body.msg || "");
    const message = msg ? `${context}: ${msg}` : `${context}: unexpected response`;
    if (isRetryableInfrastructureError(message)) {
      throw new RetryableInfrastructureError(message);
    }
    throw new Error(message);
  }
  return body;
}

module.exports = {
  RetryableInfrastructureError,
  isIgnorableCleanupMessage,
  isRetryableInfrastructureError,
  isRetryableHttpStatus,
  readOkJson,
  responseErrorMessage,
};
