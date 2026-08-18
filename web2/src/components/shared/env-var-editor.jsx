import * as React from "react";
import {Plus, X} from "lucide-react";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {SearchSelect, SimpleSelect} from "@/components/shared/simple-select";

export const ENV_SOURCE_PLAIN = "plain";
export const ENV_SOURCE_CONFIGMAP = "configmap";
export const ENV_SOURCE_SECRET = "secret";

const SOURCE_OPTIONS = [
  {label: "Plain", value: ENV_SOURCE_PLAIN},
  {label: "ConfigMap", value: ENV_SOURCE_CONFIGMAP},
  {label: "Secret", value: ENV_SOURCE_SECRET},
];

/**
 * Environment variables for a deployment, each either a literal or a reference
 * into a ConfigMap or Secret. Switching the source resets the row to just its
 * variable name, because a ConfigMap key means nothing to a plain value.
 *
 * Props: value [{source, name, value, configMapName, configMapKey, secretName,
 * secretKey}], onChange, configMaps [{name, data}], secrets [{name, stringData}]
 */
export function EnvVarEditor({value = [], onChange, configMaps = [], secrets = []}) {
  function update(index, field, next) {
    const rows = [...value];
    rows[index] = field === "source" ? {source: next, name: rows[index].name} : {...rows[index], [field]: next};
    onChange(rows);
  }

  const configMapOptions = configMaps.map((item) => ({label: item.name, value: item.name}));
  const secretOptions = secrets.map((item) => ({label: item.name, value: item.name}));

  return (
    <div className="grid gap-2">
      {value.length > 0 ? (
        <div className="text-muted-foreground grid grid-cols-[110px_150px_minmax(0,1fr)_auto] gap-2 text-xs">
          <span>Source</span>
          <span>Variable name</span>
          <span>Value / reference</span>
          <span className="w-8" />
        </div>
      ) : null}

      {value.map((row, index) => {
        const selectedConfigMap = configMaps.find((item) => item.name === row.configMapName);
        const configMapKeys = Object.keys(selectedConfigMap?.data ?? {}).map((key) => ({label: key, value: key}));
        const selectedSecret = secrets.find((item) => item.name === row.secretName);
        const secretKeys = Object.keys(selectedSecret?.stringData ?? {}).map((key) => ({label: key, value: key}));

        return (
          <div key={index} className="grid grid-cols-[110px_150px_minmax(0,1fr)_auto] items-start gap-2">
            <SimpleSelect
              value={row.source}
              onChange={(next) => update(index, "source", next)}
              options={SOURCE_OPTIONS}
              size="sm"
            />
            <Input
              value={row.name ?? ""}
              onChange={(event) => update(index, "name", event.target.value)}
              placeholder="VAR_NAME"
              className="h-8 font-mono text-xs"
            />

            {row.source === ENV_SOURCE_PLAIN ? (
              <Input
                value={row.value ?? ""}
                onChange={(event) => update(index, "value", event.target.value)}
                placeholder="value"
                className="h-8 font-mono text-xs"
              />
            ) : (
              <div className="grid grid-cols-2 gap-2">
                <SearchSelect
                  value={row.source === ENV_SOURCE_CONFIGMAP ? row.configMapName : row.secretName}
                  onChange={(next) => update(index, row.source === ENV_SOURCE_CONFIGMAP ? "configMapName" : "secretName", next)}
                  options={row.source === ENV_SOURCE_CONFIGMAP ? configMapOptions : secretOptions}
                  placeholder={row.source === ENV_SOURCE_CONFIGMAP ? "ConfigMap" : "Secret"}
                  className="h-8 text-xs"
                />
                <SearchSelect
                  value={row.source === ENV_SOURCE_CONFIGMAP ? row.configMapKey : row.secretKey}
                  onChange={(next) => update(index, row.source === ENV_SOURCE_CONFIGMAP ? "configMapKey" : "secretKey", next)}
                  options={row.source === ENV_SOURCE_CONFIGMAP ? configMapKeys : secretKeys}
                  placeholder="Key"
                  disabled={row.source === ENV_SOURCE_CONFIGMAP ? !row.configMapName : !row.secretName}
                  className="h-8 text-xs"
                />
              </div>
            )}

            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              onClick={() => onChange(value.filter((_, rowIndex) => rowIndex !== index))}
              className="text-muted-foreground hover:text-destructive"
              aria-label="Remove variable"
            >
              <X className="size-4" />
            </Button>
          </div>
        );
      })}

      <Button
        type="button"
        variant="outline"
        size="sm"
        className="w-full border-dashed"
        onClick={() => onChange([...value, {source: ENV_SOURCE_PLAIN, name: "", value: ""}])}
      >
        <Plus />
        Add Environment Variable
      </Button>
    </div>
  );
}

export default EnvVarEditor;

// The API models an env var as one of three shapes distinguished by which
// reference fields are set; the editor models it as a `source` discriminator.
// Deployments, StatefulSets and DaemonSets all need this translation, so it
// lives next to the editor rather than being re-derived per page.
export function envVarsToRows(envVars = []) {
  return envVars.map((envVar) => {
    if (envVar.configMapName) {
      return {
        source: ENV_SOURCE_CONFIGMAP,
        name: envVar.name,
        configMapName: envVar.configMapName,
        configMapKey: envVar.configMapKey,
      };
    }
    if (envVar.secretName) {
      return {source: ENV_SOURCE_SECRET, name: envVar.name, secretName: envVar.secretName, secretKey: envVar.secretKey};
    }
    return {source: ENV_SOURCE_PLAIN, name: envVar.name, value: envVar.value};
  });
}

export function rowsToEnvVars(rows = []) {
  return rows
    .filter((row) => row.name)
    .map((row) => {
      if (row.source === ENV_SOURCE_CONFIGMAP) {
        return {name: row.name, configMapName: row.configMapName ?? "", configMapKey: row.configMapKey ?? ""};
      }
      if (row.source === ENV_SOURCE_SECRET) {
        return {name: row.name, secretName: row.secretName ?? "", secretKey: row.secretKey ?? ""};
      }
      return {name: row.name, value: row.value ?? ""};
    });
}
