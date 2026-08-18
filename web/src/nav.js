import {
  AppWindow,
  Boxes,
  ClipboardList,
  Cog,
  Gauge,
  LayoutDashboard,
  Lock,
  Network,
  Server,
  Store,
} from "lucide-react";

/**
 * One description of the navigation, read by both the sidebar and the
 * breadcrumb. Keeping them on the same tree is what stops the two from
 * disagreeing about which section a page belongs to — the old UI maintained
 * that mapping twice and they drifted.
 *
 * `label` is an i18next key, resolved at render time so a language switch does
 * not need the tree rebuilt.
 */
export const navGroups = [
  {key: "/dashboard", label: "general:Dashboard", icon: LayoutDashboard, path: "/dashboard"},
  {key: "/app-store", label: "general:App Store", icon: Store, path: "/app-store"},
  {key: "/helm-releases", label: "helm:Helm Releases", icon: Boxes, path: "/helm-releases"},
  {
    key: "/workloads",
    label: "general:Workloads",
    icon: AppWindow,
    children: [
      {key: "/pods", label: "general:Pods", path: "/pods"},
      {key: "/deployments", label: "general:Deployments", path: "/deployments"},
      {key: "/statefulsets", label: "general:Stateful Sets", path: "/statefulsets"},
      {key: "/daemonsets", label: "general:Daemon Sets", path: "/daemonsets"},
      {key: "/jobs", label: "general:Jobs", path: "/jobs"},
      {key: "/cronjobs", label: "general:Cron Jobs", path: "/cronjobs"},
    ],
  },
  {
    key: "/cluster",
    label: "general:Cluster",
    icon: Server,
    children: [
      {key: "/nodes", label: "general:Nodes", path: "/nodes"},
      {key: "/namespaces", label: "general:Namespaces", path: "/namespaces"},
      {key: "/serviceaccounts", label: "general:ServiceAccounts", path: "/serviceaccounts"},
    ],
  },
  {
    key: "/configuration",
    label: "general:Configuration",
    icon: Cog,
    children: [
      {key: "/configmaps", label: "general:ConfigMaps", path: "/configmaps"},
      {key: "/secrets", label: "general:Secrets", path: "/secrets"},
      {key: "/pvcs", label: "general:Persistent Volume Claims", path: "/pvcs"},
      {key: "/storageclasses", label: "general:Storage Classes", path: "/storageclasses"},
      {key: "/resourcequotas", label: "general:Resource Quotas", path: "/resourcequotas"},
      {key: "/hpas", label: "general:Horizontal Pod Autoscaler", path: "/hpas"},
    ],
  },
  {
    key: "/networking",
    label: "general:Networking",
    icon: Network,
    children: [
      {key: "/services", label: "general:Services", path: "/services"},
      {key: "/ingresses", label: "general:Ingresses", path: "/ingresses"},
      {key: "/networkpolicies", label: "general:Network Policies", path: "/networkpolicies"},
    ],
  },
  {
    key: "/accesscontrol",
    label: "general:Access Control",
    icon: Lock,
    children: [
      {key: "/rolebindings", label: "general:Role Bindings", path: "/rolebindings"},
      {key: "/clusterrolebindings", label: "general:ClusterRoleBindings", path: "/clusterrolebindings"},
      {key: "/admission-policy", label: "general:Admission Policy", path: "/admission-policy"},
      {key: "/authorization-policy", label: "general:Authorization Policy", path: "/authorization-policy"},
      {key: "/trivy-scans", label: "general:Image Scan", path: "/trivy-scans"},
    ],
  },
  {
    key: "/observability",
    label: "general:Observability",
    icon: Gauge,
    children: [
      {key: "/monitor", label: "general:Monitor Center", path: "/monitor"},
      {key: "/log-search", label: "general:Log Search", path: "/log-search"},
      {key: "/topology", label: "general:Resource Topology", path: "/topology"},
    ],
  },
  {
    key: "/infrastructure",
    label: "general:Infrastructure",
    icon: Server,
    children: [{key: "/machines", label: "general:Machines", path: "/machines"}],
  },
  {
    key: "/admin",
    label: "general:Admin",
    icon: ClipboardList,
    children: [{key: "/sites", label: "general:Sites", path: "/sites/site-built-in"}],
  },
];

/** All leaf entries, flattened, for lookups by first path segment. */
export const navLeaves = navGroups.flatMap((group) => (group.children ? group.children : [group]));

export function findLeaf(segmentKey) {
  return navLeaves.find((leaf) => leaf.key === segmentKey);
}

export function findGroupOf(segmentKey) {
  return navGroups.find((group) => group.children?.some((child) => child.key === segmentKey));
}
