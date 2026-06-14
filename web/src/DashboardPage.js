import React, {useEffect, useState} from "react";
import {Card, Col, Row, Spin, Statistic} from "antd";
import {
  AppstoreOutlined,
  CheckCircleOutlined,
  ClusterOutlined,
  NodeIndexOutlined,
  SettingOutlined,
  UserOutlined
} from "@ant-design/icons";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Legend,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis
} from "recharts";
import * as DashboardBackend from "./backend/DashboardBackend";
import * as Setting from "./Setting";

const PHASE_COLORS = {
  Running: "#52c41a",
  Pending: "#faad14",
  Failed: "#ff4d4f",
  Succeeded: "#1677ff",
  Unknown: "#d9d9d9",
};

function DashboardPage() {
  const [stats, setStats] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    DashboardBackend.getDashboard().then(res => {
      setLoading(false);
      if (res.status === "ok") {
        setStats(res.data);
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(err => {
      setLoading(false);
      Setting.showMessage("error", `Failed to load dashboard: ${err}`);
    });
  }, []);

  if (loading) {
    return (
      <div style={{display: "flex", justifyContent: "center", alignItems: "center", height: 400}}>
        <Spin size="large" />
      </div>
    );
  }

  if (!stats) {return null;}

  const podPhaseData = Object.entries(stats.podsByPhase || {}).map(([name, value]) => ({name, value}));

  const resourceBarData = [
    {name: "Nodes", value: stats.nodesTotal},
    {name: "Pods", value: stats.podsTotal},
    {name: "Namespaces", value: stats.namespacesTotal},
    {name: "Services", value: stats.servicesTotal},
    {name: "ConfigMaps", value: stats.configMapsTotal},
    {name: "ServiceAccounts", value: stats.serviceAccounts},
  ];

  const nodeReady = stats.nodesReady;
  const nodeNotReady = stats.nodesTotal - stats.nodesReady;

  return (
    <div style={{padding: 24}}>
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} md={8} lg={4}>
          <Card>
            <Statistic
              title="Total Nodes"
              value={stats.nodesTotal}
              prefix={<ClusterOutlined />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={8} lg={4}>
          <Card>
            <Statistic
              title="Nodes Ready"
              value={nodeReady}
              valueStyle={{color: "#52c41a"}}
              prefix={<CheckCircleOutlined />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={8} lg={4}>
          <Card>
            <Statistic
              title="Total Pods"
              value={stats.podsTotal}
              prefix={<AppstoreOutlined />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={8} lg={4}>
          <Card>
            <Statistic
              title="Namespaces"
              value={stats.namespacesTotal}
              prefix={<SettingOutlined />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={8} lg={4}>
          <Card>
            <Statistic
              title="Services"
              value={stats.servicesTotal}
              prefix={<NodeIndexOutlined />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={8} lg={4}>
          <Card>
            <Statistic
              title="Service Accounts"
              value={stats.serviceAccounts}
              prefix={<UserOutlined />}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{marginTop: 16}}>
        <Col xs={24} md={12}>
          <Card title="Pod Phase Distribution">
            {podPhaseData.length === 0 ? (
              <div style={{textAlign: "center", padding: 40, color: "#999"}}>No pods found</div>
            ) : (
              <ResponsiveContainer width="100%" height={300}>
                <PieChart>
                  <Pie
                    data={podPhaseData}
                    cx="50%"
                    cy="50%"
                    outerRadius={100}
                    dataKey="value"
                    label={({name, value}) => `${name}: ${value}`}
                  >
                    {podPhaseData.map((entry, index) => (
                      <Cell key={`cell-${index}`} fill={PHASE_COLORS[entry.name] || "#8884d8"} />
                    ))}
                  </Pie>
                  <Tooltip />
                  <Legend />
                </PieChart>
              </ResponsiveContainer>
            )}
          </Card>
        </Col>

        <Col xs={24} md={12}>
          <Card title="Node Status">
            <ResponsiveContainer width="100%" height={300}>
              <PieChart>
                <Pie
                  data={[
                    {name: "Ready", value: nodeReady},
                    {name: "Not Ready", value: nodeNotReady},
                  ]}
                  cx="50%"
                  cy="50%"
                  outerRadius={100}
                  dataKey="value"
                  label={({name, value}) => value > 0 ? `${name}: ${value}` : ""}
                >
                  <Cell fill="#52c41a" />
                  <Cell fill="#ff4d4f" />
                </Pie>
                <Tooltip />
                <Legend />
              </PieChart>
            </ResponsiveContainer>
          </Card>
        </Col>

        <Col xs={24}>
          <Card title="Resource Overview">
            <ResponsiveContainer width="100%" height={260}>
              <BarChart data={resourceBarData} margin={{top: 8, right: 24, left: 0, bottom: 0}}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="name" />
                <YAxis allowDecimals={false} />
                <Tooltip />
                <Bar dataKey="value" fill="#1677ff" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </Card>
        </Col>
      </Row>
    </div>
  );
}

export default DashboardPage;
