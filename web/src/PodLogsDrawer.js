import React, {useEffect, useRef, useState} from "react";
import {Alert, Button, Drawer, Select, Space, Tag, Tooltip} from "antd";
import {DownloadOutlined, VerticalAlignBottomOutlined} from "@ant-design/icons";
import * as PodBackend from "./backend/PodBackend";

const TAIL_OPTIONS = [
  {label: "100 lines", value: 100},
  {label: "500 lines", value: 500},
  {label: "1000 lines", value: 1000},
  {label: "5000 lines", value: 5000},
];

const POLL_INTERVAL = 3000;

/**
 * Props:
 *   pod      {object|null}  - pod summary with {namespace, name, containers:[]}
 *   open     {boolean}
 *   onClose  {Function}
 */
function PodLogsDrawer({pod, open, onClose}) {
  const [container, setContainer] = useState("");
  const [tailLines, setTailLines] = useState(500);
  const [logs, setLogs] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const logsEndRef = useRef(null);
  const timerRef = useRef(null);
  const autoScrollRef = useRef(true);

  function stopPolling() {
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
  }

  function fetchLogs(ns, name, ctr, tail) {
    setLoading(true);
    PodBackend.getPodLogs(ns, name, ctr, tail).then(res => {
      if (res.status === "ok") {
        setLogs(res.data ?? "");
        setError(null);
        if (autoScrollRef.current) {
          setTimeout(() => logsEndRef.current?.scrollIntoView({behavior: "smooth"}), 50);
        }
      } else {
        setError(res.msg);
      }
    }).catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    if (!open || !pod) {
      stopPolling();
      setLogs("");
      setError(null);
      return;
    }
    const defaultContainer = pod.containers?.[0] ?? "";
    setContainer(defaultContainer);
    setLogs("");
    setError(null);
    fetchLogs(pod.namespace, pod.name, defaultContainer, tailLines);
    timerRef.current = setInterval(
      () => fetchLogs(pod.namespace, pod.name, defaultContainer, tailLines),
      POLL_INTERVAL
    );
    return stopPolling;
  }, [open, pod]);

  // restart polling when container or tailLines changes
  useEffect(() => {
    if (!open || !pod) {return;}
    stopPolling();
    fetchLogs(pod.namespace, pod.name, container, tailLines);
    timerRef.current = setInterval(
      () => fetchLogs(pod.namespace, pod.name, container, tailLines),
      POLL_INTERVAL
    );
    return stopPolling;
  }, [container, tailLines]);

  function handleDownload() {
    const blob = new Blob([logs], {type: "text/plain"});
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${pod.namespace}_${pod.name}_${container || "default"}.log`;
    a.click();
    URL.revokeObjectURL(url);
  }

  const containerOptions = (pod?.containers ?? []).map(c => ({label: c, value: c}));
  const multiContainer = containerOptions.length > 1;

  const drawerTitle = pod ? `Logs — ${pod.namespace} / ${pod.name}` : "Logs";

  const extra = (
    <Space>
      {multiContainer && (
        <Select
          size="small"
          value={container}
          onChange={v => {setContainer(v); autoScrollRef.current = true;}}
          options={containerOptions}
          style={{width: 160}}
          placeholder="Container"
        />
      )}
      <Select
        size="small"
        value={tailLines}
        onChange={v => {setTailLines(v); autoScrollRef.current = true;}}
        options={TAIL_OPTIONS}
        style={{width: 110}}
      />
      <Tooltip title="Download log">
        <Button size="small" icon={<DownloadOutlined />} onClick={handleDownload} disabled={!logs} />
      </Tooltip>
      <Tooltip title="Scroll to bottom">
        <Button
          size="small"
          icon={<VerticalAlignBottomOutlined />}
          onClick={() => {
            autoScrollRef.current = true;
            logsEndRef.current?.scrollIntoView({behavior: "smooth"});
          }}
        />
      </Tooltip>
      <Tag color={loading ? "processing" : "success"}>
        {loading ? "refreshing…" : "live · 3s"}
      </Tag>
    </Space>
  );

  return (
    <Drawer
      title={drawerTitle}
      open={open}
      onClose={onClose}
      width={860}
      extra={extra}
      styles={{body: {padding: "12px 16px"}}}
    >
      {error && <Alert type="error" message={error} style={{marginBottom: 12}} showIcon />}
      <div
        onScroll={e => {
          const el = e.currentTarget;
          autoScrollRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
        }}
        style={{
          background: "#0d1117",
          borderRadius: 6,
          padding: "12px 16px",
          fontFamily: "'Cascadia Code', 'Fira Mono', 'Consolas', monospace",
          fontSize: 13,
          lineHeight: 1.7,
          minHeight: 200,
          height: "calc(100vh - 160px)",
          overflowY: "auto",
          color: "#c9d1d9",
          whiteSpace: "pre-wrap",
          wordBreak: "break-all",
        }}
      >
        {!logs && !loading && <span style={{color: "#6e7681"}}>No logs yet…</span>}
        {logs}
        <div ref={logsEndRef} />
      </div>
    </Drawer>
  );
}

export default PodLogsDrawer;
