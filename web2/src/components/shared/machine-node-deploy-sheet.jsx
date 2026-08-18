import React, {useCallback, useEffect, useRef, useState} from "react";
import i18next from "i18next";
import {CloudCog, FileSearch, RefreshCw, Wrench} from "lucide-react";
import * as MachineNodeDeployBackend from "@/backend/MachineNodeDeployBackend";
import * as Setting from "@/Setting";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {DataTable} from "@/components/shared/data-table";
import {Field} from "@/components/shared/form-dialog";
import {ResourceSheet} from "@/components/shared/resource-sheet";
import {DescriptionList} from "@/components/shared/misc";

const TASK_STATUS_VARIANTS = {
  pending: "muted",
  running: "info",
  succeeded: "success",
  failed: "danger",
};

const REFRESH_INTERVAL = 5000;

/**
 * Turns a registered machine into a Kubernetes worker over SSH: a preflight
 * check, then a deploy or repair, then the task list and its logs. Deployment is
 * asynchronous, which is why the task list polls while the pane is open.
 */
export function MachineNodeDeploySheet({machine, account, open, onClose}) {
  const [nodeName, setNodeName] = useState("");
  const [apiserverUrl, setApiserverUrl] = useState("");
  const [nodeNameError, setNodeNameError] = useState("");

  const [tasks, setTasks] = useState([]);
  const [logs, setLogs] = useState([]);
  const [selectedTaskId, setSelectedTaskId] = useState(null);
  const [preflight, setPreflight] = useState(null);
  const [preflightError, setPreflightError] = useState(null);

  const [loadingTasks, setLoadingTasks] = useState(false);
  const [preflighting, setPreflighting] = useState(false);
  const [deploying, setDeploying] = useState(false);
  const [repairing, setRepairing] = useState(false);

  const selectedTaskIdRef = useRef(null);
  selectedTaskIdRef.current = selectedTaskId;

  const owner = machine?.owner || account?.name;

  const loadLogs = useCallback((taskId) => {
    if (!taskId) {
      return;
    }
    MachineNodeDeployBackend.getMachineNodeLogs(taskId)
      .then((res) => {
        if (res.status === "ok") {
          setLogs(res.data || []);
          setSelectedTaskId(taskId);
        } else {
          Setting.showMessage("error", res.msg);
        }
      })
      .catch((e) => Setting.showMessage("error", `${i18next.t("machine:Failed to load node logs")}: ${e.message}`));
  }, []);

  const loadTasks = useCallback(
    ({showLoading = true, preferLatest = false} = {}) => {
      if (!machine) {
        return;
      }
      if (!owner) {
        Setting.showMessage("error", i18next.t("machine:Machine owner is required"));
        return;
      }
      if (showLoading) {
        setLoadingTasks(true);
      }
      MachineNodeDeployBackend.getMachineNodeTasks(owner, machine.name)
        .then((res) => {
          if (res.status !== "ok") {
            Setting.showMessage("error", res.msg);
            return;
          }
          const list = res.data || [];
          setTasks(list);
          // A poll must not yank the reader off the task whose logs they are
          // reading; only an explicit deploy asks for the newest one.
          const current = selectedTaskIdRef.current;
          const keepCurrent = !preferLatest && list.some((task) => task.id === current);
          const nextId = keepCurrent ? current : list[0]?.id ?? null;
          if (nextId) {
            loadLogs(nextId);
          }
        })
        .catch((e) => Setting.showMessage("error", `${i18next.t("machine:Failed to load node tasks")}: ${e.message}`))
        .finally(() => setLoadingTasks(false));
    },
    [machine, owner, loadLogs]
  );

  useEffect(() => {
    if (!open || !machine) {
      return undefined;
    }
    setNodeName(machine.name || "");
    setApiserverUrl("");
    setNodeNameError("");
    setPreflight(null);
    setPreflightError(null);
    setLogs([]);
    setSelectedTaskId(null);

    loadTasks();
    const timer = setInterval(() => loadTasks({showLoading: false}), REFRESH_INTERVAL);
    return () => clearInterval(timer);
  }, [open, machine, loadTasks]);

  function buildRequest() {
    if (!nodeName) {
      setNodeNameError(i18next.t("machine:Node name is required"));
      return null;
    }
    if (!owner) {
      Setting.showMessage("error", i18next.t("machine:Machine owner is required"));
      return null;
    }
    setNodeNameError("");
    return {owner, machineName: machine?.name, nodeName, apiserverUrl: apiserverUrl || ""};
  }

  function handlePreflight() {
    const request = buildRequest();
    if (!request) {
      return;
    }
    setPreflighting(true);
    setPreflight(null);
    setPreflightError(null);
    MachineNodeDeployBackend.preflightMachineNode(request)
      .then((res) => {
        if (res.status === "ok") {
          setPreflight(res.data);
          Setting.showMessage("success", i18next.t("machine:Node preflight passed"));
        } else {
          setPreflightError(res.msg);
          Setting.showMessage("error", res.msg);
        }
      })
      .catch((e) => {
        const message = `${i18next.t("machine:Preflight check failed")}: ${e.message}`;
        setPreflightError(message);
        Setting.showMessage("error", message);
      })
      .finally(() => setPreflighting(false));
  }

  function handleDeploy(repair) {
    const request = buildRequest();
    if (!request) {
      return;
    }
    const setBusy = repair ? setRepairing : setDeploying;
    setBusy(true);
    const action = repair ? MachineNodeDeployBackend.repairMachineNode : MachineNodeDeployBackend.deployMachineNode;
    action(request)
      .then((res) => {
        if (res.status === "ok") {
          Setting.showMessage(
            "success",
            repair ? i18next.t("machine:Node repair started") : i18next.t("machine:Node deployment started")
          );
          setSelectedTaskId(res.data?.id || null);
          setLogs([]);
        } else {
          Setting.showMessage("error", res.msg);
        }
        loadTasks({preferLatest: true});
      })
      .catch((e) => {
        Setting.showMessage(
          "error",
          `${repair ? i18next.t("machine:Repair failed") : i18next.t("machine:Deployment failed")}: ${e.message}`
        );
        loadTasks({showLoading: false, preferLatest: true});
      })
      .finally(() => setBusy(false));
  }

  const selectedTask = tasks.find((task) => task.id === selectedTaskId);
  const preflightData = preflight?.preflight ?? {};

  const columns = [
    {key: "id", title: "ID", dataIndex: "id", width: 80},
    {key: "nodeName", title: i18next.t("machine:Node name"), dataIndex: "nodeName", width: 170},
    {
      key: "status",
      title: i18next.t("general:Status"),
      dataIndex: "status",
      width: 130,
      render: (value) => <Badge variant={TASK_STATUS_VARIANTS[value] ?? "muted"}>{value}</Badge>,
    },
    {key: "phase", title: i18next.t("general:Phase"), dataIndex: "phase"},
    {key: "updatedAt", title: i18next.t("general:Updated"), dataIndex: "updatedAt", width: 200},
  ];

  return (
    <ResourceSheet
      open={open}
      onOpenChange={(next) => (next ? null : onClose())}
      title={machine ? `${i18next.t("machine:Worker Node")} — ${machine.name}` : i18next.t("machine:Worker Node")}
      size="lg"
      bodyClassName="gap-4 overflow-y-auto scrollbar-thin"
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={i18next.t("machine:Node name")} htmlFor="deploy-node-name" required error={nodeNameError}>
          <Input
            id="deploy-node-name"
            value={nodeName}
            onChange={(event) => setNodeName(event.target.value)}
            placeholder={machine?.name || "worker-node"}
          />
        </Field>
        <Field label={i18next.t("machine:Apiserver URL")} htmlFor="deploy-apiserver">
          <Input
            id="deploy-apiserver"
            value={apiserverUrl}
            onChange={(event) => setApiserverUrl(event.target.value)}
            placeholder={i18next.t("machine:Apiserver URL placeholder")}
          />
        </Field>
      </div>

      <div className="flex flex-wrap gap-2">
        <Button variant="outline" onClick={handlePreflight} loading={preflighting}>
          <FileSearch />
          {i18next.t("machine:Preflight")}
        </Button>
        <Button onClick={() => handleDeploy(false)} loading={deploying}>
          <CloudCog />
          {i18next.t("machine:Deploy Node")}
        </Button>
        <Button variant="outline" onClick={() => handleDeploy(true)} loading={repairing}>
          <Wrench />
          {i18next.t("machine:Repair Node")}
        </Button>
        <Button variant="outline" onClick={() => loadTasks()} loading={loadingTasks}>
          <RefreshCw />
          {i18next.t("general:Refresh")}
        </Button>
      </div>

      {preflightError ? <MessageAlert title={i18next.t("machine:Preflight failed")} description={preflightError} /> : null}

      {preflight ? (
        <div className="bg-muted/40 rounded-lg border p-3">
          <DescriptionList
            columns={3}
            items={[
              {key: "node", label: "Node", value: preflight.nodeName || "-"},
              {key: "apiserver", label: "Apiserver", value: preflight.apiserverUrl || "-"},
              {key: "os", label: "OS", value: preflightData.os},
              {key: "arch", label: "Arch", value: preflightData.arch},
              {key: "systemd", label: "systemd", value: preflightData.systemd ? "yes" : "no"},
              {key: "package", label: "Package", value: preflightData.packageTool},
              {key: "sudo", label: "sudo", value: preflightData.canSudo ? "yes" : "no"},
              {key: "wsl", label: "WSL", value: preflightData.wsl ? "yes" : "no"},
            ]}
          />
        </div>
      ) : null}

      <DataTable
        columns={columns}
        dataSource={tasks}
        rowKey="id"
        loading={loadingTasks}
        pageSize={5}
        dense
        emptyText="No deployment tasks yet"
        onRowClick={(record) => {
          if (selectedTaskId !== record.id) {
            loadLogs(record.id);
          }
        }}
      />

      {selectedTask?.errorMsg ? (
        <MessageAlert title={i18next.t("machine:Node deployment failed")} description={selectedTask.errorMsg} />
      ) : null}

      <div>
        <div className="mb-2 text-sm font-semibold">{i18next.t("machine:Node deployment logs")}</div>
        {logs.length === 0 ? (
          <p className="text-muted-foreground text-sm">{i18next.t("machine:No node deployment logs")}</p>
        ) : (
          <div className="bg-muted/40 scrollbar-thin max-h-72 overflow-auto rounded-lg border p-3">
            {logs.map((log) => (
              <div key={log.id} className="font-mono text-xs leading-6 whitespace-pre-wrap">
                <span className="text-muted-foreground">{log.createdAt}</span>{" "}
                <Badge variant={log.level === "error" ? "danger" : "info"}>{log.level}</Badge> {log.message}
              </div>
            ))}
          </div>
        )}
      </div>
    </ResourceSheet>
  );
}

export default MachineNodeDeploySheet;
