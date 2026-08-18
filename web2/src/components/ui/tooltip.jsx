import * as React from "react";
import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import {cn} from "@/lib/utils";

function TooltipProvider({delayDuration = 200, ...props}) {
  return <TooltipPrimitive.Provider data-slot="tooltip-provider" delayDuration={delayDuration} {...props} />;
}

function Tooltip({...props}) {
  return <TooltipPrimitive.Root data-slot="tooltip" {...props} />;
}

function TooltipTrigger({...props}) {
  return <TooltipPrimitive.Trigger data-slot="tooltip-trigger" {...props} />;
}

function TooltipContent({className, sideOffset = 4, children, ...props}) {
  return (
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Content
        data-slot="tooltip-content"
        sideOffset={sideOffset}
        className={cn(
          "bg-primary text-primary-foreground animate-in fade-in-0 zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 z-50 w-fit origin-(--radix-tooltip-content-transform-origin) rounded-md px-3 py-1.5 text-xs text-balance",
          className
        )}
        {...props}
      >
        {children}
        <TooltipPrimitive.Arrow className="bg-primary fill-primary z-50 size-2.5 translate-y-[calc(-50%_-_2px)] rotate-45 rounded-[2px]" />
      </TooltipPrimitive.Content>
    </TooltipPrimitive.Portal>
  );
}

// Most call sites just want "hover this, read that". SimpleTooltip keeps them
// from repeating the four-part Radix composition every time.
function SimpleTooltip({title, children, side = "top", className, ...props}) {
  if (title === undefined || title === null || title === "") {
    return children;
  }
  return (
    <Tooltip {...props}>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent side={side} className={className}>
        {title}
      </TooltipContent>
    </Tooltip>
  );
}

export {Tooltip, TooltipTrigger, TooltipContent, TooltipProvider, SimpleTooltip};
