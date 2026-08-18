import React, {useCallback, useEffect, useMemo, useState} from "react";
import {useTranslation} from "react-i18next";
import i18next from "i18next";
import {
  Bell,
  Boxes,
  CircleAlert,
  CircleCheck,
  CircleHelp,
  CircleX,
  Clock,
  RefreshCw,
  Server,
  TriangleAlert,
} from "lucide-react";
import * as MonitorBackend from "@/backend/MonitorBackend";
import * as Setting from "@/Setting";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle} from "@/components/ui/dialog";
import {DataTable} from "@/components/shared/data-table";
import {DescriptionList} from "@/components/shared/misc";
import {Loading} from "@/components/shared/loading";
import {PageContainer} from "@/components/shared/page-header";
import {ResourceSheet} from "@/components/shared/resource-sheet";
import {StatCard} from "@/components/shared/stat-card";

const STATUS_META = {
  healthy: {variant: "success", tone: "success", Icon: CircleCheck},
  warning: {variant: "warning", tone: "warning", Icon: CircleAlert},
  critical: {variant: "danger", tone: "danger", Icon: CircleX},
  unknown: {variant: "muted", tone: "default", Icon: CircleHelp},
};

const SEVERITY_VARIANTS = {info: "info", warning: "warning", critical: "danger"};
const EVENT_TYPE_VARIANTS = {Normal: "info", Warning: "warning"};

// The i18n extractor only sees literal i18next.t(...) calls, so the keys this
// page builds dynamically (status/severity suffixes) are registered here or they
// never make it into the locale files.
function registerMonitorI18nKeys() {
  i18next.t("monitor:Abnormal Pods");
  i18next.t("monitor:Category");
  i18next.t("monitor:Check");
  i18next.t("monitor:Count");
  i18next.t("monitor:Critical Checks");
  i18next.t("monitor:Current");
  i18next.t("monitor:Details");
  i18next.t("monitor:Diagnosis");
  i18next.t("monitor:Diagnosis Context");
  i18next.t("monitor:Event Center");
  i18next.t("monitor:Event Details");
  i18next.t("monitor:Failed to load diagnosis");
  i18next.t("monitor:Failed to load events");
  i18next.t("monitor:Failed to load health checks");
  i18next.t("monitor:Failed to load monitor issues");
  i18next.t("monitor:Failed to load monitor data");
  i18next.t("monitor:Failed to load monitor summary");
  i18next.t("monitor:Health Checks");
  i18next.t("monitor:Last Seen");
  i18next.t("monitor:Log Preview");
  i18next.t("monitor:Last Checked");
  i18next.t("monitor:Message");
  i18next.t("monitor:Monitor Issues");
  i18next.t("monitor:Object");
  i18next.t("monitor:Overall Status");
  i18next.t("monitor:Previous");
  i18next.t("monitor:Ready Nodes");
  i18next.t("monitor:Reason");
  i18next.t("monitor:Related Events");
  i18next.t("monitor:Running Pods");
  i18next.t("monitor:Source");
  i18next.t("monitor:Suggestion");
  i18next.t("monitor:Time");
  i18next.t("monitor:Warning Checks");
  i18next.t("monitor:Warning Events");
  i18next.t("monitor:severity critical");
  i18next.t("monitor:severity info");
  i18next.t("monitor:severity warning");
  i18next.t("monitor:status critical");
  i18next.t("monitor:status healthy");
  i18next.t("monitor:status unknown");
  i18next.t("monitor:status warning");
}

registerMonitorI18nKeys();

function formatTime(value) {
  if (!value) {
    return "-";
  }
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString();
}

// Kubernetes reports an event's time in one of three fields depending on which
// API version emitted it.
function eventDisplayTime(event) {
  return event.lastTimestamp || event.eventTime || event.firstTimestamp;
}

function objectLabel(record) {
  if (!record) {
    return "-";
  }
  const name = record.namespace ? `${record.namespace}/${record.name}` : record.name;
  return `${record.kind || "-"} / ${name || "-"}`;
}

function MonitorPage() {
  const {t} = useTranslation();

  const [summary, setSummary] = useState(null);
  const [checks, setChecks] = useState([]);
  const [issues, setIssues] = useState([]);
  const [events, setEvents] = useState([]);

  const [loading, setLoading] = useState(true);
  const [issuesLoading, setIssuesLoading] = useState(true);
  const [eventsLoading, setEventsLoading] = useState(true);
  const [error, setError] = useState(null);
  const [issuesError, setIssuesError] = useState(null);
  const [eventsError, setEventsError] = useState(null);

  const [namespaceFilter, setNamespaceFilter] = useState("");
  const [selectedEvent, setSelectedEvent] = useState(null);
  const [selectedIssue, setSelectedIssue] = useState(null);
  const [diagnosis, setDiagnosis] = useState(null);
  const [diagnosisLoading, setDiagnosisLoading] = useState(false);
  const [diagnosisError, setDiagnosisError] = useState(null);

  const fetchOverview = useCallback(() => {
    setLoading(true);
    setError(null);
    MonitorBackend.getMonitorOverview()
      .then((res) => {
        if (res.status === "ok") {
          setSummary(res.data?.summary || null);
          setChecks(res.data?.checks || []);
        } else {
          setError(res.msg || t("monitor:Failed to load monitor data"));
        }
      })
      .catch((err) => {
        setError(err.message);
        Setting.showMessage("error", err.message);
      })
      .finally(() => setLoading(false));
  }, [t]);

  const fetchIssues = useCallback(() => {
    setIssuesLoading(true);
    setIssuesError(null);
    MonitorBackend.getMonitorIssues()
      .then((res) => {
        if (res.status === "ok") {
          setIssues(res.data || []);
        } else {
          setIssuesError(res.msg || t("monitor:Failed to load monitor issues"));
        }
      })
      .catch((err) => setIssuesError(err.message))
      .finally(() => setIssuesLoading(false));
  }, [t]);

  const fetchEvents = useCallback((namespace) => {
    setEventsLoading(true);
    setEventsError(null);
    MonitorBackend.getMonitorEvents(namespace, 100)
      .then((res) => {
        if (res.status === "ok") {
          setEvents(res.data || []);
        } else {
          setEventsError(res.msg);
        }
      })
      .catch((err) => setEventsError(err.message))
      .finally(() => setEventsLoading(false));
  }, []);

  const openDiagnosis = useCallback(
    (issue) => {
      setSelectedIssue(issue);
      setDiagnosis(null);
      setDiagnosisError(null);
      setDiagnosisLoading(true);
      MonitorBackend.getMonitorDiagnosis(issue, 100, true)
        .then((res) => {
          if (res.status === "ok") {
            setDiagnosis(res.data || null);
          } else {
            setDiagnosisError(res.msg || t("monitor:Failed to load diagnosis"));
          }
        })
        .catch((err) => setDiagnosisError(err.message))
        .finally(() => setDiagnosisLoading(false));
    },
    [t]
  );

  useEffect(() => {
    fetchOverview();
    fetchIssues();
    fetchEvents("");
  }, [fetchOverview, fetchIssues, fetchEvents]);

  const checkColumns = useMemo(
    () => [
      {key: "name", title: t("monitor:Check"), dataIndex: "name", width: 280, ellipsis: true, sortable: true},
      {
        key: "category",
        title: t("monitor:Category"),
        dataIndex: "category",
        width: 150,
        sortable: true,
        render: (value) => <Badge variant="muted">{value}</Badge>,
      },
      {
        key: "status",
        title: t("general:Status"),
        dataIndex: "status",
        width: 140,
        sortable: true,
        render: (value) => {
          const meta = STATUS_META[value] || STATUS_META.unknown;
          return (
            <Badge variant={meta.variant}>
              <meta.Icon />
              {t(`monitor:status ${value || "unknown"}`)}
            </Badge>
          );
        },
      },
      {
        key: "severity",
        title: t("trivy:Severity"),
        dataIndex: "severity",
        width: 140,
        sortable: true,
        render: (value) => (
          <Badge variant={SEVERITY_VARIANTS[value] ?? "muted"}>{t(`monitor:severity ${value || "info"}`)}</Badge>
        ),
      },
      {key: "message", title: t("monitor:Message"), dataIndex: "message", width: 340, ellipsis: true},
      {key: "suggestion", title: t("monitor:Suggestion"), dataIndex: "suggestion", width: 360, ellipsis: true},
      {
        key: "lastCheckedAt",
        title: t("monitor:Last Checked"),
        dataIndex: "lastCheckedAt",
        width: 200,
        sortable: true,
        render: formatTime,
      },
    ],
    [t]
  );

  const issueColumns = useMemo(
    () => [
      {
        key: "severity",
        title: t("trivy:Severity"),
        dataIndex: "severity",
        width: 130,
        sortable: true,
        render: (value) => (
          <Badge variant={SEVERITY_VARIANTS[value] ?? "muted"}>{t(`monitor:severity ${value || "info"}`)}</Badge>
        ),
      },
      {key: "object", title: t("monitor:Object"), width: 280, ellipsis: true, render: (_, record) => objectLabel(record)},
      {key: "reason", title: t("monitor:Reason"), dataIndex: "reason", width: 190, sortable: true},
      {key: "message", title: t("monitor:Message"), dataIndex: "message", width: 360, ellipsis: true},
      {key: "suggestion", title: t("monitor:Suggestion"), dataIndex: "suggestion", width: 360, ellipsis: true},
      {
        key: "lastSeenAt",
        title: t("monitor:Last Seen"),
        dataIndex: "lastSeenAt",
        width: 200,
        sortable: true,
        render: formatTime,
      },
      {
        key: "action",
        title: t("general:Action"),
        width: 130,
        align: "right",
        render: (_, record) => (
          <Button variant="outline" size="sm" onClick={() => openDiagnosis(record)}>
            {t("monitor:Diagnosis")}
          </Button>
        ),
      },
    ],
    [openDiagnosis, t]
  );

  const eventColumns = useMemo(
    () => [
      {key: "time", title: t("monitor:Time"), width: 200, render: (_, record) => formatTime(eventDisplayTime(record))},
      {
        key: "type",
        title: t("policy:Type"),
        dataIndex: "type",
        width: 120,
        sortable: true,
        render: (value) => <Badge variant={EVENT_TYPE_VARIANTS[value] ?? "muted"}>{value || "-"}</Badge>,
      },
      {key: "namespace", title: t("policy:Namespace"), dataIndex: "namespace", width: 160, sortable: true},
      {
        key: "object",
        title: t("monitor:Object"),
        width: 260,
        ellipsis: true,
        render: (_, record) => `${record.involvedObjectKind || "-"} / ${record.involvedObjectName || "-"}`,
      },
      {key: "reason", title: t("monitor:Reason"), dataIndex: "reason", width: 190, sortable: true},
      {key: "message", title: t("monitor:Message"), dataIndex: "message", width: 420, ellipsis: true},
      {key: "count", title: t("monitor:Count"), dataIndex: "count", width: 100, align: "right", sortable: true},
      {
        key: "action",
        title: t("general:Action"),
        width: 120,
        align: "right",
        render: (_, record) => (
          <Button variant="outline" size="sm" onClick={() => setSelectedEvent(record)}>
            {t("monitor:Details")}
          </Button>
        ),
      },
    ],
    [t]
  );

  const diagnosisEventColumns = useMemo(
    () => [
      {key: "time", title: t("monitor:Time"), width: 190, render: (_, record) => formatTime(eventDisplayTime(record))},
      {
        key: "type",
        title: t("policy:Type"),
        dataIndex: "type",
        width: 110,
        render: (value) => <Badge variant={EVENT_TYPE_VARIANTS[value] ?? "muted"}>{value || "-"}</Badge>,
      },
      {key: "reason", title: t("monitor:Reason"), dataIndex: "reason", width: 170},
      {key: "message", title: t("monitor:Message"), dataIndex: "message", ellipsis: true},
      {key: "count", title: t("monitor:Count"), dataIndex: "count", width: 90, align: "right"},
    ],
    [t]
  );

  if (loading && !summary) {
    return <Loading type="page" />;
  }

  const overallStatus = summary?.overallStatus || "unknown";
  const statusMeta = STATUS_META[overallStatus] || STATUS_META.unknown;
  const abnormalPods = summary?.podAbnormal ?? 0;
  const warningEvents = summary?.warningEventCount ?? 0;
  const criticalChecks = summary?.criticalCheckCount ?? 0;
  const warningChecks = summary?.warningCheckCount ?? 0;

  return (
    <PageContainer>
      {error ? <MessageAlert title={t("monitor:Failed to load monitor data")} description={error} /> : null}

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatCard
          label={t("monitor:Overall Status")}
          value={t(`monitor:status ${overallStatus}`)}
          icon={statusMeta.Icon}
          tone={statusMeta.tone}
        />
        <StatCard
          label={t("monitor:Ready Nodes")}
          value={summary?.nodeReady ?? 0}
          suffix={`/ ${summary?.nodeTotal ?? 0}`}
          icon={Server}
          tone="info"
        />
        <StatCard
          label={t("monitor:Running Pods")}
          value={summary?.podRunning ?? 0}
          suffix={`/ ${summary?.podTotal ?? 0}`}
          icon={Boxes}
          tone="info"
        />
        <StatCard
          label={t("monitor:Abnormal Pods")}
          value={abnormalPods}
          icon={TriangleAlert}
          tone={abnormalPods > 0 ? "danger" : "success"}
        />
        <StatCard
          label={t("monitor:Warning Events")}
          value={warningEvents}
          icon={Bell}
          tone={warningEvents > 0 ? "warning" : "success"}
        />
        <StatCard
          label={t("monitor:Critical Checks")}
          value={criticalChecks}
          icon={CircleX}
          tone={criticalChecks > 0 ? "danger" : "success"}
        />
        <StatCard
          label={t("monitor:Warning Checks")}
          value={warningChecks}
          icon={CircleAlert}
          tone={warningChecks > 0 ? "warning" : "success"}
        />
        <StatCard label={t("monitor:Last Checked")} value={formatTime(summary?.lastCheckedAt)} icon={Clock} />
      </div>

      <DataTable
        title={t("monitor:Health Checks")}
        columns={checkColumns}
        dataSource={checks}
        rowKey="id"
        loading={loading}
        pageSize={0}
        searchable
        emptyText="No health checks"
        toolbar={
          <Button variant="outline" size="sm" loading={loading} onClick={fetchOverview}>
            <RefreshCw />
            {t("general:Refresh")}
          </Button>
        }
      />

      {issuesError ? (
        <MessageAlert title={t("monitor:Failed to load monitor issues")} description={issuesError} />
      ) : null}

      <DataTable
        title={t("monitor:Monitor Issues")}
        columns={issueColumns}
        dataSource={issues}
        rowKey="id"
        loading={issuesLoading}
        searchable
        emptyText="No issues detected"
        onRowClick={openDiagnosis}
        toolbar={
          <Button variant="outline" size="sm" loading={issuesLoading} onClick={fetchIssues}>
            <RefreshCw />
            {t("general:Refresh")}
          </Button>
        }
      />

      {eventsError ? <MessageAlert title={t("monitor:Failed to load events")} description={eventsError} /> : null}

      <DataTable
        title={t("monitor:Event Center")}
        columns={eventColumns}
        dataSource={events}
        rowKey={(record, index) =>
          `${record.namespace}-${record.involvedObjectKind}-${record.involvedObjectName}-${record.reason}-${eventDisplayTime(
            record
          )}-${index}`
        }
        loading={eventsLoading}
        emptyText="No events"
        onRowClick={setSelectedEvent}
        toolbar={
          <>
            <Input
              value={namespaceFilter}
              onChange={(event) => setNamespaceFilter(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  fetchEvents(namespaceFilter);
                }
              }}
              placeholder={t("policy:Namespace")}
              className="h-8 w-52 text-xs"
            />
            <Button variant="outline" size="sm" loading={eventsLoading} onClick={() => fetchEvents(namespaceFilter)}>
              <RefreshCw />
              {t("general:Refresh")}
            </Button>
          </>
        }
      />

      <Dialog open={Boolean(selectedEvent)} onOpenChange={(next) => (next ? null : setSelectedEvent(null))}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("monitor:Event Details")}</DialogTitle>
          </DialogHeader>

          {selectedEvent ? (
            <div className="grid gap-4">
              <DescriptionList
                columns={2}
                items={[
                  {key: "time", label: t("monitor:Time"), value: formatTime(eventDisplayTime(selectedEvent))},
                  {
                    key: "type",
                    label: t("policy:Type"),
                    value: <Badge variant={EVENT_TYPE_VARIANTS[selectedEvent.type] ?? "muted"}>{selectedEvent.type || "-"}</Badge>,
                  },
                  {key: "namespace", label: t("policy:Namespace"), value: selectedEvent.namespace || "-"},
                  {
                    key: "object",
                    label: t("monitor:Object"),
                    value: `${selectedEvent.involvedObjectKind || "-"} / ${selectedEvent.involvedObjectName || "-"}`,
                  },
                  {key: "reason", label: t("monitor:Reason"), value: selectedEvent.reason || "-"},
                  {key: "count", label: t("monitor:Count"), value: selectedEvent.count ?? 0},
                  {
                    key: "source",
                    label: t("monitor:Source"),
                    value: selectedEvent.source || selectedEvent.reportingController || "-",
                  },
                ]}
              />
              <p className="bg-muted/40 rounded-lg border p-3 text-sm whitespace-pre-wrap">
                {selectedEvent.message || "-"}
              </p>
            </div>
          ) : null}

          <DialogFooter>
            <Button variant="outline" onClick={() => setSelectedEvent(null)}>
              {t("general:Close")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ResourceSheet
        open={Boolean(selectedIssue)}
        onOpenChange={(next) => {
          if (next) {
            return;
          }
          setSelectedIssue(null);
          setDiagnosis(null);
          setDiagnosisError(null);
        }}
        title={t("monitor:Diagnosis Context")}
        size="xl"
        bodyClassName="gap-4 overflow-y-auto scrollbar-thin"
      >
        {diagnosisError ? (
          <MessageAlert title={t("monitor:Failed to load diagnosis")} description={diagnosisError} />
        ) : null}

        {diagnosisLoading ? <Loading /> : null}

        {diagnosis ? (
          <>
            <div className="bg-muted/40 rounded-lg border p-3">
              <DescriptionList
                columns={2}
                items={[
                  {key: "object", label: t("monitor:Object"), value: objectLabel(diagnosis.issue)},
                  {
                    key: "severity",
                    label: t("trivy:Severity"),
                    value: (
                      <Badge variant={SEVERITY_VARIANTS[diagnosis.issue?.severity] ?? "muted"}>
                        {t(`monitor:severity ${diagnosis.issue?.severity || "info"}`)}
                      </Badge>
                    ),
                  },
                  {key: "reason", label: t("monitor:Reason"), value: diagnosis.issue?.reason || "-"},
                  {key: "message", label: t("monitor:Message"), value: diagnosis.issue?.message || "-"},
                  {key: "suggestion", label: t("monitor:Suggestion"), value: diagnosis.issue?.suggestion || "-"},
                  {key: "lastSeen", label: t("monitor:Last Seen"), value: formatTime(diagnosis.issue?.lastSeenAt)},
                ]}
              />
            </div>

            <div>
              <div className="mb-2 text-sm font-semibold">{t("monitor:Related Events")}</div>
              <DataTable
                columns={diagnosisEventColumns}
                dataSource={diagnosis.relatedEvents || []}
                rowKey={(record, index) => `${record.namespace}-${record.reason}-${eventDisplayTime(record)}-${index}`}
                pageSize={0}
                dense
                emptyText="No related events"
              />
            </div>

            <div>
              <div className="mb-2 text-sm font-semibold">{t("monitor:Log Preview")}</div>
              <div className="grid gap-3">
                {(diagnosis.logPreview || []).map((log, index) => (
                  <div key={`${log.container}-${log.previous}-${index}`}>
                    <div className="mb-1.5 flex flex-wrap gap-1.5">
                      <Badge variant="muted">{log.container || "-"}</Badge>
                      <Badge variant={log.previous ? "warning" : "info"}>
                        {log.previous ? t("monitor:Previous") : t("monitor:Current")}
                      </Badge>
                      <Badge variant="muted">tail {log.tailLines || 0}</Badge>
                    </div>
                    <pre className="scrollbar-thin bg-muted/40 max-h-56 overflow-auto rounded-lg border p-3 font-mono text-xs whitespace-pre-wrap">
                      {log.error || log.content || "-"}
                    </pre>
                  </div>
                ))}
              </div>
            </div>

            <div>
              <div className="mb-2 text-sm font-semibold">{t("monitor:Diagnosis")}</div>
              <pre className="scrollbar-thin bg-muted/40 max-h-64 overflow-auto rounded-lg border p-3 font-mono text-xs whitespace-pre-wrap">
                {JSON.stringify(diagnosis.aiContext || {}, null, 2)}
              </pre>
            </div>
          </>
        ) : null}
      </ResourceSheet>
    </PageContainer>
  );
}

export default MonitorPage;
