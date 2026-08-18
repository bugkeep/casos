import * as React from "react";
import {Slot} from "@radix-ui/react-slot";
import {cva} from "class-variance-authority";
import {cn} from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-all disabled:pointer-events-none disabled:opacity-50 shrink-0 outline-none [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*=size-])]:size-4 focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px] aria-invalid:border-destructive aria-invalid:ring-destructive/20",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground shadow-xs hover:bg-primary/90",
        destructive: "bg-destructive text-destructive-foreground shadow-xs hover:bg-destructive/90 focus-visible:ring-destructive/20",
        outline: "border bg-background shadow-xs hover:bg-accent hover:text-accent-foreground",
        secondary: "bg-secondary text-secondary-foreground shadow-xs hover:bg-secondary/80",
        ghost: "hover:bg-accent hover:text-accent-foreground",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: {
        default: "h-9 px-4 py-2",
        sm: "h-8 gap-1.5 rounded-md px-3",
        xs: "h-7 gap-1 rounded-md px-2 text-xs",
        lg: "h-10 rounded-md px-6",
        icon: "size-9",
        "icon-sm": "size-8",
        "icon-xs": "size-7",
      },
    },
    defaultVariants: {variant: "default", size: "default"},
  }
);

/**
 * `loading` mirrors the prop every antd button in the old UI relied on: it both
 * shows a spinner and blocks the click, so callers never have to disable the
 * button separately while a request is in flight.
 *
 * forwardRef is required, not optional: this app runs React 18, where a ref is
 * not an ordinary prop, and every Radix `asChild` trigger (tooltip, dropdown,
 * popover, dialog) hands its child a ref to anchor and focus.
 */
const Button = React.forwardRef(function Button(
  {className, variant, size, asChild = false, loading = false, disabled, children, ...props},
  ref
) {
  const classes = cn(buttonVariants({variant, size, className}));

  // asChild renders the caller's own element through Radix's Slot, which accepts
  // exactly one child. The spinner is therefore only injected for a real
  // <button>; adding it here would hand Slot two children and throw.
  if (asChild) {
    return (
      <Slot ref={ref} data-slot="button" className={classes} {...props}>
        {children}
      </Slot>
    );
  }

  return (
    <button ref={ref} data-slot="button" className={classes} disabled={disabled || loading} {...props}>
      {loading ? <span className="size-3.5 animate-spin rounded-full border-2 border-current border-t-transparent" /> : null}
      {children}
    </button>
  );
});

export {Button, buttonVariants};
