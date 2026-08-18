import React, {useState} from "react";
import {Check, Minus, Pencil, Plus, X} from "lucide-react";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {SimpleTooltip} from "@/components/ui/tooltip";
import {ReadyBadge} from "@/components/shared/status-badge";

/**
 * Inline replica scaling for a Deployment or StatefulSet. The cell reads as a
 * ready/desired badge until it is clicked, so a table of twenty workloads is not
 * twenty spin boxes.
 *
 * `onScale(next)` must return a promise; the control stays busy until it settles.
 */
export function ReplicasControl({readyReplicas = 0, replicas = 0, onScale}) {
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(replicas);
  const [loading, setLoading] = useState(false);

  function startEdit() {
    setValue(replicas);
    setEditing(true);
  }

  function confirm(next) {
    const target = Math.max(0, Number(next ?? value) || 0);
    if (target === replicas) {
      setEditing(false);
      return;
    }
    setLoading(true);
    Promise.resolve(onScale(target)).finally(() => {
      setLoading(false);
      setEditing(false);
    });
  }

  if (!editing) {
    return (
      <span className="flex items-center justify-end gap-1">
        <ReadyBadge ready={readyReplicas} total={replicas} />
        <SimpleTooltip title="Scale">
          <Button variant="ghost" size="icon-xs" onClick={startEdit} className="text-muted-foreground" aria-label="Scale">
            <Pencil className="size-3.5" />
          </Button>
        </SimpleTooltip>
      </span>
    );
  }

  return (
    <span className="flex items-center justify-end gap-1">
      <Button
        variant="outline"
        size="icon-xs"
        disabled={value <= 0 || loading}
        onClick={() => setValue((current) => Math.max(0, current - 1))}
        aria-label="Decrease replicas"
      >
        <Minus className="size-3.5" />
      </Button>
      <Input
        type="number"
        min={0}
        value={value}
        onChange={(event) => setValue(Number(event.target.value) || 0)}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
            confirm(value);
          }
        }}
        className="h-7 w-14 px-1 text-center text-xs tabular-nums [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none"
      />
      <Button
        variant="outline"
        size="icon-xs"
        disabled={loading}
        onClick={() => setValue((current) => current + 1)}
        aria-label="Increase replicas"
      >
        <Plus className="size-3.5" />
      </Button>
      <Button size="icon-xs" loading={loading} onClick={() => confirm(value)} aria-label="Confirm">
        {loading ? null : <Check className="size-3.5" />}
      </Button>
      <Button variant="ghost" size="icon-xs" disabled={loading} onClick={() => setEditing(false)} aria-label="Cancel">
        <X className="size-3.5" />
      </Button>
    </span>
  );
}

export default ReplicasControl;
