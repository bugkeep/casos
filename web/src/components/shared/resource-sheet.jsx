import {cn} from "@/lib/utils";
import {Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle} from "@/components/ui/sheet";

/**
 * The side pane the old UI opened with antd Drawer: pod logs, an exec terminal,
 * a file browser, a job history. The body is a flex column that owns its own
 * scrolling, so a terminal can fill it while a log tail scrolls inside it.
 */
export function ResourceSheet({open, onOpenChange, title, description, toolbar, children, size = "lg", bodyClassName, side = "right"}) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side={side} size={size} className="gap-0 p-0">
        <SheetHeader>
          <SheetTitle className="truncate">{title}</SheetTitle>
          {description ? <SheetDescription className="truncate">{description}</SheetDescription> : null}
          {toolbar ? <div className="flex flex-wrap items-center gap-2 pt-2">{toolbar}</div> : null}
        </SheetHeader>
        <div className={cn("flex min-h-0 flex-1 flex-col overflow-hidden p-4", bodyClassName)}>{children}</div>
      </SheetContent>
    </Sheet>
  );
}
