import React, {useEffect, useRef, useState} from "react";
import {Alert, Button, Form, Input, Modal, Select, Spin, Typography} from "antd";
import {useTranslation} from "react-i18next";
import * as HelmBackend from "./backend/HelmBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";

const {Text} = Typography;

const helmTaskStoragePrefix = chartName => `casos.helmTask.${encodeURIComponent(chartName)}.`;

const helmTaskStorageKey = (chartName, namespace, releaseName) =>
  `${helmTaskStoragePrefix(chartName)}${encodeURIComponent(namespace)}.${encodeURIComponent(releaseName)}`;

const removeStoredHelmTask = key => {
  if (!key) {return;}
  try {
    window.localStorage.removeItem(key);
  } catch (_) {
    // Storage may be unavailable; task polling still works for this session.
  }
};

const findStoredHelmTask = chartName => {
  const prefix = helmTaskStoragePrefix(chartName);
  const matches = [];
  const invalidKeys = [];
  try {
    for (let i = 0; i < window.localStorage.length; i += 1) {
      const key = window.localStorage.key(i);
      if (!key?.startsWith(prefix)) {continue;}
      const raw = window.localStorage.getItem(key);
      try {
        const stored = JSON.parse(raw);
        if (stored?.taskId) {
          matches.push({
            key,
            taskId: String(stored.taskId),
            createdAt: Number(stored.createdAt) || 0,
            namespace: stored.namespace,
            releaseName: stored.releaseName,
          });
        }
      } catch (_) {
        if (/^\d+$/.test(raw ?? "")) {
          matches.push({key, taskId: raw, createdAt: 0});
        } else {
          invalidKeys.push(key);
        }
      }
    }
  } catch (_) {
    return null;
  }
  invalidKeys.forEach(removeStoredHelmTask);
  return matches.sort((a, b) => b.createdAt - a.createdAt)[0] ?? null;
};

export default function HelmInstallModal({open, chart, onClose, onInstalled}) {
  const {t} = useTranslation();
  const [form] = Form.useForm();
  const [namespaces, setNamespaces] = useState([]);
  const [valuesYAML, setValuesYAML] = useState("");
  const [valuesLoading, setValuesLoading] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState(null);
  const [logs, setLogs] = useState([]);
  const logEndRef = useRef(null);
  const taskIdRef = useRef(null);
  const taskStorageKeyRef = useRef(null);
  const pollTimerRef = useRef(null);
  const mountedRef = useRef(true);
  const submittingRef = useRef(false);

  const stopTaskPolling = () => {
    if (pollTimerRef.current) {
      clearTimeout(pollTimerRef.current);
      pollTimerRef.current = null;
    }
  };

  const monitorTask = (taskId, storageKey = taskStorageKeyRef.current) => {
    stopTaskPolling();
    const poll = () => {
      HelmBackend.getHelmOperationTask(taskId)
        .then(res => {
          if (!mountedRef.current) {return;}
          if (res.status !== "ok") {
            if (res.msg === "Helm operation task not found" && storageKey) {
              removeStoredHelmTask(storageKey);
            }
            setError(res.msg);
            setInstalling(false);
            submittingRef.current = false;
            return;
          }
          const task = res.data;
          setLogs((res.data2 ?? []).map(log => log.message));
          if (task.status === "succeeded") {
            setDone(true);
            setInstalling(false);
            submittingRef.current = false;
            removeStoredHelmTask(storageKey);
            return;
          }
          if (task.status === "failed") {
            setError(task.errorMsg || "Helm operation failed");
            setInstalling(false);
            submittingRef.current = false;
            removeStoredHelmTask(storageKey);
            return;
          }
          setInstalling(true);
          pollTimerRef.current = setTimeout(poll, 2000);
        })
        .catch(e => {
          if (!mountedRef.current) {return;}
          setError(e.message);
          setInstalling(false);
          submittingRef.current = false;
        });
    };
    poll();
  };

  useEffect(() => {
    if (!open || !chart) {return;}
    setError(null);
    setLogs([]);
    setDone(false);
    setInstalling(false);
    taskIdRef.current = null;
    taskStorageKeyRef.current = null;
    submittingRef.current = false;
    stopTaskPolling();

    const savedTask = findStoredHelmTask(chart.chartName);
    if (savedTask) {
      taskIdRef.current = savedTask.taskId;
      taskStorageKeyRef.current = savedTask.key;
      submittingRef.current = true;
      setInstalling(true);
      monitorTask(savedTask.taskId, savedTask.key);
    }

    NamespaceBackend.getNamespaces().then(res => {
      if (!mountedRef.current) {return;}
      if (res.status === "ok") {
        const ns = res.data ?? [];
        setNamespaces(ns);
        const def = ns.find(n => n.name === "default") ? "default" : (ns[0]?.name ?? "default");
        form.setFieldsValue({
          releaseName: savedTask?.releaseName || chart.chartName,
          namespace: savedTask?.namespace || def,
          version: chart.version ?? "",
        });
      }
    });

    if (chart.chartName && chart.repoURL) {
      setValuesLoading(true);
      setValuesYAML("");
      HelmBackend.getHelmChartValues(chart.chartName, chart.repoURL, chart.version ?? "")
        .then(res => {
          if (!mountedRef.current) {return;}
          if (res.status === "ok") {
            setValuesYAML(res.data ?? "");
          } else {
            setError(res.msg);
          }
        })
        .finally(() => {
          if (mountedRef.current) {setValuesLoading(false);}
        });
    }
  }, [open, chart, form]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      stopTaskPolling();
    };
  }, []);

  useEffect(() => {
    if (logEndRef.current) {
      logEndRef.current.scrollIntoView({behavior: "smooth"});
    }
  }, [logs]);

  const handleClose = () => {
    stopTaskPolling();
    taskIdRef.current = null;
    taskStorageKeyRef.current = null;
    submittingRef.current = false;
    form.resetFields();
    setValuesYAML("");
    setError(null);
    setLogs([]);
    setDone(false);
    setInstalling(false);
    onClose();
  };

  const handleOk = () => {
    if (done) {
      onInstalled?.();
      handleClose();
      return;
    }
    if (submittingRef.current) {return;}
    submittingRef.current = true;
    form.validateFields().then(values => {
      setInstalling(true);
      setError(null);
      setLogs([]);

      HelmBackend.installHelmChartStream(
        {
          releaseName: values.releaseName,
          namespace: values.namespace,
          chartName: chart.chartName,
          repoURL: chart.repoURL,
          version: values.version || chart.version,
          valuesYAML,
        },
        line => {
          if (!mountedRef.current) {return;}
          if (line.startsWith("TASK_ID:")) {
            const taskId = line.slice("TASK_ID:".length).trim();
            const storageKey = helmTaskStorageKey(chart.chartName, values.namespace, values.releaseName);
            taskIdRef.current = taskId;
            taskStorageKeyRef.current = storageKey;
            try {
              window.localStorage.setItem(storageKey, JSON.stringify({
                taskId,
                createdAt: Date.now(),
                namespace: values.namespace,
                releaseName: values.releaseName,
              }));
            } catch (_) {
              // Continue the current install even when persistence is unavailable.
            }
          } else {
            setLogs(prev => [...prev, line]);
          }
        }
      )
        .then(status => {
          if (!mountedRef.current) {return;}
          if (status === "DONE") {
            setDone(true);
            setInstalling(false);
            submittingRef.current = false;
            removeStoredHelmTask(taskStorageKeyRef.current);
          }
        })
        .catch(e => {
          if (!mountedRef.current) {return;}
          if (taskIdRef.current) {
            monitorTask(taskIdRef.current);
            return;
          }
          setError(e.message);
          setInstalling(false);
          submittingRef.current = false;
        });
    }).catch(() => {
      submittingRef.current = false;
    });
  };

  if (!chart) {return null;}

  const nsOptions = namespaces.map(ns => ({label: ns.name, value: ns.name}));
  const showLog = installing || done || (error && logs.length > 0);

  const lineColor = (line, i, total) => {
    if (line.startsWith("ERROR")) {return "#f87171";}
    if (done && i === total - 1) {return "#4ade80";}
    return "#d4d4d4";
  };

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
      onCancel={installing ? undefined : handleClose}
      closable={!installing}
      maskClosable={!installing}
      footer={
        <div style={{display: "flex", justifyContent: "flex-end", gap: 8}}>
          {!installing && (
            <Button onClick={handleClose}>
              {done ? t("general:Close") : t("general:Cancel")}
            </Button>
          )}
          {!done && (
            <Button type="primary" loading={installing} onClick={handleOk}>
              {t("helm:Install")}
            </Button>
          )}
          {done && (
            <Button type="primary" onClick={handleOk}>
              {t("general:Done")}
            </Button>
          )}
        </div>
      }
      width={700}
      destroyOnHidden
    >
      {error && (
        <Alert type="error" message={error} showIcon style={{marginBottom: 16}} closable onClose={() => setError(null)} />
      )}

      {!showLog && (
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
                <Spin size="small" />
                <Text style={{marginLeft: 8, color: "rgba(0,0,0,0.45)"}}>{t("helm:Loading values")}</Text>
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
      )}

      {showLog && (
        <div
          style={{
            background: "#1a1a1a", borderRadius: 6, padding: "10px 14px",
            fontFamily: "monospace", fontSize: 12, color: "#d4d4d4",
            height: 340, overflowY: "auto", lineHeight: 1.6,
          }}
        >
          {logs.length === 0 && installing && (
            <span style={{color: "#888"}}>
              <Spin size="small" style={{marginRight: 8}} />
              {t("helm:Installing")}...
            </span>
          )}
          {logs.map((line, i) => (
            <div key={i} style={{color: lineColor(line, i, logs.length)}}>
              {line}
            </div>
          ))}
          {installing && logs.length > 0 && (
            <span style={{color: "#888", display: "inline-flex", alignItems: "center", gap: 6, marginTop: 4}}>
              <Spin size="small" />
            </span>
          )}
          <div ref={logEndRef} />
        </div>
      )}
    </Modal>
  );
}
