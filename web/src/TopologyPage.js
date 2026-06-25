import React, {useEffect, useRef, useState} from "react";
import {Alert, Card, Empty, Select, Spin, Tag} from "antd";
import {useHistory} from "react-router-dom";
import {useTranslation} from "react-i18next";
import * as echarts from "echarts";
import * as DeploymentBackend from "./backend/DeploymentBackend";
import * as StatefulSetBackend from "./backend/StatefulSetBackend";
import * as DaemonSetBackend from "./backend/DaemonSetBackend";
import * as PodBackend from "./backend/PodBackend";
import * as ServiceBackend from "./backend/ServiceBackend";
import * as IngressBackend from "./backend/IngressBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import i18next from "i18next";

const CATEGORIES = [
  {name: "Ingress", color: "#1677ff"},
  {name: "Service", color: "#0f766e"},
  {name: "Deployment", color: "#7c3aed"},
  {name: "StatefulSet", color: "#d85a30"},
  {name: "DaemonSet", color: "#ba7517"},
  {name: "Pod", color: "#52c41a"},
];

const CAT = {
  INGRESS: 0,
  SERVICE: 1,
  DEPLOYMENT: 2,
  STATEFULSET: 3,
  DAEMONSET: 4,
  POD: 5,
};

const POD_STATUS_COLOR = {
  Running: "#52c41a",
  Pending: "#faad14",
  Failed: "#ff4d4f",
  Succeeded: "#1677ff",
  Unknown: "#bfbfbf",
};

const ROUTE_MAP = {
  ingress: "/ingresses",
  service: "/services",
  deployment: "/deployments",
  statefulset: "/statefulsets",
  daemonset: "/daemonsets",
  pod: "/pods",
};

function selectorMatches(selector, labels) {
  if (!selector || !labels) {return false;}
  const entries = Object.entries(selector);
  if (entries.length === 0) {return false;}
  return entries.every(([k, v]) => labels[k] === v);
}

function buildGraphData(deployments, statefulsets, daemonsets, pods, services, ingresses) {
  const nodes = [];
  const links = [];
  const seen = new Set();

  const addNode = (id, name, cat, color) => {
    if (seen.has(id)) {return;}
    seen.add(id);
    const isController = cat < CAT.POD;
    nodes.push({
      id,
      name,
      category: cat,
      symbolSize: isController ? 44 : 30,
      itemStyle: color ? {color} : undefined,
    });
  };

  const addLink = (source, target) => {
    if (seen.has(id => id === source) || seen.has(id => id === target)) {return;}
    links.push({source, target});
  };

  (ingresses || []).forEach(ing => {
    const name = ing.name;
    if (!name) {return;}
    addNode(`ingress/${name}`, name, CAT.INGRESS);
    (ing.rules || []).forEach(r => {
      if (r.serviceName) {
        addLink(`ingress/${name}`, `service/${r.serviceName}`);
      }
    });
  });

  (services || []).forEach(svc => {
    const name = svc.name;
    if (!name) {return;}
    addNode(`service/${name}`, name, CAT.SERVICE);
  });

  (deployments || []).forEach(d => {
    if (!d.name) {return;}
    addNode(`deployment/${d.name}`, d.name, CAT.DEPLOYMENT);
  });

  (statefulsets || []).forEach(s => {
    if (!s.name) {return;}
    addNode(`statefulset/${s.name}`, s.name, CAT.STATEFULSET);
  });

  (daemonsets || []).forEach(ds => {
    if (!ds.name) {return;}
    addNode(`daemonset/${ds.name}`, ds.name, CAT.DAEMONSET);
  });

  (pods || []).forEach(pod => {
    const name = pod.name;
    if (!name) {return;}
    const phase = pod.phase || "Unknown";
    addNode(`pod/${name}`, name, CAT.POD, POD_STATUS_COLOR[phase] || POD_STATUS_COLOR.Unknown);

    const podLabels = pod.labels || {};

    deployments.forEach(d => {
      if (selectorMatches(d.selector, podLabels)) {
        addLink(`deployment/${d.name}`, `pod/${name}`);
      }
    });

    statefulsets.forEach(s => {
      if (selectorMatches(s.selector, podLabels)) {
        addLink(`statefulset/${s.name}`, `pod/${name}`);
      }
    });

    daemonsets.forEach(ds => {
      if (selectorMatches(ds.selector, podLabels)) {
        addLink(`daemonset/${ds.name}`, `pod/${name}`);
      }
    });

    services.forEach(svc => {
      if (selectorMatches(svc.selector, podLabels)) {
        addLink(`service/${svc.name}`, `pod/${name}`);
      }
    });
  });

  return {nodes, links};
}

function buildOption(nodes, links) {
  return {
    tooltip: {
      formatter: (params) => {
        if (params.dataType === "node") {
          const [type, ...rest] = params.data.id.split("/");
          return `<b style="text-transform:capitalize">${type}</b><br/>${rest.join("/")}`;
        }
        return "";
      },
    },
    legend: {
      data: CATEGORIES.map(c => c.name),
      top: 8,
      left: "center",
      itemWidth: 12,
      itemHeight: 12,
      textStyle: {fontSize: 12},
    },
    series: [{
      type: "graph",
      layout: "force",
      data: nodes,
      links,
      categories: CATEGORIES.map(c => ({name: c.name, itemStyle: {color: c.color}})),
      roam: true,
      draggable: true,
      force: {
        repulsion: 320,
        gravity: 0.04,
        edgeLength: [90, 220],
        layoutAnimation: true,
      },
      label: {
        show: true,
        position: "bottom",
        fontSize: 11,
        formatter: (p) => {
          const n = p.data.name;
          return n.length > 22 ? n.slice(0, 20) + "…" : n;
        },
      },
      lineStyle: {
        opacity: 0.55,
        width: 1.5,
        color: "source",
        curveness: 0.08,
      },
      emphasis: {
        focus: "adjacency",
        lineStyle: {width: 3},
      },
      edgeSymbol: ["none", "arrow"],
      edgeSymbolSize: [0, 8],
    }],
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
    NamespaceBackend.getNamespaces().then(res => {
      if (res?.status === "ok" && Array.isArray(res.data)) {
        const names = res.data.map(n => n.name).filter(Boolean);
        setNamespaces(names);
        if (names.length > 0) {setNamespace(names[0]);}
      }
    });
  }, []);

  useEffect(() => {
    if (!namespace) {return;}
    setLoading(true);
    setError(null);
    Promise.all([
      DeploymentBackend.getDeployments(namespace),
      StatefulSetBackend.getStatefulSets(namespace),
      DaemonSetBackend.getDaemonSets(namespace),
      PodBackend.getPods(namespace),
      ServiceBackend.getServices(namespace),
      IngressBackend.getIngresses(namespace),
    ]).then(([deps, ssets, dsets, podList, svcs, ings]) => {
      const safeData = (r) => (r?.status === "ok" && Array.isArray(r.data)) ? r.data : [];
      setGraphData(buildGraphData(
        safeData(deps),
        safeData(ssets),
        safeData(dsets),
        safeData(podList),
        safeData(svcs),
        safeData(ings)
      ));
    }).catch(e => setError(e.message)).finally(() => setLoading(false));
  }, [namespace]);

  useEffect(() => {
    if (!containerRef.current) {return;}
    const chart = echarts.init(containerRef.current);
    chartRef.current = chart;
    const ro = new ResizeObserver(() => chart.resize());
    ro.observe(containerRef.current);
    chart.on("click", (params) => {
      if (params.dataType !== "node") {return;}
      const [type] = params.data.id.split("/");
      if (ROUTE_MAP[type]) {history.push(ROUTE_MAP[type]);}
    });
    return () => {
      ro.disconnect();
      chart.dispose();
      chartRef.current = null;
    };
  }, [history]);

  useEffect(() => {
    if (!chartRef.current) {return;}
    if (graphData.nodes.length === 0) {
      chartRef.current.clear();
      return;
    }
    chartRef.current.setOption(buildOption(graphData.nodes, graphData.links), {notMerge: true});
  }, [graphData]);

  const podLegend = [
    {label: "Running", color: POD_STATUS_COLOR.Running},
    {label: "Pending", color: POD_STATUS_COLOR.Pending},
    {label: "Failed", color: POD_STATUS_COLOR.Failed},
    {label: "Succeeded", color: POD_STATUS_COLOR.Succeeded},
  ];

  return (
    <Card
      title={i18next.t("general:Resource Topology")}
      style={{margin: "16px"}}
      extra={
        <div style={{display: "flex", alignItems: "center", gap: 12}}>
          <div style={{display: "flex", gap: 6}}>
            {podLegend.map(({label, color}) => (
              <Tag key={label} color={color} style={{margin: 0}}>{label}</Tag>
            ))}
          </div>
          <Select
            value={namespace}
            onChange={setNamespace}
            style={{width: 180}}
            placeholder={i18next.t("general:Select namespace")}
            options={namespaces.map(n => ({label: n, value: n}))}
          />
        </div>
      }
    >
      <Spin spinning={loading}>
        {error && <Alert type="error" message={error} style={{marginBottom: 12}} showIcon />}
        {!loading && graphData.nodes.length === 0 && !error && (
          <Empty
            description={i18next.t("general:No resources found")}
            style={{padding: "80px 0"}}
          />
        )}
        <div ref={containerRef} style={{width: "100%", height: "640px"}} />
      </Spin>
    </Card>
  );
}

export default TopologyPage;
