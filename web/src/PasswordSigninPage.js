import React from "react";
import {Button, Form, Input, Result, Spin, message} from "antd";
import {LockOutlined, UserOutlined} from "@ant-design/icons";
import i18next from "i18next";
import * as AccountBackend from "./backend/AccountBackend";
import * as Setting from "./Setting";

class PasswordSigninPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      loading: true,
      showSignin: false,
      errorMessage: "",
      autoSignin: false,
    };
  }

  componentDidMount() {
    AccountBackend.getSigninOptions()
      .then((res) => {
        if (res.status === "ok" && res.data?.casdoorAvailable) {
          Setting.initCasdoorSdk(res.data.authConfig);
          window.location.replace(Setting.getSigninUrl());
          return;
        }

        this.setState({
          loading: false,
          showSignin: res.status === "ok" && !res.data?.casdoorAvailable && res.data?.signinAvailable,
          errorMessage: res.status === "ok" ? "" : res.msg,
          autoSignin: res.status === "ok" && res.data?.autoSignin === true,
        });
      })
      .catch((error) => {
        this.setState({
          loading: false,
          showSignin: false,
          errorMessage: error.message,
        });
      });
  }

  onFinish(values) {
    AccountBackend.signinWithPassword(values.username, values.password)
      .then((res) => {
        if (res.status === "ok") {
          const from = sessionStorage.getItem("from") || "/";
          sessionStorage.removeItem("from");
          Setting.goToLink(from);
        } else {
          message.error(res.msg);
        }
      })
      .catch((error) => message.error(error.message));
  }

  render() {
    if (this.state.loading) {
      return (
        <div style={{display: "flex", alignItems: "center", justifyContent: "center", minHeight: "100vh"}}>
          <Spin size="large" tip={i18next.t("account:Signing in...")} />
        </div>
      );
    }

    if (!this.state.showSignin) {
      return (
        <Result
          status="warning"
          title={i18next.t("account:Sign in is unavailable")}
          subTitle={this.state.errorMessage || i18next.t("account:Sign in is unavailable - Tooltip")}
        />
      );
    }

    return (
      <div style={{display: "flex", alignItems: "center", justifyContent: "center", minHeight: "100vh"}}>
        <div style={{width: "340px", maxWidth: "100%", padding: "24px"}}>
          <div style={{textAlign: "center", marginBottom: "36px"}}>
            <img src={this.props.logo} alt="CasOS" style={{width: "260px", maxWidth: "100%"}} />
          </div>
          <Form initialValues={{username: "admin", password: this.state.autoSignin ? "123" : undefined}} onFinish={(values) => this.onFinish(values)} requiredMark={false}>
            <Form.Item name="username" rules={[{required: true, message: i18next.t("account:Please input your username")}]}>
              <Input prefix={<UserOutlined />} placeholder={i18next.t("general:Username")} autoComplete="username" size="large" />
            </Form.Item>
            <Form.Item name="password" rules={[{required: true, message: i18next.t("account:Please input your password")}]}>
              <Input.Password prefix={<LockOutlined />} placeholder={i18next.t("general:Password")} autoComplete="current-password" autoFocus size="large" />
            </Form.Item>
            <Button type="primary" htmlType="submit" block size="large">
              {i18next.t("account:Sign In")}
            </Button>
          </Form>
        </div>
      </div>
    );
  }
}

export default PasswordSigninPage;
