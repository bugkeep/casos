import React, {useEffect, useState} from "react";
import {useTranslation} from "react-i18next";
import {CircleArrowUp, History, RefreshCw, RotateCcw, Trash2} from "lucide-react";
import * as HelmBackend from "@/backend/HelmBackend";
import * as Setting from "@/Setting";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {MessageAlert} from "@/components/ui/alert";
import {SimpleTooltip} from "@/components/ui/tooltip";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {ResourceSheet} from "@/components/shared/resource-sheet";
import {SimpleSelect} from "@/components/shared/simple-select";
import {Loading} from "@/components/shared/loading";
import {HelmInstallDialog} from "@/components/shared/helm-install-dialog";

const STATUS_VARIANTS = {
  deployed: "success",
  failed: "danger",
  pending: "warning",
  "pending-install": "warning",
  "pending-upgrade": "warning",
  "pending-rollback": "warning",
  superseded: "muted",
  uninstalling: "info",
};

// Helm reports the chart as "name-version" in one string. Splitting on the first
// segment that starts with a digit is what separates "ingress-nginx" from
// "4.11.2" — chart names routinely contain dashes themselves.
function parseChartName(chart) {
  const parts = chart?.split("-") ?? [];
  const versionIndex = parts.findIndex((part) => /^\d/.test(part));
  return versionIndex > 0 ? parts.slice(0, versionIndex).join("-") : chart;
}

function parseChartVersion(chart) {
  const parts = chart?.split("-") ?? [];
  const versionIndex = parts.findIndex((part) => /^\d/.test(part));
  return versionIndex > 0 ? parts.slice(versionIndex).join("-") : "";
}

export function helmReleaseUpgradeTarget(release) {
  return {
    releaseName: release.name,
    namespace: release.namespace,
    chartName: release.chartName || parseChartName(release.chart),
    repoURL: release.repoURL,
    version: release.chartVersion || parseChartVersion(release.chart),
  };
}

function StatusBadge({status, description}) {
  const badge = <Badge variant={STATUS_VARIANTS[status] ?? "muted"}>{status}</Badge>;
  if (status === "failed" && description) {
    return <SimpleTooltip title={description}>{badge}</SimpleTooltip>;
  }
  return badge;
}

export default function HelmReleasePage() {
  const {t} = useTranslation();
  const [namespace, setNamespace] = useState("all");
  const [releases, setReleases] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const [historyRelease, setHistoryRelease] = useState(null);
  const [history, setHistory] = useState([]);
  const [historyLoading, setHistoryLoading] = useState(false);

  const [upgradeTarget, setUpgradeTarget] = useState(null);

  function fetchReleases() {
    setLoading(true);
    setError(null);
    HelmBackend.getHelmReleases(namespace)
      .then((res) => {
        if (res.status === "ok") {
          setReleases(res.data ?? []);
        } else {
          setError(res.msg);
        }
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    fetchReleases();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [namespace]);

  function openHistory(release) {
    setHistoryRelease(release);
    setHistory([]);
    setHistoryLoading(true);
    HelmBackend.getHelmReleaseHistory(release.name, release.namespace)
      .then((res) => {
        if (res.status === "ok") {
          setHistory(res.data ?? []);
        }
      })
      .finally(() => setHistoryLoading(false));
  }

  function handleRollback(release, revision) {
    HelmBackend.rollbackHelmRelease({releaseName: release.name, namespace: release.namespace, revision}).then((res) => {
      if (res.status === "ok") {
        Setting.showMessage("success", `Rolled back to revision ${revision}`);
        setHistoryRelease(null);
        fetchReleases();
      } else {
        Setting.showMessage("error", res.msg);
      }
    });
  }

  function handleUninstall(release) {
    return HelmBackend.uninstallHelmRelease({releaseName: release.name, namespace: release.namespace}).then((res) => {
      if (res.status === "ok") {
        Setting.showMessage("success", `Uninstalled ${release.name}`);
        fetchReleases();
      } else {
        setError(res.msg);
        Setting.showMessage("error", res.msg);
      }
    });
  }

  const columns = [
    {key: "name", title: t("helm:Release name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {
      key: "chart",
      title: t("helm:Chart"),
      dataIndex: "chart",
      render: (value, release) => (
        <span className="flex items-center gap-1.5">
          {release.chartName || parseChartName(value)}
          <Badge variant="muted">{release.chartVersion || parseChartVersion(value)}</Badge>
        </span>
      ),
    },
    {
      key: "namespace",
      title: t("general:Namespaces"),
      dataIndex: "namespace",
      width: 170,
      sortable: true,
      render: (value) => <Badge variant="muted">{value}</Badge>,
    },
    {
      key: "status",
      title: t("general:Status"),
      dataIndex: "status",
      width: 150,
      sortable: true,
      render: (value, record) => <StatusBadge status={value} description={record.description} />,
    },
    {key: "app_version", title: t("helm:App version"), dataIndex: "app_version", width: 140},
    {
      key: "updated",
      title: t("helm:Last deployed"),
      dataIndex: "updated",
      width: 190,
      sortable: true,
      render: (value) => (value ? <span className="text-xs">{value.slice(0, 19).replace("T", " ")}</span> : "-"),
    },
    {
      key: "action",
      title: t("general:Action"),
      width: 150,
      align: "right",
      render: (_, release) => (
        <div className="flex justify-end gap-1">
          <SimpleTooltip title={t("helm:Upgrade")}>
            <Button
              variant="outline"
              size="icon-sm"
              onClick={() => setUpgradeTarget(helmReleaseUpgradeTarget(release))}
              aria-label="Upgrade"
            >
              <CircleArrowUp className="size-4" />
            </Button>
          </SimpleTooltip>
          <SimpleTooltip title={t("helm:History")}>
            <Button variant="outline" size="icon-sm" onClick={() => openHistory(release)} aria-label="History">
              <History className="size-4" />
            </Button>
          </SimpleTooltip>
          <ConfirmDialog
            title={t("helm:Uninstall release?")}
            description={`${release.name} (${release.namespace})`}
            confirmText={t("general:Delete")}
            cancelText={t("general:Cancel")}
            onConfirm={() => handleUninstall(release)}
          >
            <Button variant="outline" size="icon-sm" className="text-destructive" aria-label="Uninstall">
              <Trash2 className="size-4" />
            </Button>
          </ConfirmDialog>
        </div>
      ),
    },
  ];

  return (
    <PageContainer>
      {error ? <MessageAlert title={error} /> : null}

      <DataTable
        title={t("helm:Helm Releases")}
        description={`${releases.length} releases`}
        columns={columns}
        dataSource={releases}
        rowKey="name"
        loading={loading}
        searchable
        emptyText={t("helm:No releases")}
        toolbar={
          <>
            <SimpleSelect
              value={namespace}
              onChange={setNamespace}
              options={[{value: "all", label: t("helm:All namespaces")}]}
              className="w-44"
            />
            <Button variant="outline" size="sm" onClick={fetchReleases} loading={loading}>
              <RefreshCw />
              {t("general:Refresh")}
            </Button>
          </>
        }
      />

      <ResourceSheet
        open={Boolean(historyRelease)}
        onOpenChange={(next) => (next ? null : setHistoryRelease(null))}
        title={historyRelease ? `${t("helm:History")}: ${historyRelease.name}` : ""}
        size="md"
        bodyClassName="overflow-y-auto scrollbar-thin"
      >
        {historyLoading ? (
          <Loading />
        ) : (
          <ol className="relative grid gap-5 border-l pl-5">
            {history.map((revision) => (
              <li key={revision.revision} className="relative">
                <span
                  className={`absolute -left-[27px] top-1.5 size-2.5 rounded-full ${
                    revision.status === "deployed"
                      ? "bg-success"
                      : revision.status === "failed"
                        ? "bg-destructive"
                        : "bg-info"
                  }`}
                />
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-semibold">#{revision.revision}</span>
                  <Badge variant="muted">{revision.chart}</Badge>
                  <Badge variant={STATUS_VARIANTS[revision.status] ?? "muted"}>{revision.status}</Badge>
                  <div className="flex-1" />
                  {revision.status !== "deployed" ? (
                    <Button variant="outline" size="sm" onClick={() => handleRollback(historyRelease, revision.revision)}>
                      <RotateCcw />
                      {t("helm:Rollback")}
                    </Button>
                  ) : null}
                </div>
                <div className="text-muted-foreground mt-1 text-xs">{revision.updated?.slice(0, 19).replace("T", " ")}</div>
                {revision.description ? <div className="mt-1 text-xs">{revision.description}</div> : null}
              </li>
            ))}
          </ol>
        )}
      </ResourceSheet>

      <HelmInstallDialog
        open={Boolean(upgradeTarget)}
        action="upgrade"
        chart={upgradeTarget}
        onClose={() => setUpgradeTarget(null)}
        onInstalled={() => {
          setUpgradeTarget(null);
          fetchReleases();
        }}
      />
    </PageContainer>
  );
}
