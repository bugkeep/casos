import * as React from "react";
import {Plus, X} from "lucide-react";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {SimpleSelect} from "@/components/shared/simple-select";

export const SUBJECT_KINDS = ["ServiceAccount", "User", "Group"];

const KIND_OPTIONS = SUBJECT_KINDS.map((kind) => ({label: kind, value: kind}));

const kindVariant = {
  ServiceAccount: "info",
  User: "success",
  Group: "secondary",
};

/** Renders a binding's subject list as badges — the table cell counterpart. */
export function SubjectBadges({subjects}) {
  return (
    <div className="flex flex-wrap gap-1">
      {(subjects ?? []).map((subject, index) => (
        <Badge key={index} variant={kindVariant[subject.kind] ?? "muted"}>
          {subject.kind}/{subject.name}
          {subject.namespace ? ` (${subject.namespace})` : ""}
        </Badge>
      ))}
    </div>
  );
}

/**
 * Subject rows for a RoleBinding or ClusterRoleBinding. Namespace only applies
 * to ServiceAccount subjects, so the field disables itself for the other kinds
 * rather than accepting a value the API will ignore.
 */
export function SubjectsEditor({value = [], onChange}) {
  function update(index, field, next) {
    onChange(value.map((row, rowIndex) => (rowIndex === index ? {...row, [field]: next} : row)));
  }

  return (
    <div className="grid gap-2">
      {value.map((row, index) => (
        <div key={index} className="grid grid-cols-[150px_minmax(0,1fr)_minmax(0,1fr)_auto] items-center gap-2">
          <SimpleSelect value={row.kind} onChange={(next) => update(index, "kind", next)} options={KIND_OPTIONS} size="sm" />
          <Input
            value={row.name ?? ""}
            onChange={(event) => update(index, "name", event.target.value)}
            placeholder="name"
            className="h-8 text-xs"
          />
          <Input
            value={row.namespace ?? ""}
            onChange={(event) => update(index, "namespace", event.target.value)}
            placeholder={row.kind === "ServiceAccount" ? "namespace" : "namespace (SA only)"}
            disabled={row.kind !== "ServiceAccount"}
            className="h-8 text-xs"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={() => onChange(value.filter((_, rowIndex) => rowIndex !== index))}
            className="text-muted-foreground hover:text-destructive"
            aria-label="Remove subject"
          >
            <X className="size-4" />
          </Button>
        </div>
      ))}

      <Button
        type="button"
        variant="outline"
        size="sm"
        className="border-dashed"
        onClick={() => onChange([...value, {kind: "ServiceAccount", name: "", namespace: ""}])}
      >
        <Plus />
        Add Subject
      </Button>
    </div>
  );
}

export function subjectsToRows(subjects) {
  return (subjects ?? []).map((subject) => ({
    kind: subject.kind,
    name: subject.name,
    namespace: subject.namespace || "",
  }));
}
