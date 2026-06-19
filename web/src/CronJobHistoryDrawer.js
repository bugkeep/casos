import React, {useCallback, useEffect, useState} from "react";
import {Badge, Button, Drawer, Modal, Space, Table, Tag, Typography} from "antd";
import {FileTextOutlined, ReloadOutlined} from "@ant-design/icons";
import * as CronJobBackend from "./backend/CronJobBackend";
import * as Setting from "./Setting";

const {Text} = Typography;

const STATUS_BADGE = {
  succeeded: "success",
  running: "processing",
  failed: "error",
  pending: "default",
};

function LogModal({namespace, podName, open, onClose}) {
  const [logs, setLogs] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!open || !podName) {return;}
    setLoading(true);
    fetch(`/api/get-pod-logs?namespace=${encodeURIComponent(namespace)}&name=${encodeURIComponent(podName)}&tailLines=500`, {
      credentials: "include",
    })
      .then(r => r.json())
      .then(res => {
        if (res.status === "ok") {setLogs(res.data ?? "");} else {setLogs(`Error: ${res.msg}`);}
      })
      .catch(e => setLogs(`Error: ${e.message}`))
      .finally(() => setLoading(false));
  }, [open, namespace, podName]);

  return (
    <Modal
      title={<span><FileTextOutlined style={{marginRight: 8}} />Logs — {podName}</span>}
      open={open}
      onCancel={onClose}
      footer={<Button onClick={onClose}>Close</Button>}
      width={860}
      destroyOnHidden
    >
      {loading
        ? <Text type="secondary">Loading logs…</Text>
        : (
          <pre style={{
            background: "#141414",
            color: "#d4d4d4",
            padding: 16,
            borderRadius: 6,
            maxHeight: 500,
            overflow: "auto",
            fontSize: 12,
            whiteSpace: "pre-wrap",
            wordBreak: "break-all",
            margin: 0,
          }}>
            {logs || "(no output)"}
          </pre>
        )
      }
    </Modal>
  );
}

function CronJobHistoryDrawer({cronJob, open, onClose}) {
  const [jobs, setJobs] = useState([]);
  const [loading, setLoading] = useState(false);
  const [triggering, setTriggering] = useState(false);
  const [logTarget, setLogTarget] = useState(null);

  const fetchJobs = useCallback(() => {
    if (!cronJob) {return;}
    setLoading(true);
    CronJobBackend.getCronJobJobs(cronJob.namespace, cronJob.name)
      .then(res => {
        if (res.status === "ok") {
          const sorted = (res.data ?? []).sort((a, b) => b.startTime.localeCompare(a.startTime));
          setJobs(sorted);
        } else {
          Setting.showMessage("error", res.msg);
        }
      })
      .catch(e => Setting.showMessage("error", e.message))
      .finally(() => setLoading(false));
  }, [cronJob]);

  useEffect(() => {
    if (open) {fetchJobs();} else {setJobs([]);}
  }, [open, fetchJobs]);

  function handleTrigger() {
    setTriggering(true);
    CronJobBackend.triggerCronJob(cronJob.namespace, cronJob.name)
      .then(res => {
        if (res.status === "ok") {
          Setting.showMessage("success", "Job triggered");
          setTimeout(fetchJobs, 800);
        } else {
          Setting.showMessage("error", res.msg);
        }
      })
      .catch(e => Setting.showMessage("error", e.message))
      .finally(() => setTriggering(false));
  }

  const columns = [
    {
      title: "Status",
      dataIndex: "status",
      key: "status",
      width: 110,
      render: (v, r) => (
        <Space size={4}>
          <Badge status={STATUS_BADGE[v] ?? "default"} />
          <span style={{textTransform: "capitalize"}}>{v}</span>
          {r.manual && <Tag color="purple" style={{fontSize: 11, lineHeight: "18px", padding: "0 5px"}}>manual</Tag>}
        </Space>
      ),
    },
    {title: "Start Time", dataIndex: "startTime", key: "startTime", width: 180},
    {title: "Duration", dataIndex: "duration", key: "duration", width: 100, render: v => v || "—"},
    {
      title: "Job Name",
      dataIndex: "name",
      key: "name",
      render: v => <Text code style={{fontSize: 12}}>{v}</Text>,
    },
    {
      title: "Actions",
      key: "actions",
      width: 100,
      render: (_, r) => (
        r.podName
          ? (
            <Button
              size="small"
              icon={<FileTextOutlined />}
              onClick={() => setLogTarget(r)}
            >
              Logs
            </Button>
          )
          : <Text type="secondary" style={{fontSize: 12}}>No pod</Text>
      ),
    },
  ];

  return (
    <>
      <Drawer
        title={
          <Space>
            <span>Execution History — <Text strong>{cronJob?.name}</Text></span>
            <Tag style={{fontFamily: "monospace"}}>{cronJob?.schedule}</Tag>
          </Space>
        }
        open={open}
        onClose={onClose}
        width={760}
        extra={
          <Space>
            <Button
              size="small"
              icon={<ReloadOutlined />}
              onClick={fetchJobs}
              loading={loading}
            >
              Refresh
            </Button>
            <Button
              size="small"
              type="primary"
              onClick={handleTrigger}
              loading={triggering}
            >
              Run Now
            </Button>
          </Space>
        }
      >
        <Table
          rowKey="name"
          columns={columns}
          dataSource={jobs}
          loading={loading}
          size="small"
          pagination={{pageSize: 20}}
          locale={{emptyText: "No execution records yet"}}
        />
      </Drawer>

      <LogModal
        namespace={cronJob?.namespace ?? ""}
        podName={logTarget?.podName}
        open={logTarget !== null}
        onClose={() => setLogTarget(null)}
      />
    </>
  );
}

export default CronJobHistoryDrawer;
