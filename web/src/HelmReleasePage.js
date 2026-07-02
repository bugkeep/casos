import React, {useEffect, useState} from "react";
import {Alert, Badge, Button, Drawer, Popconfirm, Select, Space, Spin, Table, Tag, Timeline, Tooltip, Typography} from "antd";
import {DeleteOutlined, HistoryOutlined, ReloadOutlined, RollbackOutlined, UpCircleOutlined} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import * as HelmBackend from "./backend/HelmBackend";
import HelmInstallModal from "./HelmInstallModal";

const {Text, Title} = Typography;

const STATUS_COLORS = {
  deployed: "success",
  failed: "error",
  pending: "warning",
  "pending-install": "warning",
  "pending-upgrade": "warning",
  "pending-rollback": "warning",
  superseded: "default",
  uninstalling: "processing",
};

function statusBadge(status) {
  const preset = STATUS_COLORS[status] ?? "default";
  return <Badge status={preset} text={status} />;
}

export default function HelmReleasePage() {
  const {t} = useTranslation();
  const [namespace, setNamespace] = useState("all");
  const [releases, setReleases] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const [historyDrawer, setHistoryDrawer] = useState(null);
  const [history, setHistory] = useState([]);
  const [historyLoading, setHistoryLoading] = useState(false);

  const [upgradeTarget, setUpgradeTarget] = useState(null);

  const fetchReleases = () => {
    setLoading(true);
    setError(null);
    HelmBackend.getHelmReleases(namespace)
      .then(res => {
        if (res.status === "ok") {
          setReleases(res.data ?? []);
        } else {
          setError(res.msg);
        }
      })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  };

  useEffect(() => {fetchReleases();}, [namespace]);

  const openHistory = (rel) => {
    setHistoryDrawer(rel);
    setHistory([]);
    setHistoryLoading(true);
    HelmBackend.getHelmReleaseHistory(rel.name, rel.namespace)
      .then(res => {
        if (res.status === "ok") {setHistory(res.data ?? []);}
      })
      .finally(() => setHistoryLoading(false));
  };

  const doRollback = (rel, revision) => {
    HelmBackend.rollbackHelmRelease({releaseName: rel.name, namespace: rel.namespace, revision})
      .then(res => {
        if (res.status === "ok") {
          setHistoryDrawer(null);
          fetchReleases();
        } else {
          alert(res.msg);
        }
      });
  };

  const doUninstall = (rel) => {
    HelmBackend.uninstallHelmRelease({releaseName: rel.name, namespace: rel.namespace})
      .then(res => {
        if (res.status === "ok") {
          fetchReleases();
        } else {
          setError(res.msg);
        }
      });
  };

  const parseChartName = (chartStr) => {
    const parts = chartStr?.split("-") ?? [];
    const versionIdx = parts.findIndex(p => /^\d/.test(p));
    return versionIdx > 0 ? parts.slice(0, versionIdx).join("-") : chartStr;
  };

  const parseChartVersion = (chartStr) => {
    const parts = chartStr?.split("-") ?? [];
    const versionIdx = parts.findIndex(p => /^\d/.test(p));
    return versionIdx > 0 ? parts.slice(versionIdx).join("-") : "";
  };

  const columns = [
    {
      title: t("helm:Release name"),
      dataIndex: "name",
      render: (v) => <Text strong>{v}</Text>,
    },
    {
      title: t("helm:Chart"),
      dataIndex: "chart",
      render: (v) => (
        <span>
          <Text>{parseChartName(v)}</Text>
          <Tag style={{marginLeft: 6, fontSize: 11}}>{parseChartVersion(v)}</Tag>
        </span>
      ),
    },
    {
      title: t("general:Namespaces"),
      dataIndex: "namespace",
      render: (v) => <Tag>{v}</Tag>,
    },
    {
      title: t("helm:Status"),
      dataIndex: "status",
      render: (v, record) => {
        const badge = statusBadge(v);
        if (v === "failed" && record.description) {
          return (
            <Tooltip title={record.description} color="red">
              {badge}
            </Tooltip>
          );
        }
        return badge;
      },
    },
    {
      title: t("helm:App version"),
      dataIndex: "app_version",
    },
    {
      title: t("helm:Last deployed"),
      dataIndex: "updated",
      render: (v) => v ? <Text style={{fontSize: 12}}>{v.slice(0, 19).replace("T", " ")}</Text> : "-",
    },
    {
      title: t("general:Action"),
      key: "action",
      render: (_, rel) => (
        <Space size="small">
          <Tooltip title={t("helm:Upgrade")}>
            <Button
              size="small"
              icon={<UpCircleOutlined />}
              onClick={() => {
                const chartName = parseChartName(rel.chart);
                setUpgradeTarget({
                  chartName,
                  repoURL: "",
                  version: parseChartVersion(rel.chart),
                  _releaseName: rel.name,
                  _namespace: rel.namespace,
                });
              }}
            />
          </Tooltip>
          <Tooltip title={t("helm:History")}>
            <Button size="small" icon={<HistoryOutlined />} onClick={() => openHistory(rel)} />
          </Tooltip>
          <Popconfirm
            title={t("helm:Uninstall release?")}
            description={`${rel.name} (${rel.namespace})`}
            onConfirm={() => doUninstall(rel)}
            okText={t("general:Delete")}
            okButtonProps={{danger: true}}
            cancelText={t("general:Cancel")}
          >
            <Tooltip title={t("helm:Uninstall")}>
              <Button size="small" danger icon={<DeleteOutlined />} />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div style={{padding: 24}}>
      <div style={{display: "flex", alignItems: "center", gap: 12, marginBottom: 16}}>
        <Title level={4} style={{margin: 0}}>{t("helm:Helm Releases")}</Title>
        <div style={{flex: 1}} />
        <Select
          value={namespace}
          onChange={setNamespace}
          options={[
            {value: "all", label: t("helm:All namespaces")},
          ]}
          style={{width: 160}}
          allowClear={false}
        />
        <Button icon={<ReloadOutlined />} onClick={fetchReleases} loading={loading}>
          {t("general:Refresh")}
        </Button>
      </div>

      {error && <Alert type="error" message={error} showIcon style={{marginBottom: 16}} />}

      <Table
        dataSource={releases}
        columns={columns}
        rowKey="name"
        loading={loading}
        size="small"
        pagination={{pageSize: 20, showSizeChanger: false}}
        locale={{emptyText: t("helm:No releases")}}
      />

      <Drawer
        title={historyDrawer ? `${t("helm:History")}: ${historyDrawer.name}` : ""}
        open={!!historyDrawer}
        onClose={() => setHistoryDrawer(null)}
        width={460}
      >
        {historyLoading ? (
          <div style={{textAlign: "center", padding: 40}}><Spin /></div>
        ) : (
          <Timeline
            items={history.map(h => ({
              color: h.status === "deployed" ? "green" : h.status === "failed" ? "red" : "blue",
              children: (
                <div>
                  <div style={{display: "flex", alignItems: "center", gap: 8}}>
                    <Text strong>#{h.revision}</Text>
                    <Tag style={{fontSize: 11}}>{h.chart}</Tag>
                    {statusBadge(h.status)}
                    <div style={{flex: 1}} />
                    {h.status !== "deployed" && (
                      <Button
                        size="small"
                        icon={<RollbackOutlined />}
                        onClick={() => doRollback(historyDrawer, h.revision)}
                      >
                        {t("helm:Rollback")}
                      </Button>
                    )}
                  </div>
                  <div style={{marginTop: 4, fontSize: 12, color: "rgba(0,0,0,0.45)"}}>
                    {h.updated?.slice(0, 19).replace("T", " ")}
                  </div>
                  {h.description && (
                    <div style={{marginTop: 4, fontSize: 12, color: "rgba(0,0,0,0.6)"}}>{h.description}</div>
                  )}
                </div>
              ),
            }))}
          />
        )}
      </Drawer>

      <HelmInstallModal
        open={!!upgradeTarget}
        chart={upgradeTarget}
        onClose={() => setUpgradeTarget(null)}
        onInstalled={() => {setUpgradeTarget(null); fetchReleases();}}
      />
    </div>
  );
}
