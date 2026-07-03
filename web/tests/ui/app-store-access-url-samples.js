const {
  ACCESS_URL_FAILURE_CATEGORIES,
  ACCESS_URL_TYPES,
} = require("./access-url-diagnostics");
const {
  APP_STORE_NODE_PORT,
  APP_STORE_NO_PODS_NODE_PORT,
} = require("./app-store-access-url-config");

const REPRESENTATIVE_APP_STORE_REPOS = deepFreeze([
  {
    key: "podinfo",
    repoNamePrefix: "podinfo",
    repoURL: "https://stefanprodan.github.io/podinfo",
    charts: [
      {
        key: "podinfo-nodeport",
        chartName: "podinfo",
        releasePrefix: "podinfo-ok",
        valuesYAML: podinfoNodePortValues({replicaCount: 1, nodePort: APP_STORE_NODE_PORT}),
        nodePort: APP_STORE_NODE_PORT,
        expectReachable: true,
        sampleReason: "reachable NodePort control from a real app-store chart",
      },
      {
        key: "podinfo-clusterip",
        chartName: "podinfo",
        releasePrefix: "podinfo-ci",
        expectReachable: false,
        expectedCategory: ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC,
        expectedFailureLocation: "service-not-nodeport",
        sampleReason: "default podinfo service is ClusterIP, so CasOS should not report a missing NodePort URL as a bug",
      },
      {
        key: "podinfo-nodeport-no-pods",
        chartName: "podinfo",
        releasePrefix: "podinfo-np0",
        valuesYAML: podinfoNodePortValues({replicaCount: 0, nodePort: APP_STORE_NO_PODS_NODE_PORT}),
        nodePort: APP_STORE_NO_PODS_NODE_PORT,
        expectReachable: false,
        expectedCategory: ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC,
        expectedFailureLocation: "nodeport-no-running-pods",
        sampleReason: "NodePort is rendered but the chart has no running pods behind the service",
      },
      {
        key: "podinfo-domain",
        chartName: "podinfo",
        releasePrefix: "podinfo-domain",
        valuesYAML: ({releaseName}) => podinfoDomainValues({host: `${releaseName}.casos.invalid`}),
        expectReachable: false,
        expectedAccessUrlType: ACCESS_URL_TYPES.DOMAIN,
        expectedCategory: ACCESS_URL_FAILURE_CATEGORIES.DOMAIN_ACCESS_URL_UNREACHABLE,
        expectedFailureLocation: "domain-dns-or-ingress-route",
        sampleReason: "Ingress-backed domain Access URL should be rendered from the Deployments page and classified when the URL cannot open",
      },
    ],
  },
  {
    key: "echo-server",
    repoNamePrefix: "echo-server",
    repoURL: "https://ealenn.github.io/charts",
    charts: [
      {
        key: "echo-server-clusterip",
        chartName: "echo-server",
        releasePrefix: "echo-server-ci",
        expectReachable: false,
        expectedCategory: ACCESS_URL_FAILURE_CATEGORIES.APP_WORKLOAD_DIAGNOSTIC,
        expectedFailureLocation: "service-not-nodeport",
        sampleReason: "second real app-store chart with a default ClusterIP service",
      },
    ],
  },
]);

function deepFreeze(value) {
  if (!value || typeof value !== "object" || Object.isFrozen(value)) {
    return value;
  }
  for (const propertyName of Object.getOwnPropertyNames(value)) {
    deepFreeze(value[propertyName]);
  }
  return Object.freeze(value);
}

function podinfoNodePortValues({replicaCount, nodePort}) {
  return [
    `replicaCount: ${replicaCount}`,
    "service:",
    "  enabled: true",
    "  type: NodePort",
    "  httpPort: 9898",
    "  externalPort: 9898",
    `  nodePort: ${nodePort}`,
    "redis:",
    "  enabled: false",
    "ingress:",
    "  enabled: false",
    "",
  ].join("\n");
}

function podinfoDomainValues({host}) {
  return [
    "replicaCount: 1",
    "service:",
    "  enabled: true",
    "  type: ClusterIP",
    "  httpPort: 9898",
    "  externalPort: 9898",
    "redis:",
    "  enabled: false",
    "ingress:",
    "  enabled: true",
    "  className: \"\"",
    "  hosts:",
    `    - host: ${host}`,
    "      paths:",
    "        - path: /",
    "          pathType: ImplementationSpecific",
    "",
  ].join("\n");
}

function flattenRepresentativeCharts(repos) {
  const sourceRepos = repos ?? REPRESENTATIVE_APP_STORE_REPOS;
  return sourceRepos.flatMap(repo => repo.charts.map(chart => ({
    ...chart,
    repoKey: repo.key,
    repoNamePrefix: repo.repoNamePrefix,
    repoURL: repo.repoURL,
  })));
}

function expectedObservedFailureCategories(charts) {
  return [...new Set((charts || [])
    .filter(chart => !chart.expectReachable && chart.expectedCategory)
    .map(chart => chart.expectedCategory))]
    .sort();
}

module.exports = {
  REPRESENTATIVE_APP_STORE_REPOS,
  expectedObservedFailureCategories,
  flattenRepresentativeCharts,
};
