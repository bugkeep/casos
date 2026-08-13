import React, {useEffect, useRef, useState} from "react";
import {Alert, Button, Form, Input, Modal, Select, Spin, Typography} from "antd";
import {useTranslation} from "react-i18next";
import * as HelmBackend from "./backend/HelmBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import {
  findStoredHelmTask,
  helmTaskMatchesIdentity,
  helmTaskPollRetryDelay,
  helmTaskStorageKey,
  helmTaskStorageSchemaVersion,
  removeStoredHelmTask
} from "./helmTaskStorage";
import {resolveHelmCompatibilityError} from "./helmCompatibilityErrors";
import HelmCompatibilityErrorAlert from "./HelmCompatibilityErrorAlert";

const {Text} = Typography;
const helmOperationTaskNotFoundCode = "helm_task_not_found";
const helmInstallStreamIdleTimeout = 30 * 1000;
const helmValuesReloadDebounce = 500;

export default function HelmInstallModal({open, chart, action = "install", onClose, onInstalled}) {
  const {t} = useTranslation();
  const isUpgrade = action === "upgrade";
  const [form] = Form.useForm();
  const watchedRepoURL = Form.useWatch("repoURL", form);
  const watchedVersion = Form.useWatch("version", form);
  const [namespaces, setNamespaces] = useState([]);
  const [valuesYAML, setValuesYAML] = useState("");
  const [valuesBaselineYAML, setValuesBaselineYAML] = useState("");
  const [valuesLoading, setValuesLoading] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [pollingPaused, setPollingPaused] = useState(false);
  const [activeTaskId, setActiveTaskId] = useState(null);
  const [done, setDone] = useState(false);
  const [error, setError] = useState(null);
  const [valuesLoadError, setValuesLoadError] = useState(null);
  const [storageWarning, setStorageWarning] = useState(null);
  const [logs, setLogs] = useState([]);
  const logEndRef = useRef(null);
  const taskIdRef = useRef(null);
  const taskStorageKeyRef = useRef(null);
  const taskIdentityRef = useRef(null);
  const pollTimerRef = useRef(null);
  const pollGenerationRef = useRef(0);
  const initializationGenerationRef = useRef(0);
  const valuesLoadGenerationRef = useRef(0);
  const streamAbortRef = useRef(null);
  const streamIdleTimerRef = useRef(null);
  const streamIdleControllerRef = useRef(null);
  const mountedRef = useRef(true);
  const submittingRef = useRef(false);

  const stopTaskPolling = () => {
    pollGenerationRef.current += 1;
    if (pollTimerRef.current) {
      clearTimeout(pollTimerRef.current);
      pollTimerRef.current = null;
    }
  };

  const stopStreamIdleTimer = (controller = null) => {
    if (controller && streamIdleControllerRef.current !== controller) {return;}
    if (streamIdleTimerRef.current) {
      clearTimeout(streamIdleTimerRef.current);
      streamIdleTimerRef.current = null;
    }
    streamIdleControllerRef.current = null;
  };

  const forgetTask = (storageKey = taskStorageKeyRef.current) => {
    removeStoredHelmTask(storageKey);
    if (!storageKey || taskStorageKeyRef.current === storageKey) {
      taskIdRef.current = null;
      setActiveTaskId(null);
      taskStorageKeyRef.current = null;
      taskIdentityRef.current = null;
    }
  };

  const monitorTask = (
    taskId,
    storageKey = taskStorageKeyRef.current,
    expectedIdentity = taskIdentityRef.current
  ) => {
    if (!taskId) {
      setInstalling(false);
      setPollingPaused(false);
      submittingRef.current = false;
      return;
    }
    stopTaskPolling();
    setPollingPaused(false);
    const generation = pollGenerationRef.current;
    let consecutiveFailures = 0;
    const poll = () => {
      HelmBackend.getHelmOperationTask(taskId)
        .then(res => {
          if (!mountedRef.current || generation !== pollGenerationRef.current) {return;}
          if (res.status !== "ok") {
            if (res.data === helmOperationTaskNotFoundCode) {
              forgetTask(storageKey);
              submittingRef.current = false;
            } else {
              setPollingPaused(true);
              submittingRef.current = true;
            }
            setError(res.msg);
            setInstalling(false);
            return;
          }
          consecutiveFailures = 0;
          setError(null);
          const task = res.data;
          if (!task || !task.id || !task.status) {
            setError(t("helm:Unable to refresh Helm operation status: invalid response"));
            setInstalling(false);
            setPollingPaused(true);
            submittingRef.current = true;
            return;
          }
          const matchesExpectedTask = helmTaskMatchesIdentity(task, taskId, expectedIdentity);
          if (!matchesExpectedTask) {
            forgetTask(storageKey);
            setError(t("helm:The saved Helm operation no longer matches this chart and was discarded"));
            setInstalling(false);
            setPollingPaused(false);
            submittingRef.current = false;
            return;
          }
          const taskLogs = Array.isArray(res.data2) ? res.data2 : [];
          setLogs(taskLogs
            .map(log => typeof log?.message === "string" ? log.message : "")
            .filter(Boolean));
          if (task.status === "succeeded") {
            setDone(true);
            setInstalling(false);
            setPollingPaused(false);
            submittingRef.current = false;
            forgetTask(storageKey);
            return;
          }
          if (task.status === "failed") {
            setError(task.errorMsg || t("helm:Helm operation failed"));
            setInstalling(false);
            setPollingPaused(false);
            submittingRef.current = false;
            forgetTask(storageKey);
            return;
          }
          setInstalling(true);
          pollTimerRef.current = setTimeout(poll, 2000);
        })
        .catch(e => {
          if (!mountedRef.current || generation !== pollGenerationRef.current) {return;}
          consecutiveFailures += 1;
          setError(t("helm:Unable to refresh Helm operation status", {error: e.message}));
          if (consecutiveFailures >= 6) {
            setInstalling(false);
            setPollingPaused(true);
            submittingRef.current = true;
            return;
          }
          setInstalling(true);
          const retryDelay = helmTaskPollRetryDelay(consecutiveFailures);
          pollTimerRef.current = setTimeout(poll, retryDelay);
        });
    };
    poll();
  };

  useEffect(() => {
    const generation = initializationGenerationRef.current + 1;
    initializationGenerationRef.current = generation;
    if (!open || !chart) {return;}
    const isCurrentInitialization = () => (
      mountedRef.current && generation === initializationGenerationRef.current
    );
    setError(null);
    setValuesLoadError(null);
    setStorageWarning(null);
    setLogs([]);
    setValuesYAML("");
    setValuesBaselineYAML("");
    setValuesLoading(false);
    setDone(false);
    setInstalling(false);
    setPollingPaused(false);
    taskIdRef.current = null;
    setActiveTaskId(null);
    taskStorageKeyRef.current = null;
    taskIdentityRef.current = null;
    submittingRef.current = false;
    stopTaskPolling();
    stopStreamIdleTimer();
    streamAbortRef.current?.abort();
    streamAbortRef.current = null;

    const savedTask = findStoredHelmTask(chart.chartName, isUpgrade ? {
      operation: action,
      namespace: chart.namespace,
      releaseName: chart.releaseName,
    } : null);
    const initialFields = {
      releaseName: isUpgrade ? chart.releaseName : (savedTask?.releaseName || chart.chartName),
      namespace: isUpgrade ? chart.namespace : savedTask?.namespace,
      repoURL: chart.repoURL,
      version: chart.version ?? "",
    };
    form.setFieldsValue(initialFields);
    if (savedTask) {
      taskIdRef.current = savedTask.taskId;
      setActiveTaskId(savedTask.taskId);
      taskStorageKeyRef.current = savedTask.key;
      taskIdentityRef.current = savedTask;
      submittingRef.current = true;
      setInstalling(true);
      monitorTask(savedTask.taskId, savedTask.key, savedTask);
    }

    NamespaceBackend.getNamespaces().then(res => {
      if (!isCurrentInitialization()) {return;}
      if (res.status === "ok") {
        const ns = res.data ?? [];
        setNamespaces(ns);
        const def = ns.find(n => n.name === "default") ? "default" : (ns[0]?.name ?? "default");
        if (!form.isFieldTouched("namespace") && !form.getFieldValue("namespace")) {
          form.setFieldsValue({namespace: def});
        }
      }
    });

  }, [open, chart, form, isUpgrade]);

  useEffect(() => {
    const repoURL = form.getFieldValue("repoURL");
    const effectiveRepoURL = repoURL === undefined ? chart?.repoURL : repoURL;
    const version = form.getFieldValue("version");
    const effectiveVersion = version || chart?.version || "";
    const generation = valuesLoadGenerationRef.current + 1;
    valuesLoadGenerationRef.current = generation;
    if (!open || !chart?.chartName || !effectiveRepoURL) {
      setValuesLoading(false);
      return undefined;
    }

    const controller = new AbortController();
    const repoChanged = isUpgrade &&
      watchedRepoURL !== undefined &&
      watchedRepoURL !== (chart?.repoURL ?? "");
    const versionChanged = watchedVersion !== undefined &&
      watchedVersion !== (chart?.version ?? "");
    const shouldDebounce = repoChanged || versionChanged;
    setValuesLoading(true);
    const reloadTimer = setTimeout(() => {
      HelmBackend.getHelmChartValues(
        chart.chartName,
        effectiveRepoURL,
        effectiveVersion,
        controller.signal
      )
        .then(res => {
          if (!mountedRef.current || generation !== valuesLoadGenerationRef.current) {return;}
          if (res.status === "ok") {
            const initialValues = res.data ?? "";
            setValuesYAML(initialValues);
            setValuesBaselineYAML(initialValues);
            setValuesLoadError(null);
          } else {
            setValuesLoadError(res.msg);
          }
        })
        .catch(e => {
          if (
            e.name !== "AbortError" &&
            mountedRef.current &&
            generation === valuesLoadGenerationRef.current
          ) {
            setValuesLoadError(e.message);
          }
        })
        .finally(() => {
          if (mountedRef.current && generation === valuesLoadGenerationRef.current) {
            setValuesLoading(false);
          }
        });
    }, shouldDebounce ? helmValuesReloadDebounce : 0);
    return () => {
      clearTimeout(reloadTimer);
      controller.abort();
    };
  }, [open, chart?.chartName, chart?.repoURL, chart?.version, form, isUpgrade, watchedRepoURL, watchedVersion]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      stopTaskPolling();
      stopStreamIdleTimer();
      streamAbortRef.current?.abort();
      streamAbortRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (logEndRef.current) {
      logEndRef.current.scrollIntoView({behavior: "smooth"});
    }
  }, [logs]);

  const handleClose = () => {
    stopTaskPolling();
    stopStreamIdleTimer();
    streamAbortRef.current?.abort();
    streamAbortRef.current = null;
    taskIdRef.current = null;
    setActiveTaskId(null);
    taskStorageKeyRef.current = null;
    taskIdentityRef.current = null;
    submittingRef.current = false;
    form.resetFields();
    setValuesYAML("");
    setValuesBaselineYAML("");
    setError(null);
    setValuesLoadError(null);
    setStorageWarning(null);
    setLogs([]);
    setDone(false);
    setInstalling(false);
    setPollingPaused(false);
    onClose();
  };

  const handleOk = () => {
    if (done) {
      onInstalled?.();
      handleClose();
      return;
    }
    if (submittingRef.current || valuesLoading || valuesLoadError) {return;}
    stopTaskPolling();
    submittingRef.current = true;
    form.validateFields().then(values => {
      setInstalling(true);
      setPollingPaused(false);
      setError(null);
      setLogs([]);
      const streamController = new AbortController();
      stopStreamIdleTimer();
      streamAbortRef.current?.abort();
      streamAbortRef.current = streamController;
      const fallBackToTaskPolling = () => {
        stopStreamIdleTimer(streamController);
        if (!mountedRef.current || streamAbortRef.current !== streamController || !taskIdRef.current) {return;}
        streamController.abort();
        monitorTask(taskIdRef.current, taskStorageKeyRef.current, taskIdentityRef.current);
      };
      const resetStreamIdleTimer = () => {
        if (!taskIdRef.current) {return;}
        stopStreamIdleTimer();
        streamIdleControllerRef.current = streamController;
        streamIdleTimerRef.current = setTimeout(fallBackToTaskPolling, helmInstallStreamIdleTimeout);
      };

      const releaseName = isUpgrade ? chart.releaseName : values.releaseName;
      const namespace = isUpgrade ? chart.namespace : values.namespace;
      const payload = {
        releaseName,
        namespace,
        chartName: chart.chartName,
        repoURL: values.repoURL || chart.repoURL,
        version: values.version || chart.version,
        valuesYAML,
        valuesBaselineYAML,
      };
      const helmActionStream = isUpgrade
        ? HelmBackend.upgradeHelmChartStream
        : HelmBackend.installHelmChartStream;

      helmActionStream(
        payload,
        line => {
          if (!mountedRef.current || streamAbortRef.current !== streamController) {return;}
          if (line.startsWith("TASK_ID:")) {
            const taskId = line.slice("TASK_ID:".length).trim();
            const storageKey = helmTaskStorageKey(chart.chartName, namespace, releaseName);
            taskIdRef.current = taskId;
            setActiveTaskId(taskId);
            taskStorageKeyRef.current = storageKey;
            taskIdentityRef.current = {
              operation: action,
              chartName: chart.chartName,
              namespace,
              releaseName,
            };
            try {
              window.localStorage.setItem(storageKey, JSON.stringify({
                schemaVersion: helmTaskStorageSchemaVersion,
                taskId,
                createdAt: Date.now(),
                operation: action,
                chartName: chart.chartName,
                namespace,
                releaseName,
              }));
            } catch (_) {
              setStorageWarning(t("helm:This browser cannot save the Helm operation for later recovery"));
            }
          } else {
            setLogs(prev => [...prev, line]);
          }
          resetStreamIdleTimer();
        },
        streamController.signal
      )
        .then(status => {
          if (!mountedRef.current || streamAbortRef.current !== streamController) {return;}
          if (status === "DONE") {
            stopStreamIdleTimer(streamController);
            setDone(true);
            setInstalling(false);
            setPollingPaused(false);
            setStorageWarning(null);
            submittingRef.current = false;
            forgetTask();
          }
        })
        .catch(e => {
          if (!mountedRef.current || streamAbortRef.current !== streamController) {return;}
          stopStreamIdleTimer(streamController);
          if (streamController.signal.aborted) {return;}
          if (e.code) {
            setError(resolveHelmCompatibilityError(e, t));
            setInstalling(false);
            setPollingPaused(false);
            submittingRef.current = false;
            forgetTask();
            return;
          }
          if (taskIdRef.current) {
            monitorTask(taskIdRef.current, taskStorageKeyRef.current, taskIdentityRef.current);
            return;
          }
          setError(resolveHelmCompatibilityError(e, t));
          setInstalling(false);
          setPollingPaused(false);
          submittingRef.current = false;
        })
        .finally(() => {
          if (streamAbortRef.current === streamController) {
            stopStreamIdleTimer(streamController);
            streamAbortRef.current = null;
          }
        });
    }).catch(() => {
      submittingRef.current = false;
    });
  };

  if (!chart) {return null;}

  const nsOptions = namespaces.map(ns => ({label: ns.name, value: ns.name}));
  const showLog = installing || pollingPaused || done || (error && logs.length > 0);
  const hasActiveTask = Boolean(activeTaskId) && !done;
  let closeLabel = t("general:Cancel");
  if (hasActiveTask) {
    closeLabel = t("helm:Close and continue in background");
  } else if (done) {
    closeLabel = t("general:Close");
  }

  const lineColor = (line, i, total) => {
    if (line.startsWith("ERROR")) {return "#f87171";}
    if (done && i === total - 1) {return "#4ade80";}
    return "#d4d4d4";
  };

  return (
    <Modal
      title={
        <span>
          {t(isUpgrade ? "helm:Upgrade" : "helm:Install chart")} <Text code>{chart.chartName}</Text>
          {chart.repoURL && (
            <Text style={{marginLeft: 8, fontSize: 12, color: "rgba(0,0,0,0.45)"}}>
              {chart.repoURL}
            </Text>
          )}
        </span>
      }
      open={open}
      onCancel={handleClose}
      closable
      maskClosable={false}
      footer={
        <div style={{display: "flex", justifyContent: "flex-end", gap: 8}}>
          <Button onClick={handleClose}>
            {closeLabel}
          </Button>
          {!done && !pollingPaused && (
            <Button
              type="primary"
              loading={installing}
              disabled={valuesLoading || Boolean(valuesLoadError)}
              onClick={handleOk}
            >
              {t(isUpgrade ? "helm:Upgrade" : "helm:Install")}
            </Button>
          )}
          {pollingPaused && (
            <Button
              type="primary"
              onClick={() => monitorTask(taskIdRef.current, taskStorageKeyRef.current, taskIdentityRef.current)}
            >
              {t("helm:Retry status check")}
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
        <HelmCompatibilityErrorAlert error={error} t={t} onClose={() => setError(null)} />
      )}

      {valuesLoadError && (
        <Alert
          type="error"
          message={valuesLoadError}
          showIcon
          style={{marginBottom: 16}}
        />
      )}

      {storageWarning && (
        <Alert
          type="warning"
          message={storageWarning}
          showIcon
          style={{marginBottom: 16}}
          closable
          onClose={() => setStorageWarning(null)}
        />
      )}

      {hasActiveTask && (
        <Alert
          type="info"
          message={t("helm:Closing this window does not cancel the Helm operation")}
          showIcon
          style={{marginBottom: 16}}
        />
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
              <Input disabled={isUpgrade} />
            </Form.Item>
            <Form.Item style={{flex: 1}} label={t("general:Namespaces")} name="namespace" rules={[{required: true}]}>
              <Select options={nsOptions} showSearch disabled={isUpgrade} />
            </Form.Item>
            <Form.Item style={{width: 130}} label={t("helm:Version")} name="version">
              <Input placeholder={chart.version ?? "latest"} />
            </Form.Item>
          </div>

          {isUpgrade && (
            <Form.Item label={t("helm:Repo URL")} name="repoURL" rules={[{required: true}]}>
              <Input placeholder="https://example.com/charts" />
            </Form.Item>
          )}

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
          {logs.length === 0 && (installing || pollingPaused) && (
            <span style={{color: "#888"}}>
              {installing && <Spin size="small" style={{marginRight: 8}} />}
              {pollingPaused ? t("helm:Status check paused") : `${t(isUpgrade ? "helm:Upgrading" : "helm:Installing")}...`}
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
