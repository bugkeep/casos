import React from "react";
import {Alert, Button, Space, Table, Tag, Typography} from "antd";
import {ReloadOutlined} from "@ant-design/icons";
import * as PodBackend from "./backend/PodBackend";
import * as Setting from "./Setting";

const {Title} = Typography;

const phaseColor = {
  Running: "green",
  Pending: "gold",
  Succeeded: "blue",
  Failed: "red",
  Unknown: "default",
};

const columns = [
  {
    title: "Namespace",
    dataIndex: "namespace",
    key: "namespace",
    width: 180,
  },
  {
    title: "Name",
    dataIndex: "name",
    key: "name",
  },
  {
    title: "Node",
    dataIndex: "nodeName",
    key: "nodeName",
    width: 200,
    render: (v) => v || <span style={{color: "#999"}}>—</span>,
  },
  {
    title: "Phase",
    dataIndex: "phase",
    key: "phase",
    width: 120,
    render: (phase) => (
      <Tag color={phaseColor[phase] ?? "default"}>{phase || "Unknown"}</Tag>
    ),
  },
];

class PodListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      pods: [],
      loading: true,
      error: null,
    };
  }

  componentDidMount() {
    this.getPods();
  }

  getPods() {
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

  render() {
    const {pods, loading, error} = this.state;

    return (
      <div>
        <Space style={{marginBottom: 16}}>
          <Title level={4} style={{margin: 0}}>Pods</Title>
          <Button
            icon={<ReloadOutlined />}
            onClick={() => this.getPods()}
            loading={loading}
            size="small"
          >
            Refresh
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
          rowKey={(r) => `${r.namespace}/${r.name}`}
          columns={columns}
          dataSource={pods}
          loading={loading}
          size="middle"
          pagination={{pageSize: 20}}
          locale={{emptyText: "No pods found"}}
        />
      </div>
    );
  }
}

export default PodListPage;
