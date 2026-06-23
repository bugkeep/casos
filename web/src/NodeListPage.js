import React from "react";
import {
  Alert, Button, Drawer, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Switch, Table, Tag, Tooltip, Typography
} from "antd";

const {Text} = Typography;
import {
  CheckCircleOutlined, DeleteOutlined, EditOutlined, KeyOutlined, MinusCircleOutlined, PlusOutlined,
  ReloadOutlined, SettingOutlined, StopOutlined, ToolOutlined
} from "@ant-design/icons";
import * as NodeBackend from "./backend/NodeBackend";
import {buildManagedNodeNameSet, isManagedClusterNode} from "./managedNodeUtils";
import {
  shouldPollClusterNodesInBackground,
  shouldRefreshClusterNodesAfterManagedDeploy,
  shouldRefreshClusterNodesAfterManagedRemove,
  shouldRefreshClusterNodesAfterManagedRepair
} from "./managedNodeRefreshPlan";
import {
  getManagedNodesTableScrollX,
  managedNodeActionsColumnFixed,
  managedNodeActionsColumnWidth
} from "./managedNodeTableLayout";
import * as Setting from "./Setting";

const statusColor = {
  Ready: "green",
  NotReady: "red",
  Unknown: "default",
};

const managedStateColor = {
  pending: "default",
  deploying: "processing",
  ready: "green",
  degraded: "orange",
  repairing: "processing",
  failed: "red",
  removing: "orange",
  removed: "default",
};

function isPreflightSuccessful(result) {
  return Boolean(
    result?.reachable &&
    result?.isRoot &&
    result?.supportsOs &&
    result?.supportsArch &&
    result?.hasSystemd &&
    (!result?.isWsl || result?.windowsHost)
  );
}

const testIds = {
  clusterNodesTable: "cluster-nodes-table",
  managedNodesTable: "managed-nodes-table",
  autoDeployButton: "managed-node-auto-deploy-button",
  autoDeployModal: "managed-node-auto-deploy-modal",
  autoDeployNameInput: "managed-node-name-input",
  autoDeployHostInput: "managed-node-host-input",
  autoDeployPortInput: "managed-node-port-input",
  autoDeployUsernameInput: "managed-node-username-input",
  autoDeployPasswordInput: "managed-node-password-input",
  autoDeployPreflightButton: "managed-node-preflight-button",
  autoDeployPreflightResult: "managed-node-preflight-result",
};

class NodeListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      nodes: [],
      loading: true,
      error: null,
      managedNodes: [],
      managedLoading: true,
      managedError: null,
      editModalVisible: false,
      editingNode: null,
      submitting: false,
      kubeconfigModalVisible: false,
      kubeconfigNodeName: "",
      kubeconfigContent: "",
      kubeconfigLoading: false,
      deployModalVisible: false,
      deploySubmitting: false,
      preflightSubmitting: false,
      preflightResult: null,
      taskDrawerVisible: false,
      selectedManagedNode: null,
      selectedTaskId: null,
      selectedTasks: [],
      selectedLogs: [],
      taskLoading: false,
    };
    this.formRef = React.createRef();
    this.deployFormRef = React.createRef();
    this.managedRefreshTimer = null;
  }

  componentDidMount() {
    this.fetchNodes();
    this.fetchManagedNodes();
    this.managedRefreshTimer = window.setInterval(() => {
      if (shouldPollClusterNodesInBackground()) {
        this.fetchNodes(false);
      }
      this.fetchManagedNodes(false);
      if (this.state.taskDrawerVisible && this.state.selectedManagedNode) {
        this.loadNodeTasks(this.state.selectedManagedNode.id, false);
      }
    }, 5000);
  }

  componentWillUnmount() {
    if (this.managedRefreshTimer) {
      window.clearInterval(this.managedRefreshTimer);
    }
  }

  fetchNodes(showLoading = true) {
    if (showLoading) {
      this.setState({loading: true, error: null});
    }
    NodeBackend.getNodes().then(res => {
      if (res.status === "ok") {
        this.setState({nodes: res.data ?? [], error: null});
      } else {
        Setting.showMessage("error", res.msg);
        this.setState({error: res.msg});
      }
    }).catch(e => {
      Setting.showMessage("error", e.message);
      this.setState({error: e.message});
    }).finally(() => {
      if (showLoading) {
        this.setState({loading: false});
      }
    });
  }

  fetchManagedNodes(showLoading = true) {
    if (showLoading) {
      this.setState({managedLoading: true, managedError: null});
    }
    NodeBackend.getManagedNodes().then(res => {
      if (res.status === "ok") {
        this.setState({managedNodes: res.data ?? [], managedError: null});
      } else {
        Setting.showMessage("error", res.msg);
        this.setState({managedError: res.msg});
      }
    }).catch(e => {
      Setting.showMessage("error", e.message);
      this.setState({managedError: e.message});
    }).finally(() => {
      if (showLoading) {
        this.setState({managedLoading: false});
      }
    });
  }

  handleCordon(node, unschedulable) {
    NodeBackend.updateNode({
      name: node.name,
      labels: node.labels,
      unschedulable,
      resourceVersion: node.resourceVersion,
    }).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", unschedulable ? "Node cordoned" : "Node uncordoned");
        this.fetchNodes();
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message));
  }

  openEditModal(node) {
    const labelEntries = Object.entries(node.labels ?? {}).map(([key, value]) => ({key, value}));
    this.setState({editModalVisible: true, editingNode: node}, () => {
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({labelEntries});
      }, 0);
    });
  }

  closeEditModal() {
    this.setState({editModalVisible: false, editingNode: null});
  }

  handleEditSubmit() {
    this.formRef.current?.validateFields().then(values => {
      const labels = {};
      (values.labelEntries ?? []).forEach(({key, value}) => {
        if (key) {
          labels[key] = value ?? "";
        }
      });
      const node = this.state.editingNode;
      this.setState({submitting: true});
      NodeBackend.updateNode({
        name: node.name,
        labels,
        unschedulable: node.unschedulable,
        resourceVersion: node.resourceVersion,
      }).then(res => {
        if (res.status === "ok") {
          Setting.showMessage("success", "Node labels updated");
          this.closeEditModal();
          this.fetchNodes();
        } else {
          Setting.showMessage("error", res.msg);
        }
      }).catch(e => Setting.showMessage("error", e.message))
        .finally(() => this.setState({submitting: false}));
    });
  }

  handleDelete(name) {
    NodeBackend.deleteNode(name).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", "Node unregistered from cluster");
        this.fetchNodes();
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message));
  }

  openKubeconfigModal(nodeName) {
    this.setState({kubeconfigModalVisible: true, kubeconfigNodeName: nodeName, kubeconfigContent: "", kubeconfigLoading: true});
    NodeBackend.getWorkerKubeconfig(nodeName).then(res => {
      if (res.status === "ok") {
        this.setState({kubeconfigContent: res.data?.kubeconfig ?? ""});
      } else {
        Setting.showMessage("error", res.msg);
        this.setState({kubeconfigContent: ""});
      }
    }).catch(e => {
      Setting.showMessage("error", e.message);
      this.setState({kubeconfigContent: ""});
    }).finally(() => this.setState({kubeconfigLoading: false}));
  }

  closeKubeconfigModal() {
    this.setState({kubeconfigModalVisible: false, kubeconfigContent: ""});
  }

  copyKubeconfig() {
    navigator.clipboard.writeText(this.state.kubeconfigContent).then(() => {
      Setting.showMessage("success", "Copied to clipboard");
    });
  }

  openDeployModal() {
    this.setState({deployModalVisible: true}, () => {
      this.deployFormRef.current?.setFieldsValue({
        port: 22,
        username: "root",
        labelsJson: "{\"node-role.kubernetes.io/worker\":\"\"}",
        unschedulable: false,
      });
    });
  }

  closeDeployModal() {
    this.setState({deployModalVisible: false, preflightResult: null});
    this.deployFormRef.current?.resetFields();
  }

  runPreflight() {
    this.deployFormRef.current?.validateFields(["host", "port", "username", "password"]).then(values => {
      this.setState({preflightSubmitting: true, preflightResult: null});
      NodeBackend.preflightManagedNode({
        host: values.host,
        port: values.port,
        username: values.username,
        password: values.password,
      }).then(res => {
        const result = res.data ?? null;
        if (res.status === "ok") {
          this.setState({preflightResult: result});
          Setting.showMessage("success", "Managed node preflight passed");
        } else {
          this.setState({preflightResult: result});
          Setting.showMessage("error", res.msg);
        }
      }).catch(e => Setting.showMessage("error", e.message))
        .finally(() => this.setState({preflightSubmitting: false}));
    });
  }

  submitDeploy() {
    this.deployFormRef.current?.validateFields().then(values => {
      let labels = {};
      if (values.labelsJson && values.labelsJson.trim() !== "") {
        try {
          labels = JSON.parse(values.labelsJson);
        } catch (e) {
          Setting.showMessage("error", "Labels JSON is invalid");
          return;
        }
      }
      this.setState({deploySubmitting: true});
      NodeBackend.addManagedNode({
        name: values.name,
        host: values.host,
        port: values.port,
        username: values.username,
        password: values.password,
        labels,
        unschedulable: values.unschedulable,
      }).then(res => {
        if (res.status === "ok") {
          Setting.showMessage("success", "Managed node deployment queued");
          this.closeDeployModal();
          this.fetchManagedNodes();
          if (shouldRefreshClusterNodesAfterManagedDeploy()) {
            this.fetchNodes(false);
          }
        } else {
          Setting.showMessage("error", res.msg);
        }
      }).catch(e => Setting.showMessage("error", e.message))
        .finally(() => this.setState({deploySubmitting: false}));
    });
  }

  openTaskDrawer(node) {
    this.setState({
      taskDrawerVisible: true,
      selectedManagedNode: node,
      selectedTaskId: null,
      selectedTasks: [],
      selectedLogs: [],
    }, () => this.loadNodeTasks(node.id, true));
  }

  closeTaskDrawer() {
    this.setState({
      taskDrawerVisible: false,
      selectedManagedNode: null,
      selectedTaskId: null,
      selectedTasks: [],
      selectedLogs: [],
    });
  }

  loadNodeTasks(nodeId, showLoading = true) {
    if (showLoading) {
      this.setState({taskLoading: true});
    }
    NodeBackend.getNodeDeployTasks(nodeId).then(res => {
      if (res.status !== "ok") {
        Setting.showMessage("error", res.msg);
        return;
      }
      const tasks = res.data ?? [];
      const selectedTaskId = this.state.selectedTaskId ?? tasks[0]?.id ?? null;
      this.setState({selectedTasks: tasks, selectedTaskId}, () => {
        if (selectedTaskId) {
          this.loadTaskLogs(selectedTaskId, false);
        } else {
          this.setState({selectedLogs: []});
        }
      });
    }).catch(e => Setting.showMessage("error", e.message))
      .finally(() => {
        if (showLoading) {
          this.setState({taskLoading: false});
        }
      });
  }

  loadTaskLogs(taskId, showLoading = true) {
    if (showLoading) {
      this.setState({taskLoading: true});
    }
    NodeBackend.getNodeDeployLogs(taskId).then(res => {
      if (res.status === "ok") {
        this.setState({selectedLogs: res.data ?? [], selectedTaskId: taskId});
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message))
      .finally(() => {
        if (showLoading) {
          this.setState({taskLoading: false});
        }
      });
  }

  repairManagedNode(node) {
    NodeBackend.repairManagedNode(node.id).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", "Managed node repair queued");
        this.fetchManagedNodes();
        if (shouldRefreshClusterNodesAfterManagedRepair()) {
          this.fetchNodes(false);
        }
        this.setState({selectedTaskId: null}, () => this.loadNodeTasks(node.id, true));
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message));
  }

  removeManagedNode(node) {
    NodeBackend.removeManagedNode(node.id).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", "Managed node removed");
        this.fetchManagedNodes();
        if (shouldRefreshClusterNodesAfterManagedRemove()) {
          this.fetchNodes(false);
        }
        if (this.state.selectedManagedNode?.id === node.id) {
          this.closeTaskDrawer();
        }
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message));
  }

  render() {
    const {
      nodes, loading, error, managedNodes, managedLoading, managedError,
      editModalVisible, submitting, kubeconfigModalVisible, kubeconfigNodeName,
      kubeconfigContent, kubeconfigLoading, deployModalVisible, deploySubmitting,
      preflightSubmitting, preflightResult,
      taskDrawerVisible, selectedManagedNode, selectedTaskId, selectedTasks,
      selectedLogs, taskLoading,
    } = this.state;
    const managedNodeNameSet = buildManagedNodeNameSet(managedNodes);

    const nodeColumns = [
      {
        title: "Name",
        dataIndex: "name",
        key: "name",
        render: (name, record) => {
          const managed = isManagedClusterNode(name, managedNodeNameSet);

          return (
            <Space>
              {name}
              {managed && <Tag color="blue">Managed</Tag>}
              {record.unschedulable && <Tag color="orange">SchedulingDisabled</Tag>}
            </Space>
          );
        },
      },
      {
        title: "Status",
        dataIndex: "status",
        key: "status",
        width: 110,
        render: v => <Tag color={statusColor[v] ?? "default"}>{v}</Tag>,
      },
      {
        title: "Roles",
        dataIndex: "roles",
        key: "roles",
        width: 130,
        render: roles => (roles ?? []).map(r => <Tag key={r}>{r}</Tag>),
      },
      {title: "Kubelet", dataIndex: "kubeletVersion", key: "kubeletVersion", width: 120},
      {
        title: "OS / Arch",
        key: "osArch",
        width: 130,
        render: (_, r) => r.os ? `${r.os} / ${r.arch}` : "-",
      },
      {title: "Created", dataIndex: "createdAt", key: "createdAt", width: 180},
      {
        title: "Actions",
        key: "actions",
        width: 240,
        render: (_, record) => {
          const managed = isManagedClusterNode(record.name, managedNodeNameSet);
          const unregisterButton = (
            <Button size="small" danger icon={<DeleteOutlined />} disabled={managed}>Unregister</Button>
          );

          return (
            <Space>
              <Tooltip title={record.unschedulable ? "Uncordon (re-enable scheduling)" : "Cordon (disable scheduling)"}>
                <Button
                  size="small"
                  icon={record.unschedulable ? <CheckCircleOutlined /> : <StopOutlined />}
                  onClick={() => this.handleCordon(record, !record.unschedulable)}
                >
                  {record.unschedulable ? "Uncordon" : "Cordon"}
                </Button>
              </Tooltip>
              <Button
                size="small"
                icon={<EditOutlined />}
                onClick={() => this.openEditModal(record)}
              >
              Labels
              </Button>
              <Tooltip title="Generate kubeconfig for this node">
                <Button
                  size="small"
                  icon={<KeyOutlined />}
                  onClick={() => this.openKubeconfigModal(record.name)}
                >
                Kubeconfig
                </Button>
              </Tooltip>
              <Popconfirm
                title={`Unregister node "${record.name}" from cluster?`}
                description={managed
                  ? "This node is managed by CasOS. Use Managed Nodes -> Remove to stop services and remove the host cleanly."
                  : "This only removes the Kubernetes node object. The kubelet process keeps running and may register the node again. Use Managed Nodes -> Remove to stop a managed host."}
                okText="Unregister"
                okType="danger"
                cancelText="Cancel"
                onConfirm={() => this.handleDelete(record.name)}
                disabled={managed}
              >
                {managed ? (
                  <Tooltip title="This node is managed by CasOS. Use Managed Nodes -> Remove.">
                    <span>{unregisterButton}</span>
                  </Tooltip>
                ) : unregisterButton}
              </Popconfirm>
            </Space>
          );
        },
      },
    ];

    const managedColumns = [
      {title: "Name", dataIndex: "name", key: "name", width: 160},
      {title: "Host", dataIndex: "host", key: "host", width: 160},
      {
        title: "State",
        dataIndex: "state",
        key: "state",
        width: 120,
        render: value => <Tag color={managedStateColor[value] ?? "default"}>{value}</Tag>,
      },
      {
        title: "K8s",
        dataIndex: "kubernetesStatus",
        key: "kubernetesStatus",
        width: 120,
        render: value => <Tag color={statusColor[value] ?? "default"}>{value}</Tag>,
      },
      {
        title: "OS / Arch",
        key: "osArch",
        width: 130,
        render: (_, record) => record.os ? `${record.os} / ${record.arch}` : "-",
      },
      {title: "Version", dataIndex: "version", key: "version", width: 120, render: value => value || "-"},
      {
        title: "Last Error",
        dataIndex: "lastError",
        key: "lastError",
        ellipsis: true,
        render: value => value || "-",
      },
      {title: "Updated", dataIndex: "updatedAt", key: "updatedAt", width: 180},
      {
        title: "Actions",
        key: "actions",
        width: managedNodeActionsColumnWidth,
        fixed: managedNodeActionsColumnFixed,
        render: (_, record) => (
          <Space>
            <Button
              size="small"
              icon={<SettingOutlined />}
              onClick={() => this.openTaskDrawer(record)}
            >
              Tasks
            </Button>
            <Button
              size="small"
              icon={<ToolOutlined />}
              onClick={() => this.repairManagedNode(record)}
              disabled={record.state === "deploying" || record.state === "repairing"}
            >
              Repair
            </Button>
            <Popconfirm
              title={`Remove managed node "${record.name}"?`}
              description="This stops kubelet and kube-proxy on the managed host, removes the Kubernetes node, and stops CasOS from tracking it."
              okText="Remove"
              okType="danger"
              cancelText="Cancel"
              onConfirm={() => this.removeManagedNode(record)}
            >
              <Button
                size="small"
                danger
                icon={<DeleteOutlined />}
                disabled={record.state === "deploying" || record.state === "repairing" || record.state === "removing"}
              >
                Remove
              </Button>
            </Popconfirm>
          </Space>
        ),
      },
    ];

    const selectedTask = selectedTasks.find(task => task.id === selectedTaskId) ?? null;

    return (
      <div style={{padding: "24px"}}>
        {error && (
          <Alert
            type="error"
            message="Failed to fetch nodes"
            description={error}
            style={{marginBottom: 16}}
            showIcon
          />
        )}

        <div data-testid={testIds.clusterNodesTable}>
          <Table
            rowKey="name"
            columns={nodeColumns}
            dataSource={nodes}
            loading={loading}
            size="middle"
            pagination={{pageSize: 20}}
            locale={{emptyText: "No nodes registered. Start kubelet on a worker to join the cluster."}}
            title={() => (
              <div>
                <span style={{fontWeight: 600}}>Cluster Nodes</span>
                &nbsp;&nbsp;&nbsp;&nbsp;
                <Button
                  icon={<ReloadOutlined />}
                  onClick={() => this.fetchNodes()}
                  loading={loading}
                  size="small"
                >
                  Refresh
                </Button>
              </div>
            )}
          />
        </div>

        <div style={{height: 24}} />

        {managedError && (
          <Alert
            type="error"
            message="Failed to fetch managed nodes"
            description={managedError}
            style={{marginBottom: 16}}
            showIcon
          />
        )}

        <div data-testid={testIds.managedNodesTable}>
          <Table
            rowKey="id"
            columns={managedColumns}
            dataSource={managedNodes}
            loading={managedLoading}
            size="middle"
            scroll={{x: getManagedNodesTableScrollX()}}
            pagination={{pageSize: 10}}
            locale={{emptyText: "No managed nodes yet. Use Auto Deploy to bootstrap a worker host."}}
            title={() => (
              <div style={{display: "flex", justifyContent: "space-between", alignItems: "center"}}>
                <span style={{fontWeight: 600}}>Managed Nodes</span>
                <Space>
                  <Button
                    icon={<ReloadOutlined />}
                    onClick={() => this.fetchManagedNodes()}
                    loading={managedLoading}
                    size="small"
                  >
                    Refresh
                  </Button>
                  <Button
                    type="primary"
                    icon={<PlusOutlined />}
                    onClick={() => this.openDeployModal()}
                    size="small"
                    data-testid={testIds.autoDeployButton}
                  >
                    Auto Deploy
                  </Button>
                </Space>
              </div>
            )}
          />
        </div>

        <Modal
          title={`Edit Labels - ${this.state.editingNode?.name ?? ""}`}
          open={editModalVisible}
          onOk={() => this.handleEditSubmit()}
          onCancel={() => this.closeEditModal()}
          confirmLoading={submitting}
          okText="Save"
          width={560}
          destroyOnHidden
        >
          <Form ref={this.formRef} layout="vertical">
            <Form.List name="labelEntries">
              {(fields, {add, remove}) => (
                <>
                  <div style={{marginBottom: 8, fontWeight: 500}}>Labels (key=value)</div>
                  {fields.map(({key, name, ...rest}) => (
                    <Space key={key} align="baseline" style={{display: "flex", marginBottom: 4}}>
                      <Form.Item
                        {...rest}
                        name={[name, "key"]}
                        rules={[{required: true, message: "Key required"}]}
                        style={{marginBottom: 0}}
                      >
                        <Input placeholder="key" style={{width: 200}} />
                      </Form.Item>
                      <Form.Item
                        {...rest}
                        name={[name, "value"]}
                        style={{marginBottom: 0}}
                      >
                        <Input placeholder="value" style={{width: 200}} />
                      </Form.Item>
                      <MinusCircleOutlined
                        onClick={() => remove(name)}
                        style={{color: "#ff4d4f", cursor: "pointer"}}
                      />
                    </Space>
                  ))}
                  <Button
                    type="dashed"
                    onClick={() => add()}
                    icon={<PlusOutlined />}
                    style={{marginTop: 4}}
                    size="small"
                  >
                    Add Label
                  </Button>
                </>
              )}
            </Form.List>
          </Form>
        </Modal>

        <Modal
          title={`Worker Kubeconfig - ${kubeconfigNodeName}`}
          open={kubeconfigModalVisible}
          onCancel={() => this.closeKubeconfigModal()}
          footer={[
            <Button key="copy" type="primary" onClick={() => this.copyKubeconfig()} disabled={!kubeconfigContent}>
              Copy
            </Button>,
            <Button key="close" onClick={() => this.closeKubeconfigModal()}>Close</Button>,
          ]}
          width={680}
          destroyOnHidden
        >
          <Text type="secondary" style={{display: "block", marginBottom: 8}}>
            Save this as <Text code>/etc/kubernetes/worker.kubeconfig</Text> on the worker node, then start kubelet with <Text code>--kubeconfig=/etc/kubernetes/worker.kubeconfig</Text>.
          </Text>
          <Input.TextArea
            value={kubeconfigLoading ? "Loading..." : kubeconfigContent}
            readOnly
            autoSize={{minRows: 12, maxRows: 20}}
            style={{fontFamily: "monospace", fontSize: 12}}
          />
        </Modal>

        <Modal
          title="Auto Deploy Managed Node"
          open={deployModalVisible}
          onOk={() => this.submitDeploy()}
          onCancel={() => this.closeDeployModal()}
          confirmLoading={deploySubmitting}
          okText="Deploy"
          footer={[
            <Button
              key="preflight"
              onClick={() => this.runPreflight()}
              loading={preflightSubmitting}
              data-testid={testIds.autoDeployPreflightButton}
            >
              Preflight
            </Button>,
            <Button key="cancel" onClick={() => this.closeDeployModal()}>
              Cancel
            </Button>,
            <Button key="deploy" type="primary" onClick={() => this.submitDeploy()} loading={deploySubmitting}>
              Deploy
            </Button>,
          ]}
          width={620}
          destroyOnHidden
        >
          <div data-testid={testIds.autoDeployModal}>
            <Form ref={this.deployFormRef} layout="vertical">
              <Form.Item label="Node Name" required>
                <div data-testid={testIds.autoDeployNameInput}>
                  <Form.Item name="name" noStyle rules={[{required: true, message: "Node name is required"}]}>
                    <Input placeholder="worker-01" />
                  </Form.Item>
                </div>
              </Form.Item>
              <Form.Item label="Host IP / DNS" required>
                <div data-testid={testIds.autoDeployHostInput}>
                  <Form.Item name="host" noStyle rules={[{required: true, message: "Host is required"}]}>
                    <Input placeholder="192.168.1.20" />
                  </Form.Item>
                </div>
              </Form.Item>
              <Form.Item label="SSH Port" required>
                <div data-testid={testIds.autoDeployPortInput}>
                  <Form.Item name="port" noStyle rules={[{required: true, message: "Port is required"}]}>
                    <InputNumber min={1} max={65535} style={{width: "100%"}} />
                  </Form.Item>
                </div>
              </Form.Item>
              <Form.Item label="SSH Username" required>
                <div data-testid={testIds.autoDeployUsernameInput}>
                  <Form.Item name="username" noStyle rules={[{required: true, message: "Username is required"}]}>
                    <Input placeholder="root" />
                  </Form.Item>
                </div>
              </Form.Item>
              <Form.Item label="SSH Password" required>
                <div data-testid={testIds.autoDeployPasswordInput}>
                  <Form.Item name="password" noStyle rules={[{required: true, message: "Password is required"}]}>
                    <Input.Password placeholder="root password" />
                  </Form.Item>
                </div>
              </Form.Item>
              <Form.Item name="labelsJson" label="Node Labels JSON">
                <Input.TextArea autoSize={{minRows: 3, maxRows: 6}} />
              </Form.Item>
              <Form.Item name="unschedulable" label="Disable Scheduling" valuePropName="checked">
                <Switch />
              </Form.Item>
            </Form>
            {preflightResult && (
              <div data-testid={testIds.autoDeployPreflightResult}>
                <Alert
                  style={{marginTop: 12}}
                  type={isPreflightSuccessful(preflightResult) ? "success" : "warning"}
                  message="Preflight Result"
                  description={
                    <div>
                      <div>{preflightResult.message}</div>
                      <div style={{marginTop: 8}}>
                        <Space wrap>
                          <Tag color={preflightResult.reachable ? "green" : "red"}>SSH</Tag>
                          <Tag color={preflightResult.isRoot ? "green" : "red"}>root</Tag>
                          <Tag color={preflightResult.supportsOs ? "green" : "red"}>{preflightResult.os || "unknown os"}</Tag>
                          <Tag color={preflightResult.supportsArch ? "green" : "red"}>{preflightResult.arch || "unknown arch"}</Tag>
                          <Tag color={preflightResult.hasSystemd ? "green" : "red"}>systemd</Tag>
                          {preflightResult.initProcess && <Tag color={preflightResult.hasSystemd ? "green" : "orange"}>PID 1: {preflightResult.initProcess}</Tag>}
                          {preflightResult.systemdState && <Tag color={preflightResult.hasSystemd ? "green" : "orange"}>systemd: {preflightResult.systemdState}</Tag>}
                          {preflightResult.isWsl && <Tag color="blue">WSL</Tag>}
                          {preflightResult.isWsl && (
                            <Tag color={preflightResult.windowsHost ? "cyan" : "red"}>
                              Windows host: {preflightResult.windowsHost || "unresolved"}
                            </Tag>
                          )}
                          {preflightResult.version && <Tag>{preflightResult.version}</Tag>}
                        </Space>
                      </div>
                    </div>
                  }
                  showIcon
                />
              </div>
            )}
          </div>
        </Modal>

        <Drawer
          title={`Managed Node Tasks${selectedManagedNode ? ` - ${selectedManagedNode.name}` : ""}`}
          open={taskDrawerVisible}
          width={720}
          onClose={() => this.closeTaskDrawer()}
        >
          <Space direction="vertical" size={16} style={{display: "flex"}}>
            {selectedManagedNode && (
              <div>
                <Space wrap>
                  <Tag color={managedStateColor[selectedManagedNode.state] ?? "default"}>{selectedManagedNode.state}</Tag>
                  <Tag color={statusColor[selectedManagedNode.kubernetesStatus] ?? "default"}>{selectedManagedNode.kubernetesStatus}</Tag>
                  <Text type="secondary">{selectedManagedNode.host}:{selectedManagedNode.port}</Text>
                </Space>
                {selectedManagedNode.lastError && (
                  <Alert
                    type="warning"
                    message="Last Error"
                    description={selectedManagedNode.lastError}
                    showIcon
                    style={{marginTop: 12}}
                  />
                )}
              </div>
            )}

            <div>
              <div style={{fontWeight: 600, marginBottom: 8}}>Tasks</div>
              <Select
                style={{width: "100%"}}
                value={selectedTaskId}
                placeholder="Select a task"
                loading={taskLoading}
                onChange={(value) => this.loadTaskLogs(value, true)}
                options={selectedTasks.map(task => ({
                  value: task.id,
                  label: `#${task.id} ${task.type} - ${task.status} (${task.stage || "queued"})`,
                }))}
              />
            </div>

            {selectedTask && (
              <div>
                <div style={{fontWeight: 600, marginBottom: 8}}>Task Detail</div>
                <Space wrap>
                  <Tag>{selectedTask.type}</Tag>
                  <Tag color={managedStateColor[selectedTask.status] ?? "default"}>{selectedTask.status}</Tag>
                  <Tag color="blue">{selectedTask.stage || "queued"}</Tag>
                  <Tag>{selectedTask.progress}%</Tag>
                </Space>
                {selectedTask.errorMsg && (
                  <Alert
                    type="error"
                    message="Task Error"
                    description={selectedTask.errorMsg}
                    showIcon
                    style={{marginTop: 12}}
                  />
                )}
              </div>
            )}

            <div>
              <div style={{fontWeight: 600, marginBottom: 8}}>Logs</div>
              <Input.TextArea
                readOnly
                value={(selectedLogs ?? []).map(log => `[${log.createdAt}] ${log.level}: ${log.message}`).join("\n")}
                autoSize={{minRows: 16, maxRows: 24}}
                style={{fontFamily: "monospace", fontSize: 12}}
              />
            </div>
          </Space>
        </Drawer>
      </div>
    );
  }
}

export default NodeListPage;
