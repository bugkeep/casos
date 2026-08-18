import {Toaster as Sonner} from "sonner";

// Sonner is themed through CSS variables rather than its own light/dark prop, so
// it follows the `.dark` class on <html> exactly like the rest of the UI and
// never disagrees with the page behind it.
function Toaster({...props}) {
  return (
    <Sonner
      className="toaster group"
      position="top-center"
      richColors
      closeButton
      toastOptions={{
        classNames: {
          toast: "group toast group-[.toaster]:bg-popover group-[.toaster]:text-popover-foreground group-[.toaster]:border-border group-[.toaster]:shadow-lg",
          description: "group-[.toast]:text-muted-foreground",
          actionButton: "group-[.toast]:bg-primary group-[.toast]:text-primary-foreground",
          cancelButton: "group-[.toast]:bg-muted group-[.toast]:text-muted-foreground",
        },
      }}
      {...props}
    />
  );
}

export {Toaster};
