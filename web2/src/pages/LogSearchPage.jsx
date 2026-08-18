import React, {useCallback, useEffect, useRef, useState} from "react";
import {ArrowDownToLine, Download, Eraser, Search} from "lucide-react";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import * as DeploymentBackend from "@/backend/DeploymentBackend";
import * as LogBackend from "@/backend/LogBackend";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {SimpleTooltip} from "@/components/ui/tooltip";
import {PageContainer} from "@/components/shared/page-header";
import {SearchSelect, SimpleSelect} from "@/components/shared/simple-select";

// Ten hues that stay distinguishable against the dark log surface. Pod identity
// is the whole point of an aggregated view, so the colour has to carry it.
const POD_COLORS = [
  "#60a5fa",
  "#4ade80",
  "#fb923c",
  "#f472b6",
  "#a78bfa",
  "#22d3ee",
  "#facc15",
  "#f87171",
  "#818cf8",
  "#34d399",
];

const TAIL_OPTIONS = [
  {label: "100 lines / pod", value: 100},
  {label: "200 lines / pod", value: 200},
  {label: "500 lines / pod", value: 500},
  {label: "1000 lines / pod", value: 1000},
];

function assignPodColors(pods) {
  return Object.fromEntries((pods ?? []).map((pod, index) => [pod, POD_COLORS[index % POD_COLORS.length]]));
}

function highlight(text, keyword) {
  if (!keyword) {
    return text;
  }
  const parts = text.split(new RegExp(`(${keyword.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")})`, "gi"));
  return parts.map((part, index) =>
    part.toLowerCase() === keyword.toLowerCase() ? (
      <mark key={index} className="rounded-xs bg-amber-300 px-0 text-neutral-900">
        {part}
      </mark>
    ) : (
      part
    )
  );
}

function LogSearchPage() {
  const [namespaces, setNamespaces] = useState([]);
  const [namespace, setNamespace] = useState("");
  const [deployments, setDeployments] = useState([]);
  const [deployment, setDeployment] = useState("");
  const [keyword, setKeyword] = useState("");
  const [tailLines, setTailLines] = useState(200);

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [lines, setLines] = useState([]);
  const [podColors, setPodColors] = useState({});
  const [searched, setSearched] = useState(false);

  const logEndRef = useRef(null);
  const autoScrollRef = useRef(true);

  useEffect(() => {
    NamespaceBackend.getNamespaces()
      .then((res) => {
        if (res.status !== "ok") {
          return;
        }
        const list = res.data ?? [];
        setNamespaces(list);
        const preferred = list.find((item) => item.name === "default") ?? list[0];
        if (preferred) {
          setNamespace(preferred.name);
        }
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    if (!namespace) {
      return;
    }
    setDeployment("");
    setDeployments([]);
    DeploymentBackend.getDeployments(namespace)
      .then((res) => {
        if (res.status === "ok") {
          setDeployments(res.data ?? []);
        }
      })
      .catch(() => {});
  }, [namespace]);

  const handleSearch = useCallback(() => {
    if (!namespace || !deployment) {
      return;
    }
    setLoading(true);
    setError(null);
    setLines([]);
    setSearched(true);
    autoScrollRef.current = true;

    LogBackend.getAggregatedLogs(namespace, deployment, keyword, tailLines)
      .then((res) => {
        if (res.status === "ok") {
          const data = res.data ?? {lines: [], pods: []};
          setLines(data.lines ?? []);
          setPodColors(assignPodColors(data.pods));
          setTimeout(() => {
            if (autoScrollRef.current) {
              logEndRef.current?.scrollIntoView({behavior: "smooth"});
            }
          }, 50);
        } else {
          setError(res.msg);
        }
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [namespace, deployment, keyword, tailLines]);

  function handleDownload() {
    const text = lines.map((line) => `[${line.pod}][${line.container}] ${line.text}`).join("\n");
    const blob = new Blob([text], {type: "text/plain"});
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `${namespace}_${deployment}_aggregated.log`;
    link.click();
    URL.revokeObjectURL(url);
  }

  const podNames = Object.keys(podColors);

  return (
    <PageContainer className="h-full">
      <div className="bg-card flex flex-wrap items-center gap-2 rounded-xl border p-3 shadow-sm">
        <SearchSelect
          value={namespace}
          onChange={setNamespace}
          options={namespaces.map((item) => ({label: item.name, value: item.name}))}
          placeholder="Namespace"
          className="w-44"
        />
        <SearchSelect
          value={deployment}
          onChange={setDeployment}
          options={deployments.map((item) => ({label: item.name, value: item.name}))}
          placeholder="Deployment"
          emptyText={namespace ? "No deployments" : "Select a namespace first"}
          disabled={!namespace}
          className="w-56"
        />
        <div className="relative">
          <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2" />
          <Input
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                handleSearch();
              }
            }}
            placeholder="Keyword filter (optional)"
            className="w-56 pl-8"
          />
        </div>
        <SimpleSelect value={tailLines} onChange={(next) => setTailLines(Number(next))} options={TAIL_OPTIONS} className="w-44" />

        <Button onClick={handleSearch} loading={loading} disabled={!namespace || !deployment}>
          <Search />
          Search
        </Button>
        <Button
          variant="outline"
          onClick={() => {
            setLines([]);
            setError(null);
            setSearched(false);
          }}
          disabled={lines.length === 0 && !error}
        >
          <Eraser />
          Clear
        </Button>

        <div className="ml-auto flex items-center gap-2">
          {lines.length > 0 ? (
            <span className="text-muted-foreground text-xs">
              {lines.length} lines from {podNames.length} pod{podNames.length !== 1 ? "s" : ""}
            </span>
          ) : null}
          <SimpleTooltip title="Scroll to bottom">
            <Button
              variant="outline"
              size="icon-sm"
              disabled={lines.length === 0}
              onClick={() => {
                autoScrollRef.current = true;
                logEndRef.current?.scrollIntoView({behavior: "smooth"});
              }}
              aria-label="Scroll to bottom"
            >
              <ArrowDownToLine className="size-4" />
            </Button>
          </SimpleTooltip>
          <SimpleTooltip title="Download log">
            <Button
              variant="outline"
              size="icon-sm"
              onClick={handleDownload}
              disabled={lines.length === 0}
              aria-label="Download log"
            >
              <Download className="size-4" />
            </Button>
          </SimpleTooltip>
        </div>
      </div>

      {podNames.length > 0 ? (
        <div className="flex flex-wrap gap-1.5">
          {podNames.map((pod) => (
            <span
              key={pod}
              className="rounded-md border px-2 py-0.5 font-mono text-xs"
              style={{borderColor: podColors[pod], color: podColors[pod]}}
            >
              {pod}
            </span>
          ))}
        </div>
      ) : null}

      {error ? <MessageAlert title={error} /> : null}

      <div
        onScroll={(event) => {
          const element = event.currentTarget;
          autoScrollRef.current = element.scrollHeight - element.scrollTop - element.clientHeight < 60;
        }}
        className="scrollbar-thin h-[calc(100vh-300px)] min-h-64 overflow-y-auto rounded-xl bg-neutral-950 p-4 font-mono text-[12.5px] leading-relaxed text-neutral-300"
      >
        {!searched && !loading ? (
          <span className="text-neutral-500">Select a namespace and deployment, then search to view aggregated logs.</span>
        ) : null}
        {searched && !loading && lines.length === 0 && !error ? (
          <span className="text-neutral-500">No log lines found{keyword ? ` matching "${keyword}"` : ""}.</span>
        ) : null}

        {lines.map((line, index) => (
          <div key={index} className="flex gap-2">
            <span
              className="max-w-[200px] shrink-0 truncate font-semibold"
              style={{color: podColors[line.pod] ?? "#888"}}
              title={`${line.pod} / ${line.container}`}
            >
              [{line.pod}]
            </span>
            <span className="break-all whitespace-pre-wrap">{highlight(line.text, keyword)}</span>
          </div>
        ))}
        <div ref={logEndRef} />
      </div>
    </PageContainer>
  );
}

export default LogSearchPage;
