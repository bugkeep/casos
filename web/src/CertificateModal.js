import React, {useEffect, useRef, useState} from "react";
import {Alert, Button, Form, Input, InputNumber, Modal, Space, Tabs, Typography} from "antd";
import {CheckCircleOutlined, ClockCircleOutlined, ExclamationCircleOutlined, LockOutlined, SyncOutlined} from "@ant-design/icons";
import * as CertificateBackend from "./backend/CertificateBackend";
import * as Setting from "./Setting";

const {Text} = Typography;

const STATUS_ICONS = {
  issued: <CheckCircleOutlined style={{color: "#52c41a"}} />,
  verifying: <SyncOutlined spin style={{color: "#1677ff"}} />,
  pending: <SyncOutlined spin style={{color: "#1677ff"}} />,
  failed: <ExclamationCircleOutlined style={{color: "#ff4d4f"}} />,
  none: <ClockCircleOutlined style={{color: "#bbb"}} />,
};

function CertificateModal({ingress, open, onClose, onUpdated}) {
  const [uploadForm] = Form.useForm();
  const [leForm] = Form.useForm();
  const [activeTab, setActiveTab] = useState("le");
  const [submitting, setSubmitting] = useState(false);
  const [certStatus, setCertStatus] = useState(null);
  const pollTimer = useRef(null);

  const domain = (ingress?.rules ?? [])[0]?.host ?? "";

  useEffect(() => {
    if (!open || !ingress) {return;}
    setActiveTab("le");
    uploadForm.resetFields();
    leForm.setFieldsValue({
      domain: domain,
      casosServiceName: "",
      casosServicePort: 9000,
    });
    fetchStatus();
  }, [open, ingress]);

  useEffect(() => {
    return () => clearInterval(pollTimer.current);
  }, []);

  function fetchStatus() {
    if (!ingress) {return;}
    CertificateBackend.getCertStatus(ingress.namespace, ingress.name)
      .then(res => {
        if (res.status === "ok") {
          setCertStatus(res.data);
          if (res.data?.status === "pending" || res.data?.status === "verifying") {
            startPolling();
          } else {
            clearInterval(pollTimer.current);
          }
        }
      })
      .catch(() => {});
  }

  function startPolling() {
    clearInterval(pollTimer.current);
    pollTimer.current = setInterval(fetchStatus, 4000);
  }

  function handleUpload() {
    uploadForm.validateFields().then(values => {
      setSubmitting(true);
      CertificateBackend.uploadCert({
        namespace: ingress.namespace,
        ingressName: ingress.name,
        certPEM: values.certPEM.trim(),
        keyPEM: values.keyPEM.trim(),
      }).then(res => {
        if (res.status === "ok") {
          Setting.showMessage("success", "Certificate uploaded and applied");
          setCertStatus(res.data);
          onUpdated?.();
        } else {
          Setting.showMessage("error", res.msg);
        }
      }).catch(e => Setting.showMessage("error", e.message))
        .finally(() => setSubmitting(false));
    });
  }

  function handleRequestLE() {
    leForm.validateFields().then(values => {
      setSubmitting(true);
      CertificateBackend.requestLECert({
        namespace: ingress.namespace,
        ingressName: ingress.name,
        domain: values.domain,
        casosServiceName: values.casosServiceName || undefined,
        casosServicePort: values.casosServicePort || 9000,
      }).then(res => {
        if (res.status === "ok") {
          Setting.showMessage("success", "Certificate request started — this may take up to 2 minutes");
          setCertStatus({status: "pending"});
          startPolling();
        } else {
          Setting.showMessage("error", res.msg);
        }
      }).catch(e => Setting.showMessage("error", e.message))
        .finally(() => setSubmitting(false));
    });
  }

  function renderStatusBadge() {
    if (!certStatus || certStatus.status === "none") {return null;}
    const icon = STATUS_ICONS[certStatus.status] ?? STATUS_ICONS.none;
    const labels = {
      issued: `Certificate active — expires ${certStatus.expiry ?? "unknown"}`,
      verifying: "Verifying domain ownership with Let's Encrypt…",
      pending: "Certificate request queued…",
      failed: `Failed: ${certStatus.error ?? "unknown error"}`,
    };
    const colors = {issued: "success", verifying: "processing", pending: "processing", failed: "error"};
    return (
      <Alert
        icon={icon}
        type={colors[certStatus.status] ?? "info"}
        message={labels[certStatus.status] ?? certStatus.status}
        showIcon
        style={{marginBottom: 16}}
      />
    );
  }

  const leTab = (
    <div>
      <Alert
        type="info"
        showIcon
        style={{marginBottom: 16}}
        message="Requirements"
        description={
          <ul style={{margin: 0, paddingLeft: 16}}>
            <li>The domain must be publicly accessible on port 80 (via your Ingress controller).</li>
            <li>casos needs its own k8s Service so Let&apos;s Encrypt can reach the challenge endpoint.</li>
          </ul>
        }
      />
      <Form form={leForm} layout="vertical">
        <Form.Item
          label="Domain"
          name="domain"
          rules={[{required: true, message: "Domain is required"}]}
          extra="The public domain to issue the certificate for"
        >
          <Input placeholder="myapp.example.com" prefix="http://" />
        </Form.Item>
        <Form.Item
          label="casos k8s Service Name"
          name="casosServiceName"
          extra="The Kubernetes Service that exposes the casos server (leave blank to use the value from app.conf)"
        >
          <Input placeholder="casos" />
        </Form.Item>
        <Form.Item
          label="casos Service Port"
          name="casosServicePort"
          extra="Port of the casos Service (default 9000)"
        >
          <InputNumber min={1} max={65535} style={{width: 120}} />
        </Form.Item>
      </Form>
      <Button
        type="primary"
        icon={<LockOutlined />}
        loading={submitting}
        onClick={handleRequestLE}
        disabled={certStatus?.status === "pending" || certStatus?.status === "verifying"}
      >
        Request Free Certificate
      </Button>
    </div>
  );

  const uploadTab = (
    <Form form={uploadForm} layout="vertical">
      <Form.Item
        label="Certificate (PEM)"
        name="certPEM"
        rules={[
          {required: true, message: "Certificate PEM is required"},
          {
            validator: (_, v) =>
              v && v.trim().includes("-----BEGIN CERTIFICATE-----")
                ? Promise.resolve()
                : Promise.reject("Must be a valid PEM certificate"),
          },
        ]}
      >
        <Input.TextArea
          rows={8}
          placeholder={"-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"}
          style={{fontFamily: "monospace", fontSize: 12}}
        />
      </Form.Item>
      <Form.Item
        label="Private Key (PEM)"
        name="keyPEM"
        rules={[
          {required: true, message: "Private key PEM is required"},
          {
            validator: (_, v) =>
              v && v.trim().includes("PRIVATE KEY")
                ? Promise.resolve()
                : Promise.reject("Must be a valid PEM private key"),
          },
        ]}
      >
        <Input.TextArea
          rows={8}
          placeholder={"-----BEGIN EC PRIVATE KEY-----\n...\n-----END EC PRIVATE KEY-----"}
          style={{fontFamily: "monospace", fontSize: 12}}
        />
      </Form.Item>
      <Button type="primary" icon={<LockOutlined />} loading={submitting} onClick={handleUpload}>
        Upload & Apply Certificate
      </Button>
    </Form>
  );

  return (
    <Modal
      title={
        <Space>
          <LockOutlined style={{color: "#1677ff"}} />
          <span>Manage HTTPS — <Text code>{ingress?.name ?? ""}</Text></span>
        </Space>
      }
      open={open}
      onCancel={() => {
        clearInterval(pollTimer.current);
        onClose();
      }}
      footer={null}
      width={580}
      destroyOnHidden
    >
      {renderStatusBadge()}

      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        items={[
          {key: "le", label: "Let's Encrypt (auto)", children: leTab},
          {key: "upload", label: "Upload Certificate", children: uploadTab},
        ]}
      />
    </Modal>
  );
}

export default CertificateModal;
