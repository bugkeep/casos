import * as React from "react";
import * as ProgressPrimitive from "@radix-ui/react-progress";
import {cn} from "@/lib/utils";

// `tone` exists because the progress bars in this app are mostly utilisation
// gauges: CPU, memory and disk go amber then red as they fill, and a caller
// should be able to say that without reaching for arbitrary classes.
const toneClasses = {
  default: "bg-primary",
  success: "bg-success",
  warning: "bg-warning",
  danger: "bg-destructive",
  info: "bg-info",
};

function Progress({className, value, tone = "default", indicatorClassName, ...props}) {
  return (
    <ProgressPrimitive.Root
      data-slot="progress"
      className={cn("bg-muted relative h-2 w-full overflow-hidden rounded-full", className)}
      value={value}
      {...props}
    >
      <ProgressPrimitive.Indicator
        data-slot="progress-indicator"
        className={cn("h-full w-full flex-1 transition-all", toneClasses[tone] ?? toneClasses.default, indicatorClassName)}
        style={{transform: `translateX(-${100 - (Number(value) || 0)}%)`}}
      />
    </ProgressPrimitive.Root>
  );
}

export {Progress};
