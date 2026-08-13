import React, {useEffect} from "react";
import {Button, Form, Input, Result, Typography} from "antd";
import {LockOutlined, LoginOutlined, UserOutlined} from "@ant-design/icons";
import * as AccountBackend from "./backend/AccountBackend";
import * as Setting from "./Setting";
import "./AuthPage.less";

const {Title} = Typography;

const passwordRules = [
  {required: true},
  {validator: (_, value) => {
    const bytes = value ? new TextEncoder().encode(value).length : 0;
    return !value || (bytes >= 12 && bytes <= 72) ? Promise.resolve() : Promise.reject(new Error("Use 12 to 72 bytes"));
  }},
];

function SigninPage({mode, auth, onAuthenticated}) {
  const [form] = Form.useForm();
  const [submitting, setSubmitting] = React.useState(false);
  const [error, setError] = React.useState("");

  useEffect(() => {
    if (mode === "signin" && auth.provider === "casdoor") {
      window.location.replace(Setting.getSigninUrl());
    }
  }, [auth.provider, mode]);

  if (mode === "error") {
    return <AuthShell><Result status="error" title="CasOS could not start" subTitle={auth.error} /></AuthShell>;
  }

  if (mode === "setup" && !auth.canSetup) {
    return (
      <AuthShell>
        <Result status="403" title="Setup unavailable" subTitle="Initial setup is restricted to a direct loopback connection." />
      </AuthShell>
    );
  }

  if (mode === "recover" && !auth.canRecover) {
    return (
      <AuthShell>
        <Result status="403" title="Recovery unavailable" subTitle="Password recovery is restricted to a direct loopback connection." />
      </AuthShell>
    );
  }
  if (mode === "signin" && auth.provider === "casdoor") {
    return <AuthShell><div className="auth-redirect"><LoginOutlined /><span>Redirecting to enterprise sign-in</span></div></AuthShell>;
  }

  const isSetup = mode === "setup";
  const isRecover = mode === "recover";
  const title = isSetup ? "Set up CasOS" : isRecover ? "Reset password" : "Sign in";

  const submit = async(values) => {
    setSubmitting(true);
    setError("");
    try {
      let response;
      if (isSetup) {
        response = await AccountBackend.setup(values.username, values.password);
      } else if (isRecover) {
        response = await AccountBackend.recover(values.password);
      } else {
        response = await AccountBackend.login(values.username, values.password);
      }
      if (response.status !== "ok") {
        setError(response.msg || "Authentication failed");
        return;
      }
      await onAuthenticated();
    } catch {
      setError("Unable to reach the CasOS service");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <AuthShell>
      <Title level={2}>{title}</Title>
      <Form form={form} layout="vertical" requiredMark={false} onFinish={submit} className="auth-form">
        {!isRecover && (
          <Form.Item label="Username" name="username" rules={[{required: true}, {min: 3, max: 64}]}>
            <Input prefix={<UserOutlined />} autoComplete={isSetup ? "username" : "current-username"} autoFocus />
          </Form.Item>
        )}
        <Form.Item label={isRecover ? "New password" : "Password"} name="password" rules={passwordRules}>
          <Input.Password prefix={<LockOutlined />} autoComplete={isSetup || isRecover ? "new-password" : "current-password"} autoFocus={isRecover} />
        </Form.Item>
        {(isSetup || isRecover) && (
          <Form.Item
            label="Confirm password"
            name="confirmPassword"
            dependencies={["password"]}
            rules={[
              {required: true},
              ({getFieldValue}) => ({validator(_, value) {
                return !value || getFieldValue("password") === value ? Promise.resolve() : Promise.reject(new Error("Passwords do not match"));
              }}),
            ]}>
            <Input.Password prefix={<LockOutlined />} autoComplete="new-password" />
          </Form.Item>
        )}
        {error && <div className="auth-error" role="alert">{error}</div>}
        <Button type="primary" htmlType="submit" loading={submitting} block icon={isSetup ? <UserOutlined /> : <LoginOutlined />}>
          {isSetup ? "Create administrator" : isRecover ? "Reset password" : "Sign in"}
        </Button>
        {!isSetup && !isRecover && auth.canRecover && <Button type="link" href="/recover" block>Forgot password</Button>}
      </Form>
    </AuthShell>
  );
}

function AuthShell({children}) {
  return (
    <main className="auth-page">
      <section className="auth-panel">
        <img src="/casos-logo.png" alt="CasOS" className="auth-logo" />
        {children}
      </section>
    </main>
  );
}

export default SigninPage;
