import React, {useCallback, useEffect, useRef, useState} from "react";
import {Alert, Button, Card, Col, Divider, Form, Input, Modal, Popconfirm, Row, Spin, Tag, Tooltip, Typography} from "antd";
import {DeleteOutlined, PlusOutlined, ReloadOutlined, RocketOutlined, ShopOutlined} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import {Link} from "react-router-dom";
import * as HelmBackend from "./backend/HelmBackend";
import HelmInstallModal from "./HelmInstallModal";

const {Text, Paragraph, Title} = Typography;

const PRESET_REPOS = [
  {name: "ArtifactHub", url: null, desc: "artifacthub.io — 8 000+ charts"},
  {name: "Bitnami", url: "https://charts.bitnami.com/bitnami", desc: "~200 curated charts"},
  {name: "Rancher", url: "https://charts.rancher.io", desc: "Rancher Charts"},
  {name: "ingress-nginx", url: "https://kubernetes.github.io/ingress-nginx", desc: "Official ingress-nginx"},
];

function isSupportedHelmRepoURL(value) {
  try {
    const parsed = new URL(value);
    return parsed.protocol === "http:" || parsed.protocol === "https:" || parsed.protocol === "oci:";
  } catch {
    return false;
  }
}

function ChartIcon({icon, name, size = 40}) {
  const [err, setErr] = useState(false);
  if (!err && icon) {
    return (
      <img
        src={icon}
        alt={name}
        style={{width: size, height: size, objectFit: "contain"}}
        onError={() => setErr(true)}
      />
    );
  }
  return (
    <div style={{
      width: size, height: size,
      background: "#f0f0f0", borderRadius: 8,
      display: "flex", alignItems: "center", justifyContent: "center",
      fontSize: 18, fontWeight: 600, color: "#888",
    }}>
      {(name || "?")[0].toUpperCase()}
    </div>
  );
}

function ChartCard({chart, onInstall}) {
  const {t} = useTranslation();
  return (
    <Card
      hoverable
      size="small"
      style={{height: "100%"}}
      styles={{body: {padding: 12}}}
    >
      <div style={{display: "flex", gap: 10, alignItems: "flex-start"}}>
        <ChartIcon icon={chart.icon || chart.logo_url} name={chart.display_name || chart.name} />
        <div style={{flex: 1, minWidth: 0}}>
          <Text strong style={{fontSize: 13}}>{chart.display_name || chart.name}</Text>
          {chart.version && (
            <Tag style={{marginLeft: 6, fontSize: 11}}>{chart.version}</Tag>
          )}
          <Paragraph
            ellipsis={{rows: 2}}
            style={{marginTop: 3, marginBottom: 6, fontSize: 12, color: "rgba(0,0,0,0.5)"}}
          >
            {chart.description || ""}
          </Paragraph>
          <div style={{display: "flex", justifyContent: "flex-end"}}>
            <Button type="primary" size="small" icon={<RocketOutlined />} onClick={() => onInstall(chart)}>
              {t("helm:Install")}
            </Button>
          </div>
        </div>
      </div>
    </Card>
  );
}

function AddRepoModal({open, onClose, onAdded}) {
  const {t} = useTranslation();
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);

  function handleOk() {
    form.validateFields().then(values => {
      setLoading(true);
      HelmBackend.addHelmRepo(values)
        .then(res => {
          if (res.status === "ok") {
            form.resetFields();
            onAdded();
            onClose();
          } else {
            Modal.error({title: t("helm:Add repo failed"), content: res.msg});
          }
        })
        .finally(() => setLoading(false));
    });
  }

  return (
    <Modal
      title={t("helm:Add Helm Repo")}
      open={open}
      onOk={handleOk}
      onCancel={onClose}
      confirmLoading={loading}
      destroyOnHidden
    >
      <Form form={form} layout="vertical">
        <Form.Item name="name" label={t("helm:Repo name")} rules={[{required: true}]}>
          <Input placeholder="my-charts" />
        </Form.Item>
        <Form.Item
          name="url"
          label={t("helm:Repo URL")}
          rules={[
            {required: true},
            {
              validator: (_, value) => {
                if (!value || isSupportedHelmRepoURL(value)) {
                  return Promise.resolve();
                }
                return Promise.reject(new Error("Use http(s)://... or oci://... with an explicit tag or digest"));
              },
            },
          ]}
        >
          <Input placeholder="https://example.com/charts or oci://registry/chart:tag" />
        </Form.Item>
      </Form>
    </Modal>
  );
}

export default function AppStorePage() {
  const {t} = useTranslation();
  const [source, setSource] = useState(PRESET_REPOS[0]);
  const [query, setQuery] = useState("");
  const [charts, setCharts] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(true);
  const [customRepos, setCustomRepos] = useState([]);
  const [addRepoOpen, setAddRepoOpen] = useState(false);
  const [installTarget, setInstallTarget] = useState(null);

  const queryRef = useRef(query);
  const sourceRef = useRef(source);
  queryRef.current = query;
  sourceRef.current = source;

  const loadCustomRepos = () => {
    HelmBackend.getHelmRepos().then(res => {
      if (res.status === "ok") {setCustomRepos(res.data ?? []);}
    });
  };

  useEffect(() => {loadCustomRepos();}, []);

  const fetchCharts = useCallback((s, q, p) => {
    setLoading(true);
    setError(null);

    const isAH = !s.url;
    const promise = isAH
      ? HelmBackend.searchArtifactHub(q, p)
      : HelmBackend.getRepoCharts(s.url);

    promise.then(res => {
      if (res.status !== "ok") {
        setError(res.msg);
        return;
      }
      const data = res.data ?? [];
      if (isAH) {
        setCharts(p === 1 ? data : prev => [...prev, ...data]);
        setHasMore(data.length === 20);
      } else {
        setCharts(data);
        setHasMore(false);
      }
    }).catch(e => setError(e.message)).finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    setCharts([]);
    setPage(1);
    setHasMore(true);
    fetchCharts(source, query, 1);
  }, [source, query, fetchCharts]);

  useEffect(() => {
    if (page > 1) {fetchCharts(source, query, page);}
  }, [page, fetchCharts, source, query]);

  const isAH = !source.url;

  const filteredCharts = isAH ? charts : charts.filter(c => {
    const q = query.toLowerCase();
    return !q || (c.name || "").toLowerCase().includes(q) || (c.description || "").toLowerCase().includes(q);
  });

  const getChartInstallInfo = (chart) => {
    if (isAH) {
      return {
        chartName: chart.name,
        repoURL: chart.repository?.url ?? "",
        version: chart.version ?? "",
        displayName: chart.display_name || chart.name,
        icon: chart.logo_image_id
          ? `https://artifacthub.io/image/${chart.logo_image_id}`
          : null,
      };
    }
    return {
      chartName: chart.name,
      repoURL: source.url,
      version: chart.version ?? "",
      displayName: chart.name,
      icon: chart.icon,
    };
  };

  const deleteCustomRepo = (id) => {
    HelmBackend.deleteHelmRepo(id).then(res => {
      if (res.status === "ok") {
        loadCustomRepos();
        if (source.id === id) {setSource(PRESET_REPOS[0]);}
      }
    });
  };

  return (
    <div style={{display: "flex", height: "100%", overflow: "hidden"}}>
      {/* Sidebar */}
      <div style={{
        width: 200, flexShrink: 0, borderRight: "1px solid rgba(0,0,0,0.06)",
        padding: "16px 0", overflowY: "auto", background: "#fafafa",
      }}>
        <div style={{padding: "0 12px 8px", fontSize: 11, fontWeight: 600, color: "rgba(0,0,0,0.4)", textTransform: "uppercase", letterSpacing: 1}}>
          {t("helm:Sources")}
        </div>
        {PRESET_REPOS.map(repo => (
          <Tooltip key={repo.name} title={repo.desc} placement="right">
            <div
              onClick={() => {setSource(repo); setQuery("");}}
              style={{
                padding: "7px 12px", cursor: "pointer", fontSize: 13,
                borderRadius: 4, margin: "1px 8px",
                background: source.name === repo.name ? "#e6f4ff" : "transparent",
                color: source.name === repo.name ? "#1677ff" : "inherit",
                fontWeight: source.name === repo.name ? 500 : 400,
              }}
            >
              {repo.name}
            </div>
          </Tooltip>
        ))}

        {customRepos.length > 0 && (
          <>
            <Divider style={{margin: "8px 0"}} />
            <div style={{padding: "0 12px 6px", fontSize: 11, fontWeight: 600, color: "rgba(0,0,0,0.4)", textTransform: "uppercase", letterSpacing: 1}}>
              {t("helm:My Repos")}
            </div>
            {customRepos.map(repo => (
              <div
                key={repo.id}
                style={{
                  display: "flex", alignItems: "center", padding: "7px 12px",
                  margin: "1px 8px", borderRadius: 4, cursor: "pointer",
                  background: source.id === repo.id ? "#e6f4ff" : "transparent",
                  color: source.id === repo.id ? "#1677ff" : "inherit",
                }}
              >
                <div
                  style={{flex: 1, fontSize: 13, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap"}}
                  onClick={() => {setSource({...repo, url: repo.url}); setQuery("");}}
                >
                  {repo.name}
                </div>
                <Popconfirm
                  title={t("helm:Delete repo?")}
                  onConfirm={() => deleteCustomRepo(repo.id)}
                  okText={t("general:Delete")}
                  cancelText={t("general:Cancel")}
                >
                  <DeleteOutlined style={{fontSize: 11, color: "rgba(0,0,0,0.35)"}} onClick={e => e.stopPropagation()} />
                </Popconfirm>
              </div>
            ))}
          </>
        )}

        <div style={{padding: "8px 12px"}}>
          <Button
            size="small"
            icon={<PlusOutlined />}
            type="dashed"
            block
            onClick={() => setAddRepoOpen(true)}
          >
            {t("helm:Add Repo")}
          </Button>
        </div>
      </div>

      {/* Main */}
      <div style={{flex: 1, padding: 20, overflowY: "auto"}}>
        <div style={{display: "flex", alignItems: "center", gap: 12, marginBottom: 16}}>
          <ShopOutlined style={{fontSize: 20}} />
          <Title level={4} style={{margin: 0}}>{source.name}</Title>
          <div style={{flex: 1}} />
          <Link to="/helm-releases">
            <Button size="small">{t("helm:My Releases")} →</Button>
          </Link>
        </div>

        <div style={{display: "flex", gap: 8, marginBottom: 16}}>
          <Input.Search
            placeholder={t("helm:Search charts")}
            value={query}
            onChange={e => setQuery(e.target.value)}
            onSearch={v => setQuery(v)}
            style={{width: 280}}
            allowClear
          />
          <Button icon={<ReloadOutlined />} onClick={() => {setCharts([]); setPage(1); fetchCharts(source, query, 1);}} loading={loading}>
            {t("general:Refresh")}
          </Button>
        </div>

        {error && (
          <Alert type="error" message={error} showIcon style={{marginBottom: 16}} />
        )}

        <Row gutter={[12, 12]}>
          {filteredCharts.map((chart, i) => {
            const info = getChartInstallInfo(chart);
            return (
              <Col key={`${info.chartName}-${i}`} xs={24} sm={12} lg={8} xl={6}>
                <ChartCard
                  chart={{
                    ...chart,
                    icon: info.icon || chart.icon,
                    display_name: info.displayName,
                  }}
                  onInstall={() => setInstallTarget(info)}
                />
              </Col>
            );
          })}
        </Row>

        {loading && (
          <div style={{textAlign: "center", padding: "40px 0"}}>
            <Spin />
          </div>
        )}

        {!loading && isAH && hasMore && filteredCharts.length > 0 && (
          <div style={{textAlign: "center", marginTop: 20}}>
            <Button onClick={() => setPage(p => p + 1)}>{t("helm:Load more")}</Button>
          </div>
        )}

        {!loading && filteredCharts.length === 0 && !error && (
          <div style={{textAlign: "center", color: "rgba(0,0,0,0.4)", padding: "60px 0"}}>
            {t("helm:No charts found")}
          </div>
        )}
      </div>

      <AddRepoModal
        open={addRepoOpen}
        onClose={() => setAddRepoOpen(false)}
        onAdded={loadCustomRepos}
      />

      <HelmInstallModal
        open={!!installTarget}
        chart={installTarget}
        onClose={() => setInstallTarget(null)}
        onInstalled={() => setInstallTarget(null)}
      />
    </div>
  );
}
