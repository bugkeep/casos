const CATALOGS = {
  Bitnami: `
    airflow apache apisix appsmith argo-cd argo-workflows aspnet-core cadvisor cassandra cert-manager
    chainloop cilium clickhouse clickhouse-operator cloudnative-pg common concourse consul contour contour-operator
    dataplatform-bp2 deepspeed discourse dokuwiki dremio drupal ejbca elasticsearch envoy-gateway etcd
    external-dns flink fluent-bit fluentd flux geode ghost gitea gitlab-runner grafana
    grafana-alloy grafana-k6-operator grafana-loki grafana-mimir grafana-operator grafana-tempo haproxy haproxy-intel harbor influxdb
    jaeger janusgraph jasperreports jenkins joomla jupyterhub kafka keycloak keydb kiam
    kibana kong kube-arangodb kube-prometheus kube-state-metrics kubeapps kuberay kubernetes-event-exporter logstash magento
    mariadb mariadb-galera mastodon matomo mediawiki memcached metallb metrics-server milvus minio
    minio-operator mlflow mongodb mongodb-sharded moodle multus-cni mxnet mysql nats neo4j
    nessie nginx nginx-ingress-controller nginx-intel node node-exporter oauth2-proxy odoo opencart opensearch
    osclass owncloud parse phpbb phpmyadmin pinniped postgresql postgresql-ha prestashop prometheus
    pytorch rabbitmq rabbitmq-cluster-operator redis redis-cluster redmine schema-registry scylladb sealed-secrets seaweedfs
    solr sonarqube spark spring-cloud-dataflow suitecrm supabase superset tensorflow-resnet thanos tomcat
    valkey valkey-cluster vault victoriametrics wavefront wavefront-adapter-for-istio wavefront-hpa-adapter wavefront-prometheus-storage-adapter whereabouts wildfly
    wordpress wordpress-intel zipkin zookeeper
  `,
  Rancher: `
    elemental elemental-crd epinio epinio-crd fleet fleet-agent fleet-crd harvester-cloud-provider harvester-csi-driver harvester-rbac
    longhorn longhorn-crd neuvector neuvector-crd neuvector-monitor prometheus-federator rancher-aks-operator rancher-aks-operator-crd rancher-alerting-drivers rancher-backup
    rancher-backup-crd rancher-cis-benchmark rancher-cis-benchmark-crd rancher-compliance rancher-compliance-crd rancher-csp-adapter rancher-eks-operator rancher-eks-operator-crd rancher-gatekeeper rancher-gatekeeper-crd
    rancher-gke-operator rancher-gke-operator-crd rancher-istio rancher-logging rancher-logging-crd rancher-monitoring rancher-monitoring-crd rancher-project-monitoring rancher-pushprox rancher-supportability-review
    rancher-supportability-review-crd rancher-turtles rancher-webhook remotedialer-proxy sriov sriov-crd system-upgrade-controller ui-plugin-operator ui-plugin-operator-crd
  `,
  "ingress-nginx": "ingress-nginx",
};

const PRESET_REPO_URLS = {
  Bitnami: "https://charts.bitnami.com/bitnami",
  Rancher: "https://charts.rancher.io",
  "ingress-nginx": "https://kubernetes.github.io/ingress-nginx",
};

const UNSUPPORTED = {
  Bitnami: `
    apisix argo-cd argo-workflows clickhouse-operator cloudnative-pg contour contour-operator envoy-gateway flux grafana-alloy
    grafana-k6-operator grafana-loki grafana-operator kong kube-arangodb kube-prometheus kubeapps kuberay metallb minio-operator
    multus-cni nginx-ingress-controller pinniped rabbitmq-cluster-operator sealed-secrets supabase wavefront-adapter-for-istio wavefront-hpa-adapter whereabouts
  `,
  Rancher: `
    elemental elemental-crd epinio epinio-crd fleet-agent fleet-crd harvester-csi-driver harvester-rbac longhorn longhorn-crd
    neuvector-crd rancher-aks-operator-crd rancher-backup rancher-backup-crd rancher-cis-benchmark rancher-cis-benchmark-crd rancher-compliance rancher-compliance-crd rancher-csp-adapter rancher-eks-operator-crd
    rancher-gatekeeper rancher-gke-operator-crd rancher-istio rancher-logging rancher-logging-crd rancher-monitoring rancher-project-monitoring rancher-pushprox rancher-supportability-review rancher-supportability-review-crd
    rancher-turtles sriov sriov-crd ui-plugin-operator-crd
  `,
  "ingress-nginx": "ingress-nginx",
};

function catalogChartNames(rawCharts) {
  const trimmed = rawCharts.trim();
  return trimmed ? trimmed.split(/\s+/) : [];
}

function validateCatalogPartition(catalogs, unsupported) {
  const catalogRepos = Object.keys(catalogs).sort();
  const unsupportedRepos = Object.keys(unsupported).sort();
  if (catalogRepos.join("\n") !== unsupportedRepos.join("\n")) {
    throw new Error("Catalog and unsupported repository keys do not match");
  }

  for (const repo of catalogRepos) {
    const charts = catalogChartNames(catalogs[repo]);
    const chartSet = new Set(charts);
    if (chartSet.size !== charts.length) {
      throw new Error(`Catalog ${repo} contains duplicate chart names`);
    }
    const unsupportedCharts = catalogChartNames(unsupported[repo]);
    if (new Set(unsupportedCharts).size !== unsupportedCharts.length) {
      throw new Error(`Catalog ${repo} unsupported list contains duplicate chart names`);
    }
    for (const chart of unsupportedCharts) {
      if (!chartSet.has(chart)) {
        throw new Error(`Unsupported chart ${repo}/${chart} is not present in its catalog`);
      }
    }
  }
}

validateCatalogPartition(CATALOGS, UNSUPPORTED);

const UNSUPPORTED_SETS = Object.fromEntries(
  Object.entries(UNSUPPORTED).map(([repo, rawCharts]) => [repo, new Set(catalogChartNames(rawCharts))])
);

const ALL_CATALOG_CASES = Object.entries(CATALOGS).flatMap(([repo, rawCharts]) =>
  catalogChartNames(rawCharts).map(chart => ({repo, chart}))
);

const INSTALLABLE_CASES = ALL_CATALOG_CASES.filter(({repo, chart}) => !UNSUPPORTED_SETS[repo].has(chart));
const UNSUPPORTED_CASES = ALL_CATALOG_CASES.filter(({repo, chart}) => UNSUPPORTED_SETS[repo].has(chart));

if (ALL_CATALOG_CASES.length !== 194 || INSTALLABLE_CASES.length !== 130 || UNSUPPORTED_CASES.length !== 64) {
  throw new Error("Unexpected App Store catalog snapshot size");
}

function assertCatalogSnapshot(currentCatalog) {
  for (const [repo, rawCharts] of Object.entries(CATALOGS)) {
    const expected = catalogChartNames(rawCharts).sort();
    const actual = [...(currentCatalog[repo] ?? [])].sort();
    if (expected.join("\n") !== actual.join("\n")) {
      const expectedSet = new Set(expected);
      const actualSet = new Set(actual);
      const missing = expected.filter(chart => !actualSet.has(chart));
      const unexpected = actual.filter(chart => !expectedSet.has(chart));
      throw new Error(
        `${repo} catalog snapshot differs: missing=[${missing.join(", ")}], unexpected=[${unexpected.join(", ")}]`
      );
    }
  }
}

module.exports = {
  ALL_CATALOG_CASES,
  INSTALLABLE_CASES,
  PRESET_REPO_URLS,
  UNSUPPORTED_CASES,
  assertCatalogSnapshot,
  validateCatalogPartition,
};
