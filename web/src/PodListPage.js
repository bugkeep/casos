import React from "react";
import {
  Alert, Button, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, Typography,
} from "antd";
import {DeleteOutlined, EditOutlined, MinusCircleOutlined, PlusOutlined, ReloadOutlined} from "@ant-design/icons";
import * as PodBackend from "./backend/PodBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import * as Setting from "./Setting";

const {Title} = Typography;

const phaseColor = {
  Running: "green",
  Pending: "gold",
  Succeeded: "blue",
  Failed: "red",
  Unknown: "default",
};

class PodListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      pods: [],
      namespaces: [],
      loading: true,
      error: null,
      modalVisible: false,
      modalMode: "add",
      submitting: false,
      editingPod: null,
    };
    this.formRef = React.createRef();
  }

  componentDidMount() {
    this.fetchPods();
    this.fetchNamespaces();
  }

  fetchNamespaces() {
    NamespaceBackend.getNamespaces().then(res => {
      if (res.status === "ok") {
        this.setState({namespaces: res.data ?? []});
      }
    }).catch(() => {});
  }

  fetchPods() {
    this.setState({loading: true, error: null});
    PodBackend.getPods().then(res => {
      if (res.status === "ok") {
        this.setState({pods: res.data ?? []});
      } else {
        Setting.showMessage("error", res.msg);
        this.setState({error: res.msg});
      }
    }).catch(e => {
      Setting.showMessage("error", e.message);
      this.setState({error: e.message});
    }).finally(() => {
      this.setState({loading: false});
    });
  }

  openAddModal() {
    this.setState({modalVisible: true, modalMode: "add", editingPod: null}, () => {
      const defaultNs = this.state.namespaces.length > 0 ? this.state.namespaces[0].name : "default";
      this.formRef.current?.setFieldsValue({
        namespace: defaultNs,
        name: "",
        image: "",
        containerName: "app",
        labelEntries: [],
      });
    });
  }

  openEditModal(pod) {
    const labelEntries = Object.entries(pod.labels ?? {}).map(([key, value]) => ({key, value}));
    this.setState({modalVisible: true, modalMode: "edit", editingPod: pod}, () => {
      this.formRef.current?.setFieldsValue({
        namespace: pod.namespace,
        name: pod.name,
        image: pod.image,
        containerName: "",
        labelEntries,
      });
    });
  }

  closeModal() {
    this.setState({modalVisible: false, editingPod: null});
  }

  handleSubmit() {
    this.formRef.current?.validateFields().then(values => {
      const labels = {};
      (values.labelEntries ?? []).forEach(({key, value}) => {
        if (key) {
          labels[key] = value ?? "";
        }
      });

      this.setState({submitting: true});

      if (this.state.modalMode === "add") {
        PodBackend.addPod({
          namespace: values.namespace,
          name: values.name,
          image: values.image,
          containerName: values.containerName || "app",
          labels,
        }).then(res => {
          if (res.status === "ok") {
            Setting.showMessage("success", "Pod created (will be Pending without a worker node)");
            this.closeModal();
            this.fetchPods();
          } else {
            Setting.showMessage("error", res.msg);
          }
        }).catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      } else {
        const pod = this.state.editingPod;
        PodBackend.updatePod({
          namespace: pod.namespace,
          name: pod.name,
          labels,
          resourceVersion: pod.resourceVersion,
        }).then(res => {
          if (res.status === "ok") {
            Setting.showMessage("success", "Pod labels updated");
            this.closeModal();
            this.fetchPods();
          } else {
            Setting.showMessage("error", res.msg);
          }
        }).catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      }
    });
  }

  handleDelete(pod) {
    PodBackend.deletePod(pod.namespace, pod.name).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", "Pod deleted");
        this.fetchPods();
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message));
  }

  render() {
    const {pods, namespaces, loading, error, modalVisible, modalMode, submitting} = this.state;

    const nsOptions = namespaces.map(ns => ({label: ns.name, value: ns.name}));

    const columns = [
      {title: "Namespace", dataIndex: "namespace", key: "namespace", width: 140},
      {title: "Name", dataIndex: "name", key: "name"},
      {title: "Image", dataIndex: "image", key: "image"},
      {
        title: "Node",
        dataIndex: "nodeName",
        key: "nodeName",
        width: 160,
        render: v => v || <span style={{color: "#999"}}>—</span>,
      },
      {
        title: "Phase",
        dataIndex: "phase",
        key: "phase",
        width: 110,
        render: phase => (
          <Tag color={phaseColor[phase] ?? "default"}>{phase || "Unknown"}</Tag>
        ),
      },
      {title: "Created", dataIndex: "createdAt", key: "createdAt", width: 180},
      {
        title: "Actions",
        key: "actions",
        width: 140,
        render: (_, record) => (
          <Space>
            <Button
              size="small"
              icon={<EditOutlined />}
              onClick={() => this.openEditModal(record)}
            >
              Edit
            </Button>
            <Popconfirm
              title={`Delete Pod "${record.name}"?`}
              okText="Delete"
              okType="danger"
              cancelText="Cancel"
              onConfirm={() => this.handleDelete(record)}
            >
              <Button size="small" danger icon={<DeleteOutlined />}>Delete</Button>
            </Popconfirm>
          </Space>
        ),
      },
    ];

    return (
      <div>
        <Space style={{marginBottom: 16}}>
          <Title level={4} style={{margin: 0}}>Pods</Title>
          <Button
            icon={<ReloadOutlined />}
            onClick={() => this.fetchPods()}
            loading={loading}
            size="small"
          >
            Refresh
          </Button>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            size="small"
            onClick={() => this.openAddModal()}
          >
            Add
          </Button>
        </Space>

        {error && (
          <Alert
            type="error"
            message="Failed to fetch pods"
            description={error}
            style={{marginBottom: 16}}
            showIcon
          />
        )}

        <Table
          rowKey={r => `${r.namespace}/${r.name}`}
          columns={columns}
          dataSource={pods}
          loading={loading}
          size="middle"
          pagination={{pageSize: 20}}
          locale={{emptyText: "No pods found"}}
        />

        <Modal
          title={modalMode === "add" ? "Add Pod" : "Edit Pod Labels"}
          open={modalVisible}
          onOk={() => this.handleSubmit()}
          onCancel={() => this.closeModal()}
          confirmLoading={submitting}
          okText={modalMode === "add" ? "Create" : "Update"}
          width={580}
          destroyOnHidden
        >
          <Form ref={this.formRef} layout="vertical">
            <Form.Item
              label="Namespace"
              name="namespace"
              rules={[{required: true, message: "Namespace is required"}]}
            >
              <Select
                disabled={modalMode === "edit"}
                options={nsOptions}
                placeholder="Select a namespace"
                showSearch
              />
            </Form.Item>
            <Form.Item
              label="Name"
              name="name"
              rules={[{required: true, message: "Name is required"}]}
            >
              <Input disabled={modalMode === "edit"} placeholder="my-pod" />
            </Form.Item>
            <Form.Item
              label="Image"
              name="image"
              rules={modalMode === "add" ? [{required: true, message: "Image is required"}] : []}
            >
              <Input
                disabled={modalMode === "edit"}
                placeholder="nginx:latest"
              />
            </Form.Item>
            {modalMode === "add" && (
              <Form.Item label="Container Name" name="containerName">
                <Input placeholder="app" />
              </Form.Item>
            )}
            {modalMode === "edit" && (
              <div style={{marginBottom: 8, color: "#888", fontSize: 12}}>
                Note: pod spec (image, containers) is immutable after creation. Only labels can be updated.
              </div>
            )}

            <Form.List name="labelEntries">
              {(fields, {add, remove}) => (
                <>
                  <div style={{marginBottom: 8, fontWeight: 500}}>Labels</div>
                  {fields.map(({key, name, ...rest}) => (
                    <Space key={key} align="baseline" style={{display: "flex", marginBottom: 4}}>
                      <Form.Item
                        {...rest}
                        name={[name, "key"]}
                        rules={[{required: true, message: "Key required"}]}
                        style={{marginBottom: 0}}
                      >
                        <Input placeholder="key" style={{width: 180}} />
                      </Form.Item>
                      <Form.Item
                        {...rest}
                        name={[name, "value"]}
                        style={{marginBottom: 0}}
                      >
                        <Input placeholder="value" style={{width: 200}} />
                      </Form.Item>
                      <MinusCircleOutlined onClick={() => remove(name)} style={{color: "#ff4d4f", cursor: "pointer"}} />
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
      </div>
    );
  }
}

export default PodListPage;
