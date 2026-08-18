import * as React from "react";
import {Plus, X} from "lucide-react";
import {cn} from "@/lib/utils";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {Textarea} from "@/components/ui/textarea";
import {PasswordInput} from "@/components/shared/password-input";

/**
 * Repeating key/value rows — ConfigMap data, Secret data, pod labels. The old UI
 * built this out of antd's Form.List at four different call sites; here it is a
 * controlled component so the containing form stays plain state.
 *
 * `value` is [{key, value}] and is passed straight through to onChange.
 */
export function KeyValueEditor({
  value = [],
  onChange,
  keyPlaceholder = "key",
  valuePlaceholder = "value",
  valueType = "text",
  addLabel = "Add Entry",
  keyLabel = "Key",
  valueLabel = "Value",
  disabled = false,
  className,
}) {
  function update(index, field, next) {
    onChange(value.map((entry, entryIndex) => (entryIndex === index ? {...entry, [field]: next} : entry)));
  }

  function remove(index) {
    onChange(value.filter((_, entryIndex) => entryIndex !== index));
  }

  function add() {
    onChange([...value, {key: "", value: ""}]);
  }

  const ValueControl = valueType === "password" ? PasswordInput : valueType === "textarea" ? Textarea : Input;

  return (
    <div className={cn("grid gap-2", className)}>
      {value.length > 0 ? (
        <div className="text-muted-foreground grid grid-cols-[minmax(0,180px)_minmax(0,1fr)_auto] gap-2 text-xs">
          <span>{keyLabel}</span>
          <span>{valueLabel}</span>
          <span className="w-8" />
        </div>
      ) : null}

      {value.map((entry, index) => (
        <div key={index} className="grid grid-cols-[minmax(0,180px)_minmax(0,1fr)_auto] items-start gap-2">
          <Input
            value={entry.key ?? ""}
            onChange={(event) => update(index, "key", event.target.value)}
            placeholder={keyPlaceholder}
            disabled={disabled}
            className="font-mono text-xs"
          />
          <ValueControl
            value={entry.value ?? ""}
            onChange={(event) => update(index, "value", event.target.value)}
            placeholder={valuePlaceholder}
            disabled={disabled}
            className={cn("font-mono text-xs", valueType === "textarea" && "min-h-9 py-1.5")}
            rows={valueType === "textarea" ? 1 : undefined}
          />
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={() => remove(index)}
            disabled={disabled}
            className="text-muted-foreground hover:text-destructive"
            aria-label="Remove entry"
          >
            <X className="size-4" />
          </Button>
        </div>
      ))}

      <Button type="button" variant="outline" size="sm" onClick={add} disabled={disabled} className="border-dashed">
        <Plus />
        {addLabel}
      </Button>
    </div>
  );
}

/** A repeating list of bare strings — image pull secrets, hosts, arguments. */
export function StringListEditor({value = [], onChange, placeholder = "value", addLabel = "Add", disabled = false, className}) {
  return (
    <div className={cn("grid gap-2", className)}>
      {value.map((entry, index) => (
        <div key={index} className="flex items-center gap-2">
          <Input
            value={entry ?? ""}
            onChange={(event) => onChange(value.map((item, itemIndex) => (itemIndex === index ? event.target.value : item)))}
            placeholder={placeholder}
            disabled={disabled}
            className="font-mono text-xs"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={() => onChange(value.filter((_, itemIndex) => itemIndex !== index))}
            disabled={disabled}
            className="text-muted-foreground hover:text-destructive"
            aria-label="Remove"
          >
            <X className="size-4" />
          </Button>
        </div>
      ))}
      <Button type="button" variant="outline" size="sm" onClick={() => onChange([...value, ""])} disabled={disabled} className="border-dashed">
        <Plus />
        {addLabel}
      </Button>
    </div>
  );
}

/** Converts a plain object into the [{key, value}] rows this editor expects. */
export function toEntries(object) {
  return Object.entries(object ?? {}).map(([key, value]) => ({key, value}));
}

/** Inverse of toEntries; rows without a key are dropped, as the API expects. */
export function fromEntries(entries) {
  const result = {};
  (entries ?? []).forEach(({key, value}) => {
    if (key) {
      result[key] = value ?? "";
    }
  });
  return result;
}
