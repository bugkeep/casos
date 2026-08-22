import * as React from "react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";

/**
 * Stands in for the antd Popconfirm that guarded every destructive row action.
 * The trigger is passed as children so a call site stays a single element:
 *
 *   <ConfirmDialog title="Delete pod?" onConfirm={...}>
 *     <Button variant="outline" size="sm">Delete</Button>
 *   </ConfirmDialog>
 *
 * The confirm button is destructive by default because that is what nearly
 * every call site is guarding; pass variant="default" for benign confirmations.
 */
export function ConfirmDialog({
  children,
  title,
  description,
  extra,
  confirmText = "Confirm",
  cancelText = "Cancel",
  variant = "destructive",
  onConfirm,
  disabled = false,
  open,
  onOpenChange,
}) {
  const [pending, setPending] = React.useState(false);

  async function handleConfirm(event) {
    // The dialog closes itself on action; awaiting first would let it close
    // before an async handler reports a failure, so errors are surfaced by the
    // handler's own toast instead.
    event.preventDefault();
    setPending(true);
    try {
      await onConfirm?.();
    } finally {
      setPending(false);
      onOpenChange?.(false);
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogTrigger asChild disabled={disabled}>
        {children}
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          {description ? <AlertDialogDescription>{description}</AlertDialogDescription> : null}
        </AlertDialogHeader>
        {extra}
        <AlertDialogFooter>
          <AlertDialogCancel>{cancelText}</AlertDialogCancel>
          <AlertDialogAction variant={variant} onClick={handleConfirm} disabled={pending}>
            {confirmText}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
