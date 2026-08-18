import React, {useEffect, useRef, useState} from "react";
import {useHistory} from "react-router-dom";
import {useTranslation} from "react-i18next";
import i18next from "i18next";
import * as echarts from "echarts";
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
import {Loading} from "@/components/shared/loading";
import {PageContainer} from "@/components/shared/page-header";
import {SearchSelect} from "@/components/shared/simple-select";
import {isDarkMode} from "@/components/shared/echarts-widget";

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
      symbolSize: category < CAT.POD ? 44 : 30,
      itemStyle: color ? {color} : undefined,
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

function buildOption(nodes, links, dark) {
  return {
    backgroundColor: "transparent",
    tooltip: {
      formatter: (params) => {
        if (params.dataType !== "node") {
          return "";
        }
        const [type, ...rest] = params.data.id.split("/");
        return `<b style="text-transform:capitalize">${type}</b><br/>${rest.join("/")}`;
      },
    },
    legend: {
      data: CATEGORIES.map((category) => category.name),
      top: 8,
      left: "center",
      itemWidth: 12,
      itemHeight: 12,
      textStyle: {fontSize: 12, color: dark ? "#d4d4d4" : "#404040"},
    },
    series: [
      {
        type: "graph",
        layout: "force",
        data: nodes,
        links,
        categories: CATEGORIES.map((category) => ({name: category.name, itemStyle: {color: category.color}})),
        roam: true,
        draggable: true,
        force: {repulsion: 320, gravity: 0.04, edgeLength: [90, 220], layoutAnimation: true},
        label: {
          show: true,
          position: "bottom",
          fontSize: 11,
          color: dark ? "#a3a3a3" : "#525252",
          formatter: (params) => (params.data.name.length > 22 ? `${params.data.name.slice(0, 20)}…` : params.data.name),
        },
        lineStyle: {opacity: 0.55, width: 1.5, color: "source", curveness: 0.08},
        emphasis: {focus: "adjacency", lineStyle: {width: 3}},
        edgeSymbol: ["none", "arrow"],
        edgeSymbolSize: [0, 8],
      },
    ],
  };
}

function TopologyPage() {
  useTranslation();
  const history = useHistory();
  const containerRef = useRef(null);
  const chartRef = useRef(null);

  const [namespaces, setNamespaces] = useState([]);
  const [namespace, setNamespace] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [graphData, setGraphData] = useState({nodes: [], links: []});

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

  useEffect(() => {
    if (!containerRef.current) {
      return undefined;
    }
    const chart = echarts.init(containerRef.current, isDarkMode() ? "dark" : undefined);
    chart.setOption({backgroundColor: "transparent"});
    chartRef.current = chart;

    const resizeObserver = new ResizeObserver(() => chart.resize());
    resizeObserver.observe(containerRef.current);

    // Clicking a node jumps to the list page for that kind, which is what makes
    // the graph a navigation aid rather than a picture.
    chart.on("click", (params) => {
      if (params.dataType !== "node") {
        return;
      }
      const [type] = params.data.id.split("/");
      if (ROUTE_MAP[type]) {
        history.push(ROUTE_MAP[type]);
      }
    });

    return () => {
      resizeObserver.disconnect();
      chart.dispose();
      chartRef.current = null;
    };
  }, [history]);

  useEffect(() => {
    if (!chartRef.current) {
      return;
    }
    if (graphData.nodes.length === 0) {
      chartRef.current.clear();
      return;
    }
    chartRef.current.setOption(buildOption(graphData.nodes, graphData.links, isDarkMode()), {notMerge: true});
  }, [graphData]);

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
          {loading ? (
            <div className="absolute inset-0 z-10 flex items-center justify-center">
              <Loading />
            </div>
          ) : null}
          {!loading && graphData.nodes.length === 0 && !error ? (
            <EmptyState title={i18next.t("general:No resources found")} className="py-24" />
          ) : null}
          <div ref={containerRef} className="h-[640px] w-full" />
        </CardContent>
      </Card>
    </PageContainer>
  );
}

export default TopologyPage;
