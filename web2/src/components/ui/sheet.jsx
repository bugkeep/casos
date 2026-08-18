import * as React from "react";
import * as SheetPrimitive from "@radix-ui/react-dialog";
import {XIcon} from "lucide-react";
import {cn} from "@/lib/utils";

function Sheet({...props}) {
  return <SheetPrimitive.Root data-slot="sheet" {...props} />;
}

function SheetTrigger({...props}) {
  return <SheetPrimitive.Trigger data-slot="sheet-trigger" {...props} />;
}

function SheetClose({...props}) {
  return <SheetPrimitive.Close data-slot="sheet-close" {...props} />;
}

function SheetPortal({...props}) {
  return <SheetPrimitive.Portal data-slot="sheet-portal" {...props} />;
}

function SheetOverlay({className, ...props}) {
  return (
    <SheetPrimitive.Overlay
      data-slot="sheet-overlay"
      className={cn(
        "data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 fixed inset-0 z-50 bg-black/50",
        className
      )}
      {...props}
    />
  );
}

// The old UI leaned on antd Drawers for logs, terminals and file browsers, which
// are wide panes of monospace content. `size` therefore goes well past the
// shadcn default width instead of forcing every caller to pass a class.
const sideWidths = {
  sm: "sm:max-w-sm",
  md: "sm:max-w-xl",
  lg: "sm:max-w-3xl",
  xl: "sm:max-w-5xl",
  full: "sm:max-w-[92vw]",
};

function SheetContent({className, children, side = "right", size = "md", showCloseButton = true, ...props}) {
  const isHorizontal = side === "left" || side === "right";
  return (
    <SheetPortal>
      <SheetOverlay />
      <SheetPrimitive.Content
        data-slot="sheet-content"
        className={cn(
          "bg-background data-[state=open]:animate-in data-[state=closed]:animate-out fixed z-50 flex flex-col gap-4 shadow-lg transition ease-in-out data-[state=closed]:duration-300 data-[state=open]:duration-500",
          side === "right" &&
            "data-[state=closed]:slide-out-to-right data-[state=open]:slide-in-from-right inset-y-0 right-0 h-full w-full border-l",
          side === "left" &&
            "data-[state=closed]:slide-out-to-left data-[state=open]:slide-in-from-left inset-y-0 left-0 h-full w-full border-r",
          side === "top" &&
            "data-[state=closed]:slide-out-to-top data-[state=open]:slide-in-from-top inset-x-0 top-0 h-auto border-b",
          side === "bottom" &&
            "data-[state=closed]:slide-out-to-bottom data-[state=open]:slide-in-from-bottom inset-x-0 bottom-0 h-auto border-t",
          isHorizontal && sideWidths[size],
          className
        )}
        {...props}
      >
        {children}
        {showCloseButton && (
          <SheetPrimitive.Close className="ring-offset-background focus:ring-ring data-[state=open]:bg-secondary absolute top-4 right-4 rounded-xs opacity-70 transition-opacity hover:opacity-100 focus:ring-2 focus:ring-offset-2 focus:outline-hidden disabled:pointer-events-none">
            <XIcon className="size-4" />
            <span className="sr-only">Close</span>
          </SheetPrimitive.Close>
        )}
      </SheetPrimitive.Content>
    </SheetPortal>
  );
}

function SheetHeader({className, ...props}) {
  return <div data-slot="sheet-header" className={cn("flex flex-col gap-1.5 border-b p-4 pr-12", className)} {...props} />;
}

function SheetFooter({className, ...props}) {
  return <div data-slot="sheet-footer" className={cn("mt-auto flex flex-col gap-2 border-t p-4", className)} {...props} />;
}

function SheetTitle({className, ...props}) {
  return <SheetPrimitive.Title data-slot="sheet-title" className={cn("text-foreground font-semibold", className)} {...props} />;
}

function SheetDescription({className, ...props}) {
  return (
    <SheetPrimitive.Description
      data-slot="sheet-description"
      className={cn("text-muted-foreground text-sm", className)}
      {...props}
    />
  );
}

export {Sheet, SheetTrigger, SheetClose, SheetContent, SheetHeader, SheetFooter, SheetTitle, SheetDescription};
