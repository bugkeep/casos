import React, {useCallback, useEffect, useState} from "react";
import {Alert, Breadcrumb, Button, Drawer, Select, Space, Spin, Table, Upload} from "antd";
import {
  ArrowUpOutlined,
  DownloadOutlined,
  FileOutlined,
  FolderOutlined,
  LinkOutlined,
  ReloadOutlined,
  UploadOutlined
} from "@ant-design/icons";
import * as PodBackend from "./backend/PodBackend";
import * as Setting from "./Setting";

function formatSize(bytes, type) {
  if (type === "dir") {return "—";}
  if (bytes < 1024) {return `${bytes} B`;}
  if (bytes < 1024 * 1024) {return `${(bytes / 1024).toFixed(1)} KB`;}
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function joinPath(...parts) {
  return ("/" + parts.join("/")).replace(/\/+/g, "/");
}

function parentPath(p) {
  const parts = p.replace(/\/+$/, "").split("/").filter(Boolean);
  parts.pop();
  return "/" + parts.join("/");
}

/**
 * Props:
 *   pod      {object|null}  pod summary with {namespace, name, containers:[]}
 *   open     {boolean}
 *   onClose  {Function}
 */
function PodFilesDrawer({pod, open, onClose}) {
  const [container, setContainer] = useState("");
  const [currentPath, setCurrentPath] = useState("/");
  const [entries, setEntries] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState(null);

  const fetchDir = useCallback((ns, podName, ctr, dirPath) => {
    setLoading(true);
    setError(null);
    PodBackend.listPodFiles(ns, podName, ctr, dirPath)
      .then(res => {
        if (res.status === "ok") {
          const sorted = (res.data ?? []).slice().sort((a, b) => {
            if (a.type === "dir" && b.type !== "dir") {return -1;}
            if (a.type !== "dir" && b.type === "dir") {return 1;}
            return a.name.localeCompare(b.name);
          });
          setEntries(sorted);
        } else {
          setError(res.msg);
        }
      })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    if (!open || !pod) {
      setEntries([]);
      setError(null);
      setCurrentPath("/");
      return;
    }
    const defaultCtr = pod.containers?.[0] ?? "";
    setContainer(defaultCtr);
    setCurrentPath("/");
    fetchDir(pod.namespace, pod.name, defaultCtr, "/");
  }, [open, pod]);

  function navigate(dirPath) {
    setCurrentPath(dirPath);
    fetchDir(pod.namespace, pod.name, container, dirPath);
  }

  function handleContainerChange(ctr) {
    setContainer(ctr);
    setCurrentPath("/");
    fetchDir(pod.namespace, pod.name, ctr, "/");
  }

  async function handleDownload(entry) {
    const filePath = joinPath(currentPath, entry.name);
    try {
      const res = await PodBackend.downloadPodFile(pod.namespace, pod.name, container, filePath);
      if (!res.ok) {
        const json = await res.json().catch(() => ({msg: res.statusText}));
        Setting.showMessage("error", json.msg || "Download failed");
        return;
      }
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = entry.name;
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      Setting.showMessage("error", e.message);
    }
  }

  async function handleUpload(file) {
    setUploading(true);
    setUploadError(null);
    try {
      const res = await PodBackend.uploadPodFile(
        pod.namespace, pod.name, container, currentPath, file
      );
      if (res.status === "ok") {
        Setting.showMessage("success", `Uploaded: ${res.data}`);
        fetchDir(pod.namespace, pod.name, container, currentPath);
      } else {
        setUploadError(res.msg);
      }
    } catch (e) {
      setUploadError(e.message);
    } finally {
      setUploading(false);
    }
    return false; // prevent antd default upload
  }

  // Build breadcrumb items from currentPath
  const pathParts = currentPath.replace(/\/+$/, "").split("/").filter(Boolean);
  const breadcrumbItems = [
    {
      title: (
        <span style={{cursor: "pointer"}} onClick={() => navigate("/")}>
          /
        </span>
      ),
    },
    ...pathParts.map((part, idx) => {
      const targetPath = "/" + pathParts.slice(0, idx + 1).join("/");
      const isLast = idx === pathParts.length - 1;
      return {
        title: isLast ? (
          <span style={{color: "#1677ff"}}>{part}</span>
        ) : (
          <span style={{cursor: "pointer"}} onClick={() => navigate(targetPath)}>
            {part}
          </span>
        ),
      };
    }),
  ];

  const columns = [
    {
      title: "Name",
      dataIndex: "name",
      key: "name",
      ellipsis: true,
      render: (name, record) => {
        const icon =
          record.type === "dir" ? (
            <FolderOutlined style={{color: "#faad14", marginRight: 8}} />
          ) : record.type === "link" ? (
            <LinkOutlined style={{color: "#52c41a", marginRight: 8}} />
          ) : (
            <FileOutlined style={{color: "#8c8c8c", marginRight: 8}} />
          );

        if (record.type === "dir") {
          return (
            <span
              style={{cursor: "pointer", color: "#1677ff"}}
              onClick={() => navigate(joinPath(currentPath, name))}
            >
              {icon}{name}
            </span>
          );
        }
        return <span>{icon}{name}</span>;
      },
    },
    {
      title: "Size",
      dataIndex: "size",
      key: "size",
      width: 90,
      render: (size, record) => (
        <span style={{color: "#888", fontSize: 12}}>{formatSize(size, record.type)}</span>
      ),
    },
    {
      title: "Modified",
      dataIndex: "modTime",
      key: "modTime",
      width: 130,
      render: v => <span style={{color: "#888", fontSize: 12}}>{v}</span>,
    },
    {
      title: "",
      key: "actions",
      width: 80,
      render: (_, record) =>
        record.type !== "dir" ? (
          <Button
            size="small"
            icon={<DownloadOutlined />}
            onClick={() => handleDownload(record)}
          >
            Download
          </Button>
        ) : null,
    },
  ];

  const containerOptions = (pod?.containers ?? []).map(c => ({label: c, value: c}));
  const multiContainer = containerOptions.length > 1;
  const drawerTitle = pod ? `Files — ${pod.namespace} / ${pod.name}` : "Files";

  return (
    <Drawer
      title={drawerTitle}
      open={open}
      onClose={onClose}
      width={700}
      destroyOnHidden
      extra={
        <Space>
          {multiContainer && (
            <Select
              size="small"
              value={container}
              onChange={handleContainerChange}
              options={containerOptions}
              style={{width: 150}}
            />
          )}
          <Upload beforeUpload={handleUpload} showUploadList={false} disabled={uploading}>
            <Button size="small" icon={<UploadOutlined />} loading={uploading}>
              Upload here
            </Button>
          </Upload>
          <Button
            size="small"
            icon={<ReloadOutlined />}
            loading={loading}
            onClick={() => fetchDir(pod.namespace, pod.name, container, currentPath)}
          />
        </Space>
      }
    >
      {/* Path bar */}
      <div style={{
        display: "flex",
        alignItems: "center",
        gap: 8,
        marginBottom: 12,
        padding: "6px 10px",
        background: "#f5f5f5",
        borderRadius: 6,
      }}>
        <Button
          size="small"
          icon={<ArrowUpOutlined />}
          disabled={currentPath === "/"}
          onClick={() => navigate(parentPath(currentPath))}
        />
        <Breadcrumb items={breadcrumbItems} style={{fontSize: 13}} />
      </div>

      {uploadError && (
        <Alert type="error" message={uploadError} closable onClose={() => setUploadError(null)}
          style={{marginBottom: 12}} showIcon />
      )}

      {error ? (
        <Alert type="error" message={error} showIcon />
      ) : (
        <Spin spinning={loading}>
          <Table
            rowKey="name"
            columns={columns}
            dataSource={entries}
            size="small"
            pagination={false}
            locale={{emptyText: loading ? " " : "Empty directory"}}
            scroll={{y: "calc(100vh - 230px)"}}
          />
        </Spin>
      )}
    </Drawer>
  );
}

export default PodFilesDrawer;
