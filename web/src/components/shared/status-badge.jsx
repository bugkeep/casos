import {Badge} from "@/components/ui/badge";
import {cn} from "@/lib/utils";

// The old pages picked antd Tag colours by name. Keeping a translation table
// means those per-page colour maps port across unchanged instead of each page
// inventing its own palette.
const antdColorToVariant = {
  green: "success",
  success: "success",
  lime: "success",
  cyan: "info",
  blue: "info",
  geekblue: "info",
  processing: "info",
  gold: "warning",
  orange: "warning",
  warning: "warning",
  yellow: "warning",
  volcano: "danger",
  red: "danger",
  error: "danger",
  magenta: "danger",
  purple: "secondary",
  default: "muted",
};

export function tagVariant(color) {
  return antdColorToVariant[color] ?? "muted";
}

/** A Badge addressed by antd colour name, for one-to-one ports of Tag cells. */
export function ColorTag({color, className, children, ...props}) {
  return (
    <Badge variant={tagVariant(color)} className={className} {...props}>
      {children}
    </Badge>
  );
}

// Phases and conditions that recur across pods, nodes, jobs and releases. A page
// with a genuinely local vocabulary still passes its own map via `variants`.
const defaultStatusVariants = {
  Running: "success",
  Ready: "success",
  Active: "success",
  Available: "success",
  Bound: "success",
  Succeeded: "info",
  Complete: "info",
  Completed: "info",
  deployed: "success",
  Healthy: "success",
  True: "success",
  Pending: "warning",
  Progressing: "warning",
  ContainerCreating: "warning",
  Updating: "warning",
  pending: "warning",
  Terminating: "danger",
  Failed: "danger",
  Error: "danger",
  CrashLoopBackOff: "danger",
  ImagePullBackOff: "danger",
  Evicted: "danger",
  NotReady: "danger",
  failed: "danger",
  False: "danger",
  Unknown: "muted",
  "": "muted",
};

export function StatusBadge({status, variants, className, fallback = "Unknown", ...props}) {
  const label = status === undefined || status === null || status === "" ? fallback : String(status);
  const variant = (variants ?? {})[label] ?? defaultStatusVariants[label] ?? "muted";
  return (
    <Badge variant={variant} className={cn("font-medium", className)} {...props}>
      {label}
    </Badge>
  );
}

/**
 * "3/5"-style readiness, coloured by whether the two halves agree. Pods,
 * deployments, statefulsets and daemonsets all show this and all care about the
 * same distinction: fully ready, partially ready, or nothing up at all.
 */
export function ReadyBadge({ready, total, className}) {
  const readyCount = Number(ready) || 0;
  const totalCount = Number(total) || 0;
  const variant = totalCount > 0 && readyCount === totalCount ? "success" : readyCount === 0 ? "danger" : "warning";
  return (
    <Badge variant={variant} className={cn("tabular-nums", className)}>
      {readyCount}/{totalCount}
    </Badge>
  );
}
