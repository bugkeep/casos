import React from "react";
import {Alert, Collapse, Divider, Form, Input, InputNumber, Modal, Select, Space, Tag, Tooltip, Typography} from "antd";
import {InfoCircleOutlined, LockOutlined} from "@ant-design/icons";
import i18next from "i18next";
import * as AppBackend from "./backend/AppBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import EnvVarEditor from "./EnvVarEditor";

const {Text} = Typography;

function t(key, opts) {
  return i18next.t(key, opts);
}

// Keys whose values are likely sensitive (API keys, passwords, tokens, secrets)
const SENSITIVE_RE = /key|secret|password|token|credential|auth/i;

function isSensitive(name) {
  return SENSITIVE_RE.test(name);
}

function editorRowsToPayload(rows = []) {
  return rows.filter(e => e.name).map(e => ({name: e.name, value: e.value ?? ""}));
}

function InputField({input}) {
  const sensitive = isSensitive(input.name);
  const Field = sensitive ? Input.Password : Input;
  const suffix = sensitive ? (
    <Tooltip title={t("appStore:Sensitive value hint")}>
      <LockOutlined style={{color: "rgba(0,0,0,0.35)"}} />
    </Tooltip>
  ) : undefined;

  return (
    <Form.Item
      key={input.name}
      label={
        <span>
          {input.name}
          {input.description && (
            <Tooltip title={input.description}>
              <InfoCircleOutlined style={{marginLeft: 6, color: "rgba(0,0,0,0.35)"}} />
            </Tooltip>
          )}
        </span>
      }
      name={["inputs", input.name]}
      initialValue={input.default ?? ""}
      rules={input.required ? [{required: true, message: `${input.name} is required`}] : []}
    >
      <Field placeholder={input.default || undefined} suffix={!sensitive ? suffix : undefined} />
    </Form.Item>
  );
}

class DeployAppModal extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      namespaces: [],
      envVars: [],
      submitting: false,
      result: null,
      error: null,
    };
    this.formRef = React.createRef();
  }

  componentDidUpdate(prevProps) {
    if (this.props.open && !prevProps.open && this.props.template) {
      this.setState({result: null, error: null, envVars: []});
      this.fetchNamespaces();
    }
  }

  fetchNamespaces() {
    NamespaceBackend.getNamespaces().then(res => {
      if (res.status === "ok") {
        const namespaces = res.data ?? [];
        this.setState({namespaces});
        const defaultNs = namespaces.length > 0 ? namespaces[0].name : "default";
        const tpl = this.props.template;
        setTimeout(() => {
          this.formRef.current?.setFieldsValue({
            namespace: defaultNs,
            name: tpl?.name ?? "",
            image: tpl?.image ?? "",
            replicas: 1,
            serviceType: "ClusterIP",
          });
        }, 0);
      }
    }).catch(() => {});
  }

  handleSubmit() {
    this.formRef.current?.validateFields().then(values => {
      const tpl = this.props.template;

      // Merge template inputs into env vars (inputs take precedence, empties are skipped)
      const inputEnvVars = Object.entries(values.inputs ?? {})
        .filter(([, v]) => v !== "" && v !== null && v !== undefined)
        .map(([k, v]) => ({name: k, value: String(v)}));

      const payload = {
        namespace: values.namespace,
        name: values.name,
        image: values.image,
        replicas: values.replicas ?? 1,
        ports: (tpl?.ports ?? []).map((p, i) => ({name: `port-${i}`, containerPort: p, protocol: "TCP"})),
        envVars: [...inputEnvVars, ...editorRowsToPayload(this.state.envVars)],
        serviceType: values.serviceType,
      };

      this.setState({submitting: true, error: null});
      AppBackend.deployApp(payload)
        .then(res => {
          if (res.status === "ok") {
            this.setState({result: res.data});
          } else {
            this.setState({error: res.msg});
          }
        })
        .catch(e => this.setState({error: e.message}))
        .finally(() => this.setState({submitting: false}));
    });
  }

  handleClose() {
    this.setState({result: null, error: null, envVars: []});
    this.props.onClose?.();
  }

  renderInputs() {
    const {template} = this.props;
    const inputs = template?.inputs ?? [];
    if (inputs.length === 0) {return null;}

    const required = inputs.filter(i => i.required);
    const optional = inputs.filter(i => !i.required);

    return (
      <>
        <Divider orientation="left" orientationMargin={0} style={{marginTop: 4, marginBottom: 12}}>
          <Text style={{fontSize: 13}}>{t("appStore:App config")}</Text>
        </Divider>

        {required.length > 0 && (
          <>
            <div style={{marginBottom: 8, fontSize: 12, color: "rgba(0,0,0,0.45)"}}>
              {t("appStore:App config desc")}
            </div>
            {required.map(inp => <InputField key={inp.name} input={inp} />)}
          </>
        )}

        {optional.length > 0 && (
          <Collapse
            ghost
            size="small"
            style={{marginBottom: 8}}
            items={[{
              key: "optional",
              label: <Text style={{fontSize: 13}}>{t("appStore:Optional settings")}</Text>,
              children: optional.map(inp => <InputField key={inp.name} input={inp} />),
            }]}
          />
        )}
      </>
    );
  }

  renderResult() {
    const {result} = this.state;
    if (!result) {return null;}
    const svc = result.service;
    return (
      <Alert
        type="success"
        showIcon
        message={t("appStore:Deploy success")}
        description={
          <div>
            <div>
              Deployment <Text code>{result.deployment.name}</Text> {t("appStore:Deployment started")}
            </div>
            {svc && (
              <div style={{marginTop: 6}}>
                {t("appStore:Service info prefix")} <Text code>{svc.name}</Text> {t("appStore:Service info suffix")}
                {svc.ports?.map(p => (
                  <Tag key={p.name} style={{marginLeft: 4}}>{svc.name}:{p.port}</Tag>
                ))}
              </div>
            )}
          </div>
        }
        style={{marginTop: 16}}
      />
    );
  }

  render() {
    const {open, template} = this.props;
    const {namespaces, envVars, submitting, result, error} = this.state;
    if (!template) {return null;}

    const nsOptions = namespaces.map(ns => ({label: ns.name, value: ns.name}));
    const isDone = !!result;

    return (
      <Modal
        title={
          <Space>
            {template.icon && (
              <img src={template.icon} alt="" style={{width: 24, height: 24, objectFit: "contain"}}
                onError={e => {e.target.style.display = "none";}} />
            )}
            <span>{t("appStore:Deploy app title", {title: template.title})}</span>
          </Space>
        }
        open={open}
        onOk={isDone ? () => this.handleClose() : () => this.handleSubmit()}
        onCancel={() => this.handleClose()}
        okText={isDone ? t("appStore:Done") : t("appStore:Deploy")}
        cancelButtonProps={isDone ? {style: {display: "none"}} : {}}
        confirmLoading={submitting}
        width={620}
        destroyOnHidden
      >
        {error && (
          <Alert type="error" message={error} showIcon style={{marginBottom: 16}} />
        )}

        {!isDone && (
          <Form ref={this.formRef} layout="vertical">
            <Form.Item
              label={t("general:Namespaces")}
              name="namespace"
              rules={[{required: true}]}
            >
              <Select options={nsOptions} showSearch placeholder={t("general:Namespaces")} />
            </Form.Item>
            <Form.Item
              label={t("appStore:App name")}
              name="name"
              rules={[
                {required: true, message: t("appStore:App name required")},
                {pattern: /^[a-z0-9][a-z0-9-]*$/, message: t("appStore:App name pattern")},
              ]}
            >
              <Input placeholder={t("appStore:App name placeholder")} />
            </Form.Item>
            <Form.Item
              label={t("general:Image")}
              name="image"
              rules={[{required: true, message: t("appStore:Image required")}]}
            >
              <Input />
            </Form.Item>
            <Form.Item label={t("appStore:Replicas")} name="replicas" rules={[{required: true}]}>
              <InputNumber min={1} max={20} style={{width: "100%"}} />
            </Form.Item>
            <Form.Item label={t("appStore:Service type")} name="serviceType">
              <Select options={[
                {label: t("appStore:ClusterIP desc"), value: "ClusterIP"},
                {label: t("appStore:NodePort desc"), value: "NodePort"},
              ]} />
            </Form.Item>

            {template.ports?.length > 0 && (
              <Form.Item label={t("appStore:Ports")}>
                <Space wrap>
                  {template.ports.map(p => <Tag key={p}>:{p}/TCP</Tag>)}
                </Space>
              </Form.Item>
            )}

            {this.renderInputs()}

            <Divider orientation="left" orientationMargin={0} style={{marginTop: 4, marginBottom: 12}}>
              <Text style={{fontSize: 13}}>{t("appStore:Env vars")}</Text>
            </Divider>
            <EnvVarEditor
              value={envVars}
              onChange={rows => this.setState({envVars: rows})}
              configMaps={[]}
              secrets={[]}
            />
          </Form>
        )}

        {this.renderResult()}
      </Modal>
    );
  }
}

export default DeployAppModal;
