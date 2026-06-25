import React, {useEffect, useState} from "react";
import {Alert, Form, Input, Modal, Select, Spin, Typography} from "antd";
import {useTranslation} from "react-i18next";
import * as HelmBackend from "./backend/HelmBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";

const {Text} = Typography;

export default function HelmInstallModal({open, chart, onClose, onInstalled}) {
  const {t} = useTranslation();
  const [form] = Form.useForm();
  const [namespaces, setNamespaces] = useState([]);
  const [valuesYAML, setValuesYAML] = useState("");
  const [valuesLoading, setValuesLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (!open || !chart) {return;}
    setError(null);

    NamespaceBackend.getNamespaces().then(res => {
      if (res.status === "ok") {
        const ns = res.data ?? [];
        setNamespaces(ns);
        const def = ns.find(n => n.name === "default") ? "default" : (ns[0]?.name ?? "default");
        form.setFieldsValue({
          releaseName: chart.chartName,
          namespace: def,
          version: chart.version ?? "",
        });
      }
    });

    if (chart.chartName && chart.repoURL) {
      setValuesLoading(true);
      setValuesYAML("");
      HelmBackend.getHelmChartValues(chart.chartName, chart.repoURL, chart.version ?? "")
        .then(res => {
          if (res.status === "ok") {
            setValuesYAML(res.data ?? "");
          } else {
            setError(res.msg);
          }
        })
        .finally(() => setValuesLoading(false));
    }
  }, [open, chart, form]);

  const handleClose = () => {
    form.resetFields();
    setValuesYAML("");
    setError(null);
    onClose();
  };

  const handleOk = () => {
    form.validateFields().then(values => {
      setSubmitting(true);
      setError(null);
      HelmBackend.installHelmChart({
        releaseName: values.releaseName,
        namespace: values.namespace,
        chartName: chart.chartName,
        repoURL: chart.repoURL,
        version: values.version || chart.version,
        valuesYAML,
      }).then(res => {
        if (res.status === "ok") {
          onInstalled?.();
          handleClose();
        } else {
          setError(res.msg);
        }
      }).catch(e => setError(e.message)).finally(() => setSubmitting(false));
    });
  };

  if (!chart) {return null;}

  const nsOptions = namespaces.map(ns => ({label: ns.name, value: ns.name}));

  return (
    <Modal
      title={
        <span>
          {t("helm:Install chart")} <Text code>{chart.chartName}</Text>
          {chart.repoURL && (
            <Text style={{marginLeft: 8, fontSize: 12, color: "rgba(0,0,0,0.45)"}}>
              {chart.repoURL}
            </Text>
          )}
        </span>
      }
      open={open}
      onOk={handleOk}
      onCancel={handleClose}
      okText={t("helm:Install")}
      confirmLoading={submitting}
      width={680}
      destroyOnHidden
    >
      {error && <Alert type="error" message={error} showIcon style={{marginBottom: 16}} closable onClose={() => setError(null)} />}

      <Form form={form} layout="vertical">
        <div style={{display: "flex", gap: 12}}>
          <Form.Item
            style={{flex: 1}}
            label={t("helm:Release name")}
            name="releaseName"
            rules={[
              {required: true},
              {pattern: /^[a-z0-9][a-z0-9-]*$/, message: t("helm:Release name pattern")},
            ]}
          >
            <Input />
          </Form.Item>
          <Form.Item style={{flex: 1}} label={t("general:Namespaces")} name="namespace" rules={[{required: true}]}>
            <Select options={nsOptions} showSearch />
          </Form.Item>
          <Form.Item style={{width: 130}} label={t("helm:Version")} name="version">
            <Input placeholder={chart.version ?? "latest"} />
          </Form.Item>
        </div>

        <Form.Item label={t("helm:Values (YAML)")}>
          {valuesLoading ? (
            <div style={{textAlign: "center", padding: 24}}>
              <Spin size="small" /> <Text style={{marginLeft: 8, color: "rgba(0,0,0,0.45)"}}>{t("helm:Loading values")}</Text>
            </div>
          ) : (
            <textarea
              value={valuesYAML}
              onChange={e => setValuesYAML(e.target.value)}
              rows={14}
              style={{
                width: "100%", fontFamily: "monospace", fontSize: 12,
                padding: "8px 10px", borderRadius: 6,
                border: "1px solid #d9d9d9", resize: "vertical", outline: "none",
                boxSizing: "border-box",
              }}
              spellCheck={false}
            />
          )}
        </Form.Item>
      </Form>
    </Modal>
  );
}
