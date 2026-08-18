import * as React from "react";
import {Slot} from "@radix-ui/react-slot";
import {cva} from "class-variance-authority";
import {cn} from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex w-fit shrink-0 items-center justify-center gap-1 overflow-hidden rounded-md border px-2 py-0.5 text-xs font-medium whitespace-nowrap transition-[color,box-shadow] [&>svg]:pointer-events-none [&>svg]:size-3",
  {
    variants: {
      variant: {
        default: "border-transparent bg-primary text-primary-foreground",
        secondary: "border-transparent bg-secondary text-secondary-foreground",
        destructive: "border-transparent bg-destructive text-destructive-foreground",
        outline: "text-foreground",
        // Semantic tones stand in for antd's colour-named Tags. They are tinted
        // rather than solid, so a table row carrying several of them still reads
        // as a row instead of a row of buttons.
        success: "border-success/25 bg-success/12 text-success",
        warning: "border-warning/30 bg-warning/15 text-warning",
        info: "border-info/25 bg-info/12 text-info",
        danger: "border-destructive/25 bg-destructive/12 text-destructive",
        muted: "border-border bg-muted text-muted-foreground",
      },
    },
    defaultVariants: {variant: "default"},
  }
);

function Badge({className, variant, asChild = false, ...props}) {
  const Comp = asChild ? Slot : "span";
  return <Comp data-slot="badge" className={cn(badgeVariants({variant}), className)} {...props} />;
}

export {Badge, badgeVariants};
