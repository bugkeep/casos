import {cn} from "@/lib/utils";
import {Progress} from "@/components/ui/progress";

/**
 * A single number with its label, used across the dashboard and the monitor
 * page. `tone` colours the value itself rather than the card, so a wall of
 * these still reads as one surface.
 */
export function StatCard({label, value, suffix, icon: Icon, tone = "default", hint, percent, className, onClick}) {
  const toneClass = {
    default: "text-foreground",
    success: "text-success",
    warning: "text-warning",
    danger: "text-destructive",
    info: "text-info",
  }[tone];

  const Wrapper = onClick ? "button" : "div";

  return (
    <Wrapper
      type={onClick ? "button" : undefined}
      onClick={onClick}
      className={cn(
        "bg-card flex flex-col gap-2 rounded-xl border p-4 text-left shadow-sm",
        onClick && "hover:border-ring/50 transition-colors",
        className
      )}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-muted-foreground truncate text-xs font-medium">{label}</span>
        {Icon ? <Icon className="text-muted-foreground size-4 shrink-0" /> : null}
      </div>
      <div className="flex items-baseline gap-1">
        <span className={cn("text-2xl font-semibold tracking-tight tabular-nums", toneClass)}>{value}</span>
        {suffix ? <span className="text-muted-foreground text-xs">{suffix}</span> : null}
      </div>
      {percent !== undefined && percent !== null ? (
        <Progress
          value={Math.min(100, Math.max(0, Number(percent) || 0))}
          tone={percent >= 90 ? "danger" : percent >= 75 ? "warning" : "success"}
          className="h-1.5"
        />
      ) : null}
      {hint ? <span className="text-muted-foreground text-xs">{hint}</span> : null}
    </Wrapper>
  );
}
