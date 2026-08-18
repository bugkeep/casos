import React, {useCallback, useEffect, useState} from "react";
import {useHistory} from "react-router-dom";
import {useTranslation} from "react-i18next";
import i18next from "i18next";
import * as DeploymentBackend from "@/backend/DeploymentBackend";
import * as StatefulSetBackend from "@/backend/StatefulSetBackend";
import * as DaemonSetBackend from "@/backend/DaemonSetBackend";
import * as PodBackend from "@/backend/PodBackend";
import * as ServiceBackend from "@/backend/ServiceBackend";
import * as IngressBackend from "@/backend/IngressBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import {Card, CardContent, CardHeader, CardTitle} from "@/components/ui/card";
import {MessageAlert} from "@/components/ui/alert";
import {EmptyState} from "@/components/shared/empty-state";
import {ForceGraph} from "@/components/shared/force-graph";
import {Loading} from "@/components/shared/loading";
import {PageContainer} from "@/components/shared/page-header";
import {SearchSelect} from "@/components/shared/simple-select";

const CATEGORIES = [
  {name: "Ingress", color: "#3b82f6"},
  {name: "Service", color: "#0f766e"},
  {name: "Deployment", color: "#7c3aed"},
  {name: "StatefulSet", color: "#d85a30"},
  {name: "DaemonSet", color: "#ba7517"},
  {name: "Pod", color: "#22c55e"},
];

const CAT = {INGRESS: 0, SERVICE: 1, DEPLOYMENT: 2, STATEFULSET: 3, DAEMONSET: 4, POD: 5};

const POD_STATUS_COLOR = {
  Running: "#22c55e",
  Pending: "#f59e0b",
  Failed: "#ef4444",
  Succeeded: "#3b82f6",
  Unknown: "#a3a3a3",
};

const ROUTE_MAP = {
  ingress: "/ingresses",
  service: "/services",
  deployment: "/deployments",
  statefulset: "/statefulsets",
  daemonset: "/daemonsets",
  pod: "/pods",
};

const EMPTY_GRAPH = {nodes: [], links: []};

// A controller owns a pod when every label in its selector matches. An empty
// selector matches nothing here rather than everything — a selector-less object
// is a mis-shaped record, not a claim on the whole namespace.
function selectorMatches(selector, labels) {
  if (!selector || !labels) {
    return false;
  }
  const entries = Object.entries(selector);
  if (entries.length === 0) {
    return false;
  }
  return entries.every(([key, value]) => labels[key] === value);
}

function buildGraphData(deployments, statefulSets, daemonSets, pods, services, ingresses) {
  const nodes = [];
  const links = [];
  const seen = new Set();

  function addNode(id, name, category, color) {
    if (seen.has(id)) {
      return;
    }
    seen.add(id);
    nodes.push({
      id,
      name,
      category,
      size: category < CAT.POD ? 44 : 30,
      color: color || CATEGORIES[category].color,
    });
  }

  (ingresses || []).forEach((ingress) => {
    if (!ingress.name) {
      return;
    }
    addNode(`ingress/${ingress.name}`, ingress.name, CAT.INGRESS);
    (ingress.rules || []).forEach((rule) => {
      if (rule.serviceName) {
        links.push({source: `ingress/${ingress.name}`, target: `service/${rule.serviceName}`});
      }
    });
  });

  (services || []).forEach((service) => {
    if (service.name) {
      addNode(`service/${service.name}`, service.name, CAT.SERVICE);
    }
  });
  (deployments || []).forEach((item) => {
    if (item.name) {
      addNode(`deployment/${item.name}`, item.name, CAT.DEPLOYMENT);
    }
  });
  (statefulSets || []).forEach((item) => {
    if (item.name) {
      addNode(`statefulset/${item.name}`, item.name, CAT.STATEFULSET);
    }
  });
  (daemonSets || []).forEach((item) => {
    if (item.name) {
      addNode(`daemonset/${item.name}`, item.name, CAT.DAEMONSET);
    }
  });

  (pods || []).forEach((pod) => {
    if (!pod.name) {
      return;
    }
    const phase = pod.phase || "Unknown";
    addNode(`pod/${pod.name}`, pod.name, CAT.POD, POD_STATUS_COLOR[phase] || POD_STATUS_COLOR.Unknown);
    const labels = pod.labels || {};

    [
      [deployments, "deployment"],
      [statefulSets, "statefulset"],
      [daemonSets, "daemonset"],
      [services, "service"],
    ].forEach(([collection, prefix]) => {
      (collection || []).forEach((owner) => {
        if (selectorMatches(owner.selector, labels)) {
          links.push({source: `${prefix}/${owner.name}`, target: `pod/${pod.name}`});
        }
      });
    });
  });

  return {nodes, links};
}

function LegendSwatch({color, label}) {
  return (
    <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
      <span className="h-2.5 w-2.5 shrink-0 rounded-[2px]" style={{backgroundColor: color}} />
      {label}
    </span>
  );
}

function TopologyPage() {
  useTranslation();
  const history = useHistory();

  const [namespaces, setNamespaces] = useState([]);
  const [namespace, setNamespace] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [graphData, setGraphData] = useState(EMPTY_GRAPH);

  useEffect(() => {
    NamespaceBackend.getNamespaces().then((res) => {
      if (res?.status !== "ok" || !Array.isArray(res.data)) {
        return;
      }
      const names = res.data.map((item) => item.name).filter(Boolean);
      setNamespaces(names);
      if (names.length > 0) {
        setNamespace(names[0]);
      }
    });
  }, []);

  useEffect(() => {
    if (!namespace) {
      return;
    }
    setLoading(true);
    setError(null);
    Promise.all([
      DeploymentBackend.getDeployments(namespace),
      StatefulSetBackend.getStatefulSets(namespace),
      DaemonSetBackend.getDaemonSets(namespace),
      PodBackend.getPods(namespace),
      ServiceBackend.getServices(namespace),
      IngressBackend.getIngresses(namespace),
    ])
      .then((responses) => {
        const rows = (response) => (response?.status === "ok" && Array.isArray(response.data) ? response.data : []);
        setGraphData(buildGraphData(...responses.map(rows)));
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [namespace]);

  // Clicking a node jumps to the list page for that kind, which is what makes
  // the graph a navigation aid rather than a picture.
  const handleNodeClick = useCallback((node) => {
    const [type] = node.id.split("/");
    if (ROUTE_MAP[type]) {
      history.push(ROUTE_MAP[type]);
    }
  }, [history]);

  const getTooltip = useCallback((node) => {
    const [type, ...rest] = node.id.split("/");
    return (
      <>
        <div className="font-medium capitalize">{type}</div>
        <div className="text-muted-foreground">{rest.join("/")}</div>
      </>
    );
  }, []);

  return (
    <PageContainer>
      <Card className="gap-3 py-4">
        <CardHeader className="flex flex-col gap-3 px-4 sm:flex-row sm:items-center sm:justify-between">
          <CardTitle className="text-sm">{i18next.t("general:Resource Topology")}</CardTitle>
          <div className="flex flex-wrap items-center gap-3">
            <div className="flex flex-wrap gap-1.5">
              {["Running", "Pending", "Failed", "Succeeded"].map((phase) => (
                <span
                  key={phase}
                  className="rounded-md border px-2 py-0.5 text-xs"
                  style={{borderColor: POD_STATUS_COLOR[phase], color: POD_STATUS_COLOR[phase]}}
                >
                  {phase}
                </span>
              ))}
            </div>
            <SearchSelect
              value={namespace}
              onChange={setNamespace}
              options={namespaces.map((name) => ({label: name, value: name}))}
              placeholder={i18next.t("general:Select namespace")}
              className="w-48"
            />
          </div>
        </CardHeader>

        <CardContent className="relative px-2">
          {error ? <MessageAlert title={error} className="mb-3" /> : null}
          <div className="mb-1 flex flex-wrap justify-center gap-x-4 gap-y-1.5">
            {CATEGORIES.map((category) => (
              <LegendSwatch key={category.name} color={category.color} label={category.name} />
            ))}
          </div>
          {loading ? (
            <div className="absolute inset-0 z-10 flex items-center justify-center">
              <Loading />
            </div>
          ) : null}
          {!loading && graphData.nodes.length === 0 && !error ? (
            <EmptyState title={i18next.t("general:No resources found")} className="py-24" />
          ) : (
            <ForceGraph
              nodes={graphData.nodes}
              links={graphData.links}
              onNodeClick={handleNodeClick}
              getTooltip={getTooltip}
              className="h-[640px] w-full"
            />
          )}
        </CardContent>
      </Card>
    </PageContainer>
  );
}

export default TopologyPage;
