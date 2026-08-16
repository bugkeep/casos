import React from "react";
import {Button, Result, Spin} from "antd";
import {withRouter} from "react-router-dom";
import i18next from "i18next";
import * as AccountBackend from "./backend/AccountBackend";
import * as Setting from "./Setting";

class AuthCallback extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      classes: props,
      msg: null,
    };
  }

  componentDidMount() {
    this.login();
  }

  getFromLink() {
    const from = sessionStorage.getItem("from");
    sessionStorage.removeItem("from");
    if (from === null) {
      return "/";
    }
    return from;
  }

  // The Casdoor SDK is configured by the server, so the callback has to load the
  // sign-in options before it can exchange the authorization code.
  async login() {
    try {
      if (!Setting.isCasdoorAvailable()) {
        const options = await AccountBackend.getSigninOptions();
        if (options.status !== "ok" || !options.data?.casdoorAvailable) {
          this.setState({msg: options.msg || i18next.t("account:Sign in is unavailable")});
          return;
        }
        Setting.initCasdoorSdk(options.data.authConfig);
      }

      const res = await Setting.signin();
      if (res.status === "ok") {
        Setting.showMessage("success", i18next.t("account:Logged in successfully"));
        Setting.goToLink(this.getFromLink());
      } else {
        this.setState({msg: res.msg});
      }
    } catch (error) {
      this.setState({msg: error.message});
    }
  }

  render() {
    return (
      <div style={{textAlign: "center"}}>
        {this.state.msg === null ? (
          <Spin
            size="large"
            tip={i18next.t("account:Signing in...")}
            style={{paddingTop: "10%"}}
          />
        ) : (
          <div style={{display: "inline"}}>
            <Result
              status="error"
              title={i18next.t("account:Sign in failed")}
              subTitle={this.state.msg}
              extra={
                <Button type="primary" onClick={() => Setting.goToLink("/signin")}>
                  {i18next.t("account:Back to sign in")}
                </Button>
              }
            />
          </div>
        )}
      </div>
    );
  }
}

export default withRouter(AuthCallback);
