import React, {useEffect, useRef, useState} from "react";
import {ArrowDownToLine, Download} from "lucide-react";
import * as PodBackend from "@/backend/PodBackend";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {MessageAlert} from "@/components/ui/alert";
import {SimpleTooltip} from "@/components/ui/tooltip";
import {SimpleSelect} from "@/components/shared/simple-select";
import {ResourceSheet} from "@/components/shared/resource-sheet";

const TAIL_OPTIONS = [
  {label: "100 lines", value: 100},
  {label: "500 lines", value: 500},
  {label: "1000 lines", value: 1000},
  {label: "5000 lines", value: 5000},
];

const POLL_INTERVAL = 3000;

/**
 * A live tail of one container's logs. Polling rather than streaming keeps this
 * on the same plain JSON endpoint as the rest of the app; the pane sticks to the
 * bottom only while the reader is already there, so scrolling back to read
 * something is not yanked away three seconds later.
 */
export function PodLogsSheet({pod, open, onClose}) {
  const [container, setContainer] = useState("");
  const [tailLines, setTailLines] = useState(500);
  const [logs, setLogs] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const logsEndRef = useRef(null);
  const autoScrollRef = useRef(true);

  useEffect(() => {
    if (!open || !pod) {
      setLogs("");
      setError(null);
      return undefined;
    }
    setContainer(pod.containers?.[0] ?? "");
  }, [open, pod]);

  useEffect(() => {
    if (!open || !pod) {
      return undefined;
    }

    let cancelled = false;

    function fetchLogs() {
      setLoading(true);
      PodBackend.getPodLogs(pod.namespace, pod.name, container, tailLines)
        .then((res) => {
          if (cancelled) {
            return;
          }
          if (res.status === "ok") {
            setLogs(res.data ?? "");
            setError(null);
            if (autoScrollRef.current) {
              setTimeout(() => logsEndRef.current?.scrollIntoView({behavior: "smooth"}), 50);
            }
          } else {
            setError(res.msg);
          }
        })
        .catch((e) => {
          if (!cancelled) {
            setError(e.message);
          }
        })
        .finally(() => {
          if (!cancelled) {
            setLoading(false);
          }
        });
    }

    fetchLogs();
    const timer = setInterval(fetchLogs, POLL_INTERVAL);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [open, pod, container, tailLines]);

  function handleDownload() {
    const blob = new Blob([logs], {type: "text/plain"});
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `${pod.namespace}_${pod.name}_${container || "default"}.log`;
    link.click();
    URL.revokeObjectURL(url);
  }

  const containerOptions = (pod?.containers ?? []).map((name) => ({label: name, value: name}));

  return (
    <ResourceSheet
      open={open}
      onOpenChange={(next) => (next ? null : onClose())}
      title={pod ? `Logs — ${pod.namespace} / ${pod.name}` : "Logs"}
      size="xl"
      toolbar={
        <>
          {containerOptions.length > 1 ? (
            <SimpleSelect
              value={container}
              onChange={(next) => {
                setContainer(next);
                autoScrollRef.current = true;
              }}
              options={containerOptions}
              size="sm"
              className="w-44"
              placeholder="Container"
            />
          ) : null}
          <SimpleSelect
            value={tailLines}
            onChange={(next) => {
              setTailLines(Number(next));
              autoScrollRef.current = true;
            }}
            options={TAIL_OPTIONS}
            size="sm"
            className="w-32"
          />
          <SimpleTooltip title="Download log">
            <Button variant="outline" size="icon-sm" onClick={handleDownload} disabled={!logs} aria-label="Download log">
              <Download className="size-4" />
            </Button>
          </SimpleTooltip>
          <SimpleTooltip title="Scroll to bottom">
            <Button
              variant="outline"
              size="icon-sm"
              onClick={() => {
                autoScrollRef.current = true;
                logsEndRef.current?.scrollIntoView({behavior: "smooth"});
              }}
              aria-label="Scroll to bottom"
            >
              <ArrowDownToLine className="size-4" />
            </Button>
          </SimpleTooltip>
          <Badge variant={loading ? "info" : "success"}>{loading ? "refreshing…" : "live · 3s"}</Badge>
        </>
      }
    >
      {error ? <MessageAlert title={error} className="mb-3" /> : null}

      <div
        onScroll={(event) => {
          const element = event.currentTarget;
          autoScrollRef.current = element.scrollHeight - element.scrollTop - element.clientHeight < 40;
        }}
        className="scrollbar-thin min-h-0 flex-1 overflow-y-auto rounded-lg bg-neutral-950 p-4 font-mono text-[13px] leading-relaxed break-all whitespace-pre-wrap text-neutral-300"
      >
        {!logs && !loading ? <span className="text-neutral-500">No logs yet…</span> : null}
        {logs}
        <div ref={logsEndRef} />
      </div>
    </ResourceSheet>
  );
}

export default PodLogsSheet;
