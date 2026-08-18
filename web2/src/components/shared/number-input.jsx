import * as React from "react";
import {Minus, Plus} from "lucide-react";
import {cn} from "@/lib/utils";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";

/**
 * Replacement for antd's InputNumber. Replica counts and resource limits are
 * nudged far more often than they are typed, so the steppers stay visible
 * instead of appearing on hover.
 */
export function NumberInput({value, onChange, min, max, step = 1, disabled = false, className, id, placeholder, ...props}) {
  function clamp(next) {
    if (Number.isNaN(next)) {
      return value;
    }
    if (min !== undefined && next < min) {
      return min;
    }
    if (max !== undefined && next > max) {
      return max;
    }
    return next;
  }

  return (
    <div className={cn("flex items-center gap-1", className)}>
      <Button
        type="button"
        variant="outline"
        size="icon-sm"
        disabled={disabled || (min !== undefined && Number(value) <= min)}
        onClick={() => onChange?.(clamp(Number(value ?? 0) - step))}
        aria-label="Decrease"
      >
        <Minus className="size-3.5" />
      </Button>
      <Input
        id={id}
        type="number"
        inputMode="numeric"
        value={value ?? ""}
        min={min}
        max={max}
        step={step}
        disabled={disabled}
        placeholder={placeholder}
        onChange={(event) => {
          const raw = event.target.value;
          onChange?.(raw === "" ? "" : clamp(Number(raw)));
        }}
        className="h-8 w-20 text-center tabular-nums [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
        {...props}
      />
      <Button
        type="button"
        variant="outline"
        size="icon-sm"
        disabled={disabled || (max !== undefined && Number(value) >= max)}
        onClick={() => onChange?.(clamp(Number(value ?? 0) + step))}
        aria-label="Increase"
      >
        <Plus className="size-3.5" />
      </Button>
    </div>
  );
}
