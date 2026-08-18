import React, {useEffect, useState} from "react";
import * as PodBackend from "@/backend/PodBackend";
import {Badge} from "@/components/ui/badge";
import {MessageAlert} from "@/components/ui/alert";
import {ResourceSheet} from "@/components/shared/resource-sheet";

const POLL_INTERVAL = 3000;

/**
 * The pod's recent events, polled while the pane is open. Events are where a
 * pod that will not start explains itself, so this stays a terminal-style log
 * rather than a table — the message text is the payload.
 */
export function PodEventsSheet({pod, open, onClose}) {
  const [events, setEvents] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (!open || !pod) {
      setEvents([]);
      setError(null);
      return undefined;
    }

    let cancelled = false;

    function fetchEvents() {
      setLoading(true);
      PodBackend.getPodEvents(pod.namespace, pod.name)
        .then((res) => {
          if (cancelled) {
            return;
          }
          if (res.status === "ok") {
            setEvents(res.data ?? []);
            setError(null);
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

    fetchEvents();
    const timer = setInterval(fetchEvents, POLL_INTERVAL);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [open, pod]);

  return (
    <ResourceSheet
      open={open}
      onOpenChange={(next) => (next ? null : onClose())}
      title={pod ? `Events — ${pod.namespace}/${pod.name}` : "Events"}
      size="lg"
      toolbar={<Badge variant={loading ? "info" : "success"}>{loading ? "refreshing…" : "live · 3s"}</Badge>}
    >
      {error ? <MessageAlert title={error} className="mb-3" /> : null}

      <div className="scrollbar-thin min-h-0 flex-1 overflow-y-auto rounded-lg bg-neutral-950 p-4 font-mono text-[13px] leading-relaxed text-neutral-300">
        {events.length === 0 && !loading ? <span className="text-neutral-500">No events yet…</span> : null}
        {events.map((event, index) => (
          <div key={index} className="mb-1.5">
            <span className="text-neutral-500">{event.lastTimestamp}</span>{" "}
            <span className={event.type === "Warning" ? "font-semibold text-red-400" : "font-semibold text-green-400"}>
              {event.type}
            </span>{" "}
            <span className="text-sky-300">{event.reason}</span>
            {event.count > 1 ? <span className="text-neutral-500"> (×{event.count})</span> : null}
            {" — "}
            <span>{event.message}</span>
          </div>
        ))}
      </div>
    </ResourceSheet>
  );
}

export default PodEventsSheet;
