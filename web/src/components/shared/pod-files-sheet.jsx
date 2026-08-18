import React, {useCallback, useEffect, useRef, useState} from "react";
import {ArrowUp, Download, File, FolderClosed, Link2, RefreshCw, Upload} from "lucide-react";
import * as PodBackend from "@/backend/PodBackend";
import * as Setting from "@/Setting";
import {Button} from "@/components/ui/button";
import {MessageAlert} from "@/components/ui/alert";
import {DataTable} from "@/components/shared/data-table";
import {ResourceSheet} from "@/components/shared/resource-sheet";
import {SimpleSelect} from "@/components/shared/simple-select";

function formatSize(bytes, type) {
  if (type === "dir") {
    return "—";
  }
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function joinPath(...parts) {
  return `/${parts.join("/")}`.replace(/\/+/g, "/");
}

function parentPath(path) {
  const parts = path.replace(/\/+$/, "").split("/").filter(Boolean);
  parts.pop();
  return `/${parts.join("/")}`;
}

/** Browse, download from and upload into a container's filesystem. */
export function PodFilesSheet({pod, open, onClose}) {
  const [container, setContainer] = useState("");
  const [currentPath, setCurrentPath] = useState("/");
  const [entries, setEntries] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState(null);
  const fileInputRef = useRef(null);

  const fetchDir = useCallback((namespace, podName, containerName, dirPath) => {
    setLoading(true);
    setError(null);
    PodBackend.listPodFiles(namespace, podName, containerName, dirPath)
      .then((res) => {
        if (res.status === "ok") {
          // Directories first, then alphabetical — the order a file browser is
          // expected to have, which the API does not guarantee.
          const sorted = [...(res.data ?? [])].sort((a, b) => {
            if (a.type === "dir" && b.type !== "dir") {
              return -1;
            }
            if (a.type !== "dir" && b.type === "dir") {
              return 1;
            }
            return a.name.localeCompare(b.name);
          });
          setEntries(sorted);
        } else {
          setError(res.msg);
        }
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    if (!open || !pod) {
      setEntries([]);
      setError(null);
      setCurrentPath("/");
      return;
    }
    const defaultContainer = pod.containers?.[0] ?? "";
    setContainer(defaultContainer);
    setCurrentPath("/");
    fetchDir(pod.namespace, pod.name, defaultContainer, "/");
  }, [open, pod, fetchDir]);

  function navigate(dirPath) {
    setCurrentPath(dirPath);
    fetchDir(pod.namespace, pod.name, container, dirPath);
  }

  function handleContainerChange(next) {
    setContainer(next);
    setCurrentPath("/");
    fetchDir(pod.namespace, pod.name, next, "/");
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
      const link = document.createElement("a");
      link.href = url;
      link.download = entry.name;
      link.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      Setting.showMessage("error", e.message);
    }
  }

  async function handleUpload(file) {
    if (!file) {
      return;
    }
    setUploading(true);
    setUploadError(null);
    try {
      const res = await PodBackend.uploadPodFile(pod.namespace, pod.name, container, currentPath, file);
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
  }

  const pathParts = currentPath.replace(/\/+$/, "").split("/").filter(Boolean);
  const containerOptions = (pod?.containers ?? []).map((name) => ({label: name, value: name}));

  const columns = [
    {
      key: "name",
      title: "Name",
      dataIndex: "name",
      ellipsis: true,
      render: (name, record) => {
        const Icon = record.type === "dir" ? FolderClosed : record.type === "link" ? Link2 : File;
        const iconClass = record.type === "dir" ? "text-warning" : record.type === "link" ? "text-success" : "text-muted-foreground";

        if (record.type === "dir") {
          return (
            <button
              type="button"
              onClick={() => navigate(joinPath(currentPath, name))}
              className="text-info flex items-center gap-2 hover:underline"
            >
              <Icon className={`size-4 shrink-0 ${iconClass}`} />
              {name}
            </button>
          );
        }
        return (
          <span className="flex items-center gap-2">
            <Icon className={`size-4 shrink-0 ${iconClass}`} />
            {name}
          </span>
        );
      },
    },
    {
      key: "size",
      title: "Size",
      dataIndex: "size",
      width: 100,
      align: "right",
      render: (size, record) => <span className="text-muted-foreground text-xs">{formatSize(size, record.type)}</span>,
    },
    {
      key: "modTime",
      title: "Modified",
      dataIndex: "modTime",
      width: 150,
      render: (value) => <span className="text-muted-foreground text-xs">{value}</span>,
    },
    {
      key: "actions",
      title: "",
      width: 130,
      align: "right",
      render: (_, record) =>
        record.type !== "dir" ? (
          <Button variant="outline" size="sm" onClick={() => handleDownload(record)}>
            <Download />
            Download
          </Button>
        ) : null,
    },
  ];

  return (
    <ResourceSheet
      open={open}
      onOpenChange={(next) => (next ? null : onClose())}
      title={pod ? `Files — ${pod.namespace} / ${pod.name}` : "Files"}
      size="lg"
      toolbar={
        <>
          {containerOptions.length > 1 ? (
            <SimpleSelect
              value={container}
              onChange={handleContainerChange}
              options={containerOptions}
              size="sm"
              className="w-40"
            />
          ) : null}
          <input
            ref={fileInputRef}
            type="file"
            className="hidden"
            onChange={(event) => {
              handleUpload(event.target.files?.[0]);
              event.target.value = "";
            }}
          />
          <Button variant="outline" size="sm" loading={uploading} onClick={() => fileInputRef.current?.click()}>
            <Upload />
            Upload here
          </Button>
          <Button
            variant="outline"
            size="icon-sm"
            loading={loading}
            onClick={() => fetchDir(pod.namespace, pod.name, container, currentPath)}
            aria-label="Refresh"
          >
            {loading ? null : <RefreshCw className="size-4" />}
          </Button>
        </>
      }
    >
      <div className="bg-muted mb-3 flex items-center gap-2 rounded-md px-2 py-1.5">
        <Button
          variant="outline"
          size="icon-xs"
          disabled={currentPath === "/"}
          onClick={() => navigate(parentPath(currentPath))}
          aria-label="Parent directory"
        >
          <ArrowUp className="size-3.5" />
        </Button>
        <nav className="flex min-w-0 flex-wrap items-center gap-1 text-xs">
          <button type="button" onClick={() => navigate("/")} className="hover:text-foreground text-muted-foreground">
            /
          </button>
          {pathParts.map((part, index) => {
            const isLast = index === pathParts.length - 1;
            const target = `/${pathParts.slice(0, index + 1).join("/")}`;
            return (
              <span key={target} className="flex items-center gap-1">
                {index > 0 ? <span className="text-muted-foreground">/</span> : null}
                {isLast ? (
                  <span className="text-info">{part}</span>
                ) : (
                  <button type="button" onClick={() => navigate(target)} className="hover:text-foreground text-muted-foreground">
                    {part}
                  </button>
                )}
              </span>
            );
          })}
        </nav>
      </div>

      {uploadError ? <MessageAlert title={uploadError} className="mb-3" /> : null}

      {error ? (
        <MessageAlert title={error} />
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          <DataTable
            columns={columns}
            dataSource={entries}
            rowKey="name"
            loading={loading}
            pageSize={0}
            dense
            emptyText="Empty directory"
          />
        </div>
      )}
    </ResourceSheet>
  );
}

export default PodFilesSheet;
