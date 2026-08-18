import * as React from "react";
import {Check, ChevronsUpDown, X} from "lucide-react";
import {cn} from "@/lib/utils";
import {Button} from "@/components/ui/button";
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from "@/components/ui/select";
import {Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList} from "@/components/ui/command";
import {Popover, PopoverContent, PopoverTrigger} from "@/components/ui/popover";

// Radix refuses an item whose value is the empty string, but "" is exactly how
// this app spells "no filter" (all namespaces, any status). The sentinel keeps
// that vocabulary intact at the call sites and never leaks out of this module.
const EMPTY_VALUE = "__all__";

const toInternal = (value) => (value === "" || value === undefined || value === null ? EMPTY_VALUE : String(value));
const toExternal = (value) => (value === EMPTY_VALUE ? "" : value);

function normalizeOptions(options) {
  return (options ?? []).map((option) =>
    typeof option === "string" || typeof option === "number" ? {label: String(option), value: option} : option
  );
}

/**
 * A plain dropdown with an options array, matching how the old antd Selects were
 * written. Use SearchSelect instead when the list is long enough to need typing.
 */
export function SimpleSelect({
  value,
  onChange,
  options,
  placeholder = "Select...",
  className,
  contentClassName,
  size = "default",
  disabled = false,
  id,
  "aria-label": ariaLabel,
}) {
  const items = normalizeOptions(options);
  return (
    <Select value={toInternal(value)} onValueChange={(next) => onChange?.(toExternal(next))} disabled={disabled}>
      <SelectTrigger id={id} size={size} className={cn("w-full", className)} aria-label={ariaLabel}>
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent className={contentClassName}>
        {items.map((option) => (
          <SelectItem key={String(option.value)} value={toInternal(option.value)} disabled={option.disabled}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/** Combobox for long lists — namespaces, images, service accounts. */
export function SearchSelect({
  value,
  onChange,
  options,
  placeholder = "Select...",
  searchPlaceholder = "Search...",
  emptyText = "No match found",
  className,
  disabled = false,
  allowClear = false,
  id,
}) {
  const [open, setOpen] = React.useState(false);
  const items = normalizeOptions(options);
  const selected = items.find((option) => String(option.value) === String(value ?? ""));

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          className={cn("w-full justify-between font-normal", !selected && "text-muted-foreground", className)}
        >
          <span className="truncate">{selected ? selected.label : placeholder}</span>
          <span className="flex items-center gap-1">
            {allowClear && selected ? (
              <X
                className="hover:text-foreground size-3.5 opacity-50"
                onClick={(event) => {
                  event.stopPropagation();
                  onChange?.("");
                }}
              />
            ) : null}
            <ChevronsUpDown className="size-4 shrink-0 opacity-50" />
          </span>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-(--radix-popover-trigger-width) p-0" align="start">
        <Command>
          <CommandInput placeholder={searchPlaceholder} />
          <CommandList>
            <CommandEmpty>{emptyText}</CommandEmpty>
            <CommandGroup>
              {items.map((option) => (
                <CommandItem
                  key={String(option.value)}
                  value={String(option.label)}
                  onSelect={() => {
                    onChange?.(option.value);
                    setOpen(false);
                  }}
                >
                  <Check className={cn("size-4", String(option.value) === String(value) ? "opacity-100" : "opacity-0")} />
                  {option.label}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
