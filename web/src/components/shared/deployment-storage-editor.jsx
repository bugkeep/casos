import React from "react";
import {HardDrive, Plus, X} from "lucide-react";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {SimpleTooltip} from "@/components/ui/tooltip";

/**
 * Volumes attached to a Deployment.
 *
 * Creating one provisions a PVC per mount, so the add form asks for a path and a
 * size. Those mounts are fixed once the Deployment exists — Kubernetes will not
 * re-bind a claim under a running workload — so edit mode shows them read-only
 * and says why rather than offering fields that would be ignored.
 *
 * add mode:  value = [{mountPath, size}]
 * edit mode: value = [{claimName, mountPath}]
 */
export function DeploymentStorageEditor({mode, value = [], onChange}) {
  if (mode !== "add") {
    return (
      <div className="grid gap-1.5">
        {value.length === 0 ? (
          <span className="text-muted-foreground text-xs">No persistent storage attached.</span>
        ) : (
          <div className="flex flex-wrap gap-1">
            {value.map((volume, index) => (
              <SimpleTooltip key={index} title={`PVC: ${volume.claimName}`}>
                <Badge variant="info">
                  <HardDrive />
                  {volume.mountPath}
                </Badge>
              </SimpleTooltip>
            ))}
          </div>
        )}
        <p className="text-muted-foreground text-xs">Storage mounts cannot be changed after creation.</p>
      </div>
    );
  }

  function update(index, field, next) {
    onChange(value.map((volume, volumeIndex) => (volumeIndex === index ? {...volume, [field]: next} : volume)));
  }

  return (
    <div className="grid gap-2">
      {value.map((volume, index) => (
        <div key={index} className="grid grid-cols-[minmax(0,2fr)_minmax(0,1fr)_auto] items-center gap-2">
          <Input
            value={volume.mountPath ?? ""}
            onChange={(event) => update(index, "mountPath", event.target.value)}
            placeholder="Mount path, e.g. /data"
            className="h-8 font-mono text-xs"
          />
          <Input
            value={volume.size ?? ""}
            onChange={(event) => update(index, "size", event.target.value)}
            placeholder="Size, e.g. 1Gi"
            className="h-8 font-mono text-xs"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={() => onChange(value.filter((_, volumeIndex) => volumeIndex !== index))}
            className="text-muted-foreground hover:text-destructive"
            aria-label="Remove volume"
          >
            <X className="size-4" />
          </Button>
        </div>
      ))}

      <Button
        type="button"
        variant="outline"
        size="sm"
        className="border-dashed"
        onClick={() => onChange([...value, {mountPath: "", size: "1Gi"}])}
      >
        <Plus />
        Add Volume
      </Button>

      {value.length === 0 ? (
        <p className="text-muted-foreground text-xs">No persistent storage. Data is lost when the container restarts.</p>
      ) : null}
    </div>
  );
}

export default DeploymentStorageEditor;
