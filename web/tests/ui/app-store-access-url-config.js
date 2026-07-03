const fs = require("fs");
const path = require("path");

const API_ADD_HELM_REPO = "/api/add-helm-repo";
const API_ADD_MACHINE = "/api/add-machine";
const API_DELETE_HELM_REPO = "/api/delete-helm-repo";
const API_DEPLOY_MACHINE_NODE = "/api/deploy-machine-node";
const API_DELETE_MACHINE = "/api/delete-machine";
const API_GET_DEPLOYMENTS = "/api/get-deployments";
const API_GET_REPO_CHARTS = "/api/get-repo-charts";
const API_GET_HELM_REPOS = "/api/get-helm-repos";
const API_GET_HELM_RELEASES = "/api/get-helm-releases";
const API_GET_INGRESSES = "/api/get-ingresses";
const API_GET_MACHINE_NODE_LOGS = "/api/get-machine-node-logs";
const API_GET_MACHINE_NODE_TASKS = "/api/get-machine-node-tasks";
const API_GET_NODES = "/api/get-nodes";
const API_GET_PODS = "/api/get-pods";
const API_INSTALL_HELM_CHART_STREAM = "/api/install-helm-chart-stream";
const API_GET_SERVICES = "/api/get-services";
const API_UNINSTALL_HELM_RELEASE = "/api/uninstall-helm-release";

const APP_STORE_SOURCE = "app-store";
const APP_STORE_ROUTE = "/app-store";
const DEPLOYMENTS_ROUTE = process.env.E2E_APP_STORE_DEPLOYMENTS_ROUTE || "/deployments";
const MACHINES_ROUTE = "/machines";
const NAMESPACE = process.env.E2E_APP_STORE_NAMESPACE || "default";
const SERVICES_ROUTE = process.env.E2E_APP_STORE_SERVICES_ROUTE || "/services";
const WORKER_READY_TIMEOUT_MS = readPositiveIntegerEnv("E2E_APP_STORE_WORKER_TIMEOUT_MS", 20 * 60 * 1000);
const MACHINE_CREATE_TIMEOUT_MS = readPositiveIntegerEnv("E2E_APP_STORE_MACHINE_CREATE_TIMEOUT_MS", 2 * 60 * 1000);
const UI_NAVIGATION_TIMEOUT_MS = readPositiveIntegerEnv("E2E_APP_STORE_UI_NAVIGATION_TIMEOUT_MS", 30 * 1000);
const WORKER_TASK_POLL_INTERVAL_MS = readPositiveIntegerEnv("E2E_APP_STORE_WORKER_TASK_POLL_INTERVAL_MS", 5000);
const WORKER_NODE_POLL_INTERVAL_MS = readPositiveIntegerEnv("E2E_APP_STORE_WORKER_NODE_POLL_INTERVAL_MS", 3000);
const HELM_READY_TIMEOUT_MS = readPositiveIntegerEnv("E2E_APP_STORE_HELM_TIMEOUT_MS", 3 * 60 * 1000);
const ACCESS_READY_TIMEOUT_MS = readPositiveIntegerEnv("E2E_APP_STORE_ACCESS_TIMEOUT_MS", 2 * 60 * 1000);
const ACCESS_EXPERIMENT_TIMEOUT_MS = readPositiveIntegerEnv("E2E_APP_STORE_EXPERIMENT_ACCESS_TIMEOUT_MS", 30 * 1000);
const NODE_PORT_MIN = 30000;
const NODE_PORT_MAX = 32767;
const APP_STORE_NODE_PORT = readNodePortEnv("E2E_APP_STORE_NODE_PORT", 31080);
const APP_STORE_NO_PODS_NODE_PORT = readNodePortEnv(
  "E2E_APP_STORE_UNROUTED_NODE_PORT",
  APP_STORE_NODE_PORT === NODE_PORT_MAX ? NODE_PORT_MAX - 1 : APP_STORE_NODE_PORT + 1
);
if (APP_STORE_NO_PODS_NODE_PORT === APP_STORE_NODE_PORT) {
  throw new Error("E2E_APP_STORE_UNROUTED_NODE_PORT must differ from E2E_APP_STORE_NODE_PORT");
}
const E2E_APISERVER_URL = process.env.E2E_APISERVER_URL || "https://127.0.0.1:16443";
const E2E_MACHINE_OWNER = process.env.E2E_MACHINE_OWNER || "admin";
const SSH_HOST = process.env.E2E_APP_STORE_SSH_HOST || "127.0.0.1";
const SSH_PORT = readPositiveIntegerEnv("E2E_APP_STORE_SSH_PORT", 22);
const SSH_USER = process.env.E2E_APP_STORE_SSH_USER || "";
const SSH_PRIVATE_KEY = process.env.E2E_APP_STORE_SSH_PRIVATE_KEY || readSshPrivateKeyFromPath();
const hasWorkerSshConfig = Boolean(SSH_USER && SSH_PRIVATE_KEY);
const WORKER_DIAGNOSTICS_LOG = path.join(process.cwd(), "ui-app-store-worker-diagnostics.log");
const RETRYABLE_INFRASTRUCTURE_PATTERNS = Object.freeze([
  /connection refused/i,
  /invalid connection/i,
  /dial tcp .*3306/i,
  /cluster not ready/i,
  /temporarily unavailable/i,
]);
const RETRYABLE_HTTP_STATUS_CODES = Object.freeze([408, 429]);
const LOG_PROGRESS_INTERVAL_MS = 30 * 1000;

function readPositiveIntegerEnv(name, fallback) {
  const raw = process.env[name];
  const value = raw === undefined || raw === "" ? fallback : Number(raw);
  if (!Number.isInteger(value) || value <= 0) {
    throw new Error(`${name} must be a positive integer when set`);
  }
  return value;
}

function readNodePortEnv(name, fallback) {
  const raw = process.env[name];
  const value = raw === undefined || raw === "" ? fallback : Number(raw);
  if (!Number.isInteger(value) || value < NODE_PORT_MIN || value > NODE_PORT_MAX) {
    throw new Error(`${name} must be an integer between ${NODE_PORT_MIN} and ${NODE_PORT_MAX} when set`);
  }
  return value;
}

function readSshPrivateKeyFromPath() {
  const keyPath = process.env.E2E_APP_STORE_SSH_KEY_PATH;
  if (!keyPath) {
    return "";
  }
  try {
    return fs.readFileSync(keyPath, "utf8");
  } catch (error) {
    throw new Error(`failed to read E2E_APP_STORE_SSH_KEY_PATH ${keyPath}: ${error.message}`, {cause: error});
  }
}

module.exports = {
  ACCESS_EXPERIMENT_TIMEOUT_MS,
  ACCESS_READY_TIMEOUT_MS,
  API_ADD_HELM_REPO,
  API_ADD_MACHINE,
  API_DELETE_HELM_REPO,
  API_DEPLOY_MACHINE_NODE,
  API_DELETE_MACHINE,
  API_GET_DEPLOYMENTS,
  API_GET_REPO_CHARTS,
  API_GET_HELM_REPOS,
  API_GET_HELM_RELEASES,
  API_GET_INGRESSES,
  API_GET_MACHINE_NODE_LOGS,
  API_GET_MACHINE_NODE_TASKS,
  API_GET_NODES,
  API_GET_PODS,
  API_INSTALL_HELM_CHART_STREAM,
  API_GET_SERVICES,
  API_UNINSTALL_HELM_RELEASE,
  APP_STORE_NODE_PORT,
  APP_STORE_NO_PODS_NODE_PORT,
  APP_STORE_ROUTE,
  APP_STORE_SOURCE,
  DEPLOYMENTS_ROUTE,
  E2E_APISERVER_URL,
  E2E_MACHINE_OWNER,
  HELM_READY_TIMEOUT_MS,
  LOG_PROGRESS_INTERVAL_MS,
  MACHINE_CREATE_TIMEOUT_MS,
  MACHINES_ROUTE,
  NAMESPACE,
  RETRYABLE_INFRASTRUCTURE_PATTERNS,
  RETRYABLE_HTTP_STATUS_CODES,
  SSH_HOST,
  SSH_PORT,
  SSH_PRIVATE_KEY,
  SSH_USER,
  SERVICES_ROUTE,
  UI_NAVIGATION_TIMEOUT_MS,
  WORKER_DIAGNOSTICS_LOG,
  WORKER_NODE_POLL_INTERVAL_MS,
  WORKER_READY_TIMEOUT_MS,
  WORKER_TASK_POLL_INTERVAL_MS,
  hasWorkerSshConfig,
};
