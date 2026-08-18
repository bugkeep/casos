import React from "react";
import {cn} from "@/lib/utils";

/**
 * Circular percentage gauge. Drawn as an SVG rather than pulled from a chart
 * library because it is one arc and a label — and because it then inherits the
 * theme's colours instead of carrying its own.
 */
export function RadialProgress({value = 0, size = 130, strokeWidth = 10, label, className, tone = "info"}) {
  const percent = Math.min(100, Math.max(0, Number(value) || 0));
  const radius = (size - strokeWidth) / 2;
  const circumference = 2 * Math.PI * radius;
  const offset = circumference - (percent / 100) * circumference;

  const strokeClass = {
    info: "stroke-info",
    success: "stroke-success",
    warning: "stroke-warning",
    danger: "stroke-destructive",
  }[tone];

  const textClass = {
    info: "text-info",
    success: "text-success",
    warning: "text-warning",
    danger: "text-destructive",
  }[tone];

  return (
    <div className={cn("relative shrink-0", className)} style={{width: size, height: size}}>
      <svg width={size} height={size} className="-rotate-90">
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          strokeWidth={strokeWidth}
          className="stroke-muted fill-none"
        />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          strokeWidth={strokeWidth}
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          strokeLinecap="round"
          className={cn("fill-none transition-[stroke-dashoffset] duration-500", strokeClass)}
        />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center">
        <span className={cn("text-xl font-bold tabular-nums", textClass)}>{percent}%</span>
        {label ? <span className="text-muted-foreground mt-0.5 text-[11px]">{label}</span> : null}
      </div>
    </div>
  );
}

export default RadialProgress;
