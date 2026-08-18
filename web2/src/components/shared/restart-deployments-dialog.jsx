import React, {useEffect, useState} from "react";
import {RefreshCw} from "lucide-react";
import * as DeploymentBackend from "@/backend/DeploymentBackend";
import * as Setting from "@/Setting";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Checkbox} from "@/components/ui/checkbox";
import {Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle} from "@/components/ui/dialog";
import {CodeText} from "@/components/shared/misc";

function findAffectedDeployments(deployments, namespace, configType, configName) {
  return deployments.filter((deployment) => {
    if (deployment.namespace !== namespace) {
      return false;
    }
    return (deployment.envVars ?? []).some((envVar) => {
      if (configType === "configmap") {
        return envVar.configMapName === configName;
      }
      if (configType === "secret") {
        return envVar.secretName === configName;
      }
      return false;
    });
  });
}

/**
 * Offered right after a ConfigMap or Secret is edited. A running pod keeps the
 * values it started with, so anything referencing the changed object has to roll
 * before the edit takes effect — this finds those deployments and restarts the
 * ones the reader confirms.
 */
export function RestartDeploymentsDialog({open, onClose, namespace, configType, configName}) {
  const [affected, setAffected] = useState([]);
  const [selected, setSelected] = useState([]);
  const [restarting, setRestarting] = useState(false);

  useEffect(() => {
    if (!open) {
      return;
    }
    DeploymentBackend.getDeployments(namespace)
      .then((res) => {
        if (res.status === "ok") {
          const matches = findAffectedDeployments(res.data ?? [], namespace, configType, configName);
          setAffected(matches);
          setSelected(matches.map((deployment) => deployment.name));
        }
      })
      .catch(() => {});
  }, [open, namespace, configType, configName]);

  function toggle(name) {
    setSelected((previous) => (previous.includes(name) ? previous.filter((item) => item !== name) : [...previous, name]));
  }

  function handleRestart() {
    if (selected.length === 0) {
      onClose();
      return;
    }
    setRestarting(true);
    Promise.all(selected.map((name) => DeploymentBackend.restartDeployment(namespace, name)))
      .then((results) => {
        const failed = results.filter((result) => result.status !== "ok");
        if (failed.length === 0) {
          Setting.showMessage("success", `Restarted ${selected.length} deployment(s)`);
        } else {
          Setting.showMessage("error", `${failed.length} restart(s) failed`);
        }
        onClose();
      })
      .catch((error) => Setting.showMessage("error", error.message))
      .finally(() => setRestarting(false));
  }

  const label = configType === "secret" ? "Secret" : "ConfigMap";

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? null : onClose())}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <RefreshCw className="text-info size-4" />
            Restart Apps After Config Update
          </DialogTitle>
          <DialogDescription>
            {affected.length === 0 ? (
              <>
                No deployments in <CodeText>{namespace}</CodeText> reference this {label} via environment variables. You can still
                restart apps manually from the Deployments page.
              </>
            ) : (
              <>
                These deployments reference <CodeText>{configName}</CodeText> and may need a restart to pick up the new config.
              </>
            )}
          </DialogDescription>
        </DialogHeader>

        {affected.length > 0 ? (
          <div className="grid gap-2">
            {affected.map((deployment) => (
              <label key={deployment.name} className="hover:bg-accent/50 flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5">
                <Checkbox checked={selected.includes(deployment.name)} onCheckedChange={() => toggle(deployment.name)} />
                <CodeText>{deployment.name}</CodeText>
                <Badge variant="info">{deployment.namespace}</Badge>
              </label>
            ))}
          </div>
        ) : null}

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Skip
          </Button>
          <Button onClick={handleRestart} loading={restarting} disabled={selected.length === 0}>
            <RefreshCw />
            Restart {selected.length > 0 ? `(${selected.length})` : ""}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default RestartDeploymentsDialog;
