import React from "react";
import {Button, Form, Input, Result} from "antd";
import {LockOutlined, UserOutlined} from "@ant-design/icons";
import * as AccountBackend from "./backend/AccountBackend";
import * as Setting from "./Setting";

export function resolveSigninMode(options) {
  if (options?.casdoorConfigured && options?.authConfig?.serverUrl) {
    return "casdoor";
  }
  if (options?.localSigninAvailable) {
    if (options.localAdminInitialized === true) {
      return "local";
    }
    if (options.localAdminInitialized === false) {
      return "setup";
    }
  }
  return "unavailable";
}

export function resolveSigninRedirect(from) {
  try {
    const target = new URL(from || "/", window.location.origin);
    if (!from?.startsWith("/") || from.startsWith("//") || target.origin !== window.location.origin) {
      return "/";
    }
    return `${target.pathname}${target.search}${target.hash}`;
  } catch {
    return "/";
  }
}

class SigninPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {submitting: false};
  }

  componentDidMount() {
    if (resolveSigninMode(this.props.options) === "casdoor") {
      window.location.replace(Setting.getSigninUrl());
    }
  }

  async signin(values) {
    this.setState({submitting: true});
    try {
      const res = await AccountBackend.signinWithPassword(values.username, values.password);
      if (res.status !== "ok") {
        Setting.showMessage("error", res.msg);
        return;
      }
      this.finishSignin();
    } catch {
      Setting.showMessage("error", "Sign-in failed. Please try again.");
    } finally {
      this.setState({submitting: false});
    }
  }

  async initializeLocalAdmin(values) {
    this.setState({submitting: true});
    try {
      const res = await AccountBackend.initializeLocalAdmin(values.password);
      if (res.status !== "ok") {
        Setting.showMessage("error", res.msg);
        return;
      }
      this.finishSignin();
    } catch {
      Setting.showMessage("error", "Administrator setup failed. Please try again.");
    } finally {
      this.setState({submitting: false});
    }
  }

  finishSignin() {
    const from = sessionStorage.getItem("from") || "/";
    sessionStorage.removeItem("from");
    Setting.goToLink(resolveSigninRedirect(from));
  }

  renderSetupForm() {
    return (
      <Form initialValues={{username: "admin"}} onFinish={(values) => this.initializeLocalAdmin(values)} requiredMark={false}>
        <Form.Item name="username">
          <Input prefix={<UserOutlined />} disabled size="large" />
        </Form.Item>
        <Form.Item
          name="password"
          rules={[
            {required: true, message: "Please enter an administrator password"},
            {min: 12, message: "Use at least 12 characters"},
          ]}
        >
          <Input.Password prefix={<LockOutlined />} placeholder="Password" autoComplete="new-password" autoFocus size="large" />
        </Form.Item>
        <Form.Item
          name="confirmPassword"
          dependencies={["password"]}
          rules={[
            {required: true, message: "Please confirm the administrator password"},
            ({getFieldValue}) => ({
              validator(_, value) {
                return !value || getFieldValue("password") === value
                  ? Promise.resolve()
                  : Promise.reject(new Error("Passwords do not match"));
              },
            }),
          ]}
        >
          <Input.Password prefix={<LockOutlined />} placeholder="Confirm password" autoComplete="new-password" size="large" />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={this.state.submitting} block size="large">
          Create administrator
        </Button>
      </Form>
    );
  }

  render() {
    const mode = resolveSigninMode(this.props.options);
    if (mode === "casdoor") {
      return null;
    }
    if (mode === "unavailable") {
      return <Result status="warning" title="Sign-in unavailable" />;
    }

    return (
      <div style={{display: "flex", alignItems: "center", justifyContent: "center", minHeight: "100vh", padding: "24px"}}>
        <div style={{width: "100%", maxWidth: "340px"}}>
          <h1 style={{fontSize: "28px", textAlign: "center", margin: "0 0 32px"}}>CasOS</h1>
          {mode === "setup" ? this.renderSetupForm() : (
            <Form onFinish={(values) => this.signin(values)} requiredMark={false}>
              <Form.Item name="username" rules={[{required: true, message: "Please enter your username"}]}>
                <Input prefix={<UserOutlined />} placeholder="Username" autoComplete="username" autoFocus size="large" />
              </Form.Item>
              <Form.Item name="password" rules={[{required: true, message: "Please enter your password"}]}>
                <Input.Password prefix={<LockOutlined />} placeholder="Password" autoComplete="current-password" size="large" />
              </Form.Item>
              <Button type="primary" htmlType="submit" loading={this.state.submitting} block size="large">
                Sign in
              </Button>
            </Form>
          )}
        </div>
      </div>
    );
  }
}

export default SigninPage;
