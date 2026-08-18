import React, {useCallback, useEffect, useState} from "react";
import {FileText, Play, RefreshCw} from "lucide-react";
import * as CronJobBackend from "@/backend/CronJobBackend";
import * as Setting from "@/Setting";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle} from "@/components/ui/dialog";
import {DataTable} from "@/components/shared/data-table";
import {ResourceSheet} from "@/components/shared/resource-sheet";
import {CodeText} from "@/components/shared/misc";

const STATUS_VARIANTS = {
  succeeded: "success",
  running: "info",
  failed: "danger",
  pending: "muted",
};

function LogDialog({namespace, podName, open, onClose}) {
  const [logs, setLogs] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!open || !podName) {
      return;
    }
    let cancelled = false;
    setLoading(true);
    fetch(
      `${Setting.ServerUrl}/api/get-pod-logs?namespace=${encodeURIComponent(namespace)}&name=${encodeURIComponent(
        podName
      )}&tailLines=500`,
      {credentials: "include"}
    )
      .then((response) => response.json())
      .then((res) => {
        if (cancelled) {
          return;
        }
        setLogs(res.status === "ok" ? res.data ?? "" : `Error: ${res.msg}`);
      })
      .catch((error) => {
        if (!cancelled) {
          setLogs(`Error: ${error.message}`);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [open, namespace, podName]);

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? null : onClose())}>
      <DialogContent className="sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FileText className="size-4" />
            Logs — {podName}
          </DialogTitle>
        </DialogHeader>

        {loading ? (
          <p className="text-muted-foreground text-sm">Loading logs…</p>
        ) : (
          <pre className="scrollbar-thin max-h-[60vh] overflow-auto rounded-lg bg-neutral-950 p-4 font-mono text-xs break-all whitespace-pre-wrap text-neutral-200">
            {logs || "(no output)"}
          </pre>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * The runs a CronJob has produced, newest first, with a manual trigger. The
 * backend returns them unordered, so the sort here is what makes "what happened
 * last night" the first thing on screen.
 */
export function CronJobHistorySheet({cronJob, open, onClose}) {
  const [jobs, setJobs] = useState([]);
  const [loading, setLoading] = useState(false);
  const [triggering, setTriggering] = useState(false);
  const [logTarget, setLogTarget] = useState(null);

  const fetchJobs = useCallback(() => {
    if (!cronJob) {
      return;
    }
    setLoading(true);
    CronJobBackend.getCronJobJobs(cronJob.namespace, cronJob.name)
      .then((res) => {
        if (res.status === "ok") {
          setJobs([...(res.data ?? [])].sort((a, b) => String(b.startTime).localeCompare(String(a.startTime))));
        } else {
          Setting.showMessage("error", res.msg);
        }
      })
      .catch((error) => Setting.showMessage("error", error.message))
      .finally(() => setLoading(false));
  }, [cronJob]);

  useEffect(() => {
    if (open) {
      fetchJobs();
    } else {
      setJobs([]);
    }
  }, [open, fetchJobs]);

  function handleTrigger() {
    setTriggering(true);
    CronJobBackend.triggerCronJob(cronJob.namespace, cronJob.name)
      .then((res) => {
        if (res.status === "ok") {
          Setting.showMessage("success", "Job triggered");
          // The Job object does not exist the instant the request returns; a
          // short delay avoids showing an empty list right after triggering.
          setTimeout(fetchJobs, 800);
        } else {
          Setting.showMessage("error", res.msg);
        }
      })
      .catch((error) => Setting.showMessage("error", error.message))
      .finally(() => setTriggering(false));
  }

  const columns = [
    {
      key: "status",
      title: "Status",
      dataIndex: "status",
      width: 140,
      render: (value, record) => (
        <span className="flex items-center gap-1.5">
          <Badge variant={STATUS_VARIANTS[value] ?? "muted"} className="capitalize">
            {value}
          </Badge>
          {record.manual ? <Badge variant="secondary">manual</Badge> : null}
        </span>
      ),
    },
    {key: "startTime", title: "Start Time", dataIndex: "startTime", width: 190, sortable: true},
    {key: "duration", title: "Duration", dataIndex: "duration", width: 110, render: (value) => value || "—"},
    {key: "name", title: "Job Name", dataIndex: "name", render: (value) => <CodeText>{value}</CodeText>},
    {
      key: "actions",
      title: "Actions",
      width: 110,
      align: "right",
      render: (_, record) =>
        record.podName ? (
          <Button variant="outline" size="sm" onClick={() => setLogTarget(record)}>
            <FileText />
            Logs
          </Button>
        ) : (
          <span className="text-muted-foreground text-xs">No pod</span>
        ),
    },
  ];

  return (
    <>
      <ResourceSheet
        open={open}
        onOpenChange={(next) => (next ? null : onClose())}
        title={`Execution History — ${cronJob?.name ?? ""}`}
        description={cronJob?.schedule}
        size="lg"
        toolbar={
          <>
            <Button variant="outline" size="sm" onClick={fetchJobs} loading={loading}>
              <RefreshCw />
              Refresh
            </Button>
            <Button size="sm" onClick={handleTrigger} loading={triggering}>
              <Play />
              Run Now
            </Button>
          </>
        }
      >
        <DataTable
          columns={columns}
          dataSource={jobs}
          rowKey="name"
          loading={loading}
          emptyText="No execution records yet"
          dense
        />
      </ResourceSheet>

      <LogDialog
        namespace={cronJob?.namespace ?? ""}
        podName={logTarget?.podName}
        open={logTarget !== null}
        onClose={() => setLogTarget(null)}
      />
    </>
  );
}

export default CronJobHistorySheet;
