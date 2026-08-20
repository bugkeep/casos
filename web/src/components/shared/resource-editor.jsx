import * as React from "react";
import {Wand2} from "lucide-react";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {SimpleTooltip} from "@/components/ui/tooltip";
import {bytesToQuantity, coresToQuantity, describeQuantity, parseQuantity} from "@/lib/quantity";

export const emptyResources = {cpuRequest: "", memoryRequest: "", cpuLimit: "", memoryLimit: ""};

const PRESETS = [
  {label: "XS", values: {cpuRequest: "100m", memoryRequest: "128Mi", cpuLimit: "200m", memoryLimit: "256Mi"}},
  {label: "S", values: {cpuRequest: "250m", memoryRequest: "256Mi", cpuLimit: "500m", memoryLimit: "512Mi"}},
  {label: "M", values: {cpuRequest: "500m", memoryRequest: "512Mi", cpuLimit: "1", memoryLimit: "1Gi"}},
  {label: "L", values: {cpuRequest: "1", memoryRequest: "1Gi", cpuLimit: "2", memoryLimit: "2Gi"}},
];

// Pods of a workload are named "<workload>-<suffix>"; summing them is what
// turns per-pod metrics into the workload figure the editor suggests from.
export function workloadUsage(podMetrics, namespace, name) {
  return (podMetrics ?? [])
    .filter((pod) => pod.namespace === namespace && pod.name.startsWith(`${name}-`))
    .reduce((accumulator, pod) => ({cpuM: accumulator.cpuM + pod.cpuM, memMi: accumulator.memMi + pod.memMi}), {
      cpuM: 0,
      memMi: 0,
    });
}

/** Pulls the four resource strings out of a workload summary for the form. */
export function resourcesFromRecord(record) {
  return {
    cpuRequest: record?.cpuRequest ?? "",
    memoryRequest: record?.memoryRequest ?? "",
    cpuLimit: record?.cpuLimit ?? "",
    memoryLimit: record?.memoryLimit ?? "",
  };
}

/** The four strings as the API wants them: "" means "remove this one". */
export function resourcesToPayload(form) {
  return {
    cpuRequest: (form.cpuRequest ?? "").trim(),
    memoryRequest: (form.memoryRequest ?? "").trim(),
    cpuLimit: (form.cpuLimit ?? "").trim(),
    memoryLimit: (form.memoryLimit ?? "").trim(),
  };
}

/**
 * Same checks the API runs, so a bad value is caught before the round trip.
 * Returns a {field: message} object, empty when everything parses.
 */
export function validateResources(form) {
  const errors = {};
  const parsed = {};
  for (const key of ["cpuRequest", "memoryRequest", "cpuLimit", "memoryLimit"]) {
    const {value, error} = parseQuantity(form[key]);
    if (error) {
      errors[key] = error;
    }
    parsed[key] = value;
  }
  if (Object.keys(errors).length > 0) {
    return errors;
  }
  if (parsed.cpuRequest !== null && parsed.cpuLimit !== null && parsed.cpuRequest > parsed.cpuLimit) {
    errors.cpuLimit = "CPU limit must be at least the CPU request";
  }
  if (parsed.memoryRequest !== null && parsed.memoryLimit !== null && parsed.memoryRequest > parsed.memoryLimit) {
    errors.memoryLimit = "Memory limit must be at least the memory request";
  }
  return errors;
}

function QuantityInput({id, kind, value, error, placeholder, onChange}) {
  const readout = error ? null : describeQuantity(value, kind);
  const liveError = error ?? parseQuantity(value).error;
  return (
    <div className="grid gap-1">
      <Input
        id={id}
        value={value}
        placeholder={placeholder}
        aria-invalid={liveError ? true : undefined}
        onChange={(event) => onChange(event.target.value)}
      />
      {liveError ? (
        <p className="text-destructive text-xs">{liveError}</p>
      ) : (
        <p className="text-muted-foreground h-4 font-mono text-xs">{readout ?? ""}</p>
      )}
    </div>
  );
}

/**
 * CPU and memory requests/limits for a workload's first container.
 *
 * Props: value {cpuRequest, memoryRequest, cpuLimit, memoryLimit}, onChange,
 * errors {field: message}, usage {cpuM, memMi} — the workload's live
 * consumption, used for the "suggest" button.
 */
export function ResourceEditor({value, onChange, errors = {}, usage = null}) {
  function set(patch) {
    onChange({...value, ...patch});
  }

  const isEmpty = Object.values(resourcesToPayload(value)).every((item) => item === "");
  const hasUsage = usage && (usage.cpuM > 0 || usage.memMi > 0);

  // Requests track what the workload actually uses; limits get headroom so a
  // burst does not get throttled or OOM-killed on the first spike.
  function suggestFromUsage() {
    const cores = Math.max(usage.cpuM / 1000, 0.01);
    const bytes = Math.max(usage.memMi * 1024 * 1024, 32 * 1024 * 1024);
    set({
      cpuRequest: coresToQuantity(cores * 1.2),
      memoryRequest: bytesToQuantity(bytes * 1.2),
      cpuLimit: coresToQuantity(cores * 2),
      memoryLimit: bytesToQuantity(bytes * 2),
    });
  }

  return (
    <div className="grid gap-3">
      <p className="text-muted-foreground text-xs leading-relaxed">
        <span className="text-foreground font-medium">Request</span> is the amount reserved when the pod is scheduled — a
        floor, not a cap. <span className="text-foreground font-medium">Limit</span> is the hard ceiling: over the CPU
        limit the container is throttled, over the memory limit it is killed. Leave everything blank to run unrestricted.
      </p>

      <div className="grid grid-cols-[64px_minmax(0,1fr)_minmax(0,1fr)] items-start gap-x-3 gap-y-2">
        <span />
        <span className="text-muted-foreground text-xs">Request</span>
        <span className="text-muted-foreground text-xs">Limit</span>

        <span className="pt-2 text-sm font-medium">CPU</span>
        <QuantityInput
          id="res-cpu-request"
          kind="cpu"
          value={value.cpuRequest}
          error={errors.cpuRequest}
          placeholder="250m"
          onChange={(next) => set({cpuRequest: next})}
        />
        <QuantityInput
          id="res-cpu-limit"
          kind="cpu"
          value={value.cpuLimit}
          error={errors.cpuLimit}
          placeholder="500m"
          onChange={(next) => set({cpuLimit: next})}
        />

        <span className="pt-2 text-sm font-medium">Memory</span>
        <QuantityInput
          id="res-memory-request"
          kind="memory"
          value={value.memoryRequest}
          error={errors.memoryRequest}
          placeholder="256Mi"
          onChange={(next) => set({memoryRequest: next})}
        />
        <QuantityInput
          id="res-memory-limit"
          kind="memory"
          value={value.memoryLimit}
          error={errors.memoryLimit}
          placeholder="512Mi"
          onChange={(next) => set({memoryLimit: next})}
        />
      </div>

      <div className="flex flex-wrap items-center gap-1.5">
        <span className="text-muted-foreground mr-1 text-xs">Presets</span>
        {PRESETS.map((preset) => (
          <SimpleTooltip
            key={preset.label}
            title={`Requests ${preset.values.cpuRequest} / ${preset.values.memoryRequest}, limits ${preset.values.cpuLimit} / ${preset.values.memoryLimit}`}
          >
            <Button type="button" variant="outline" size="xs" onClick={() => set(preset.values)}>
              {preset.label}
            </Button>
          </SimpleTooltip>
        ))}
        <Button type="button" variant="ghost" size="xs" disabled={isEmpty} onClick={() => set(emptyResources)}>
          Clear
        </Button>
      </div>

      {hasUsage ? (
        <div className="bg-muted/50 flex flex-wrap items-center justify-between gap-2 rounded-md border px-3 py-2">
          <span className="text-muted-foreground text-xs">
            Using right now: <span className="text-foreground font-mono">{usage.cpuM}m</span> CPU ·{" "}
            <span className="text-foreground font-mono">{usage.memMi} MiB</span>
          </span>
          <Button type="button" variant="outline" size="xs" onClick={suggestFromUsage}>
            <Wand2 />
            Suggest from usage
          </Button>
        </div>
      ) : null}
    </div>
  );
}
