import React from "react";
import {Button, Result, Spin} from "antd";
import {withRouter} from "react-router-dom";
import * as Setting from "./Setting";
import {resolveSigninRedirect} from "./SigninPage";

class AuthCallback extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      classes: props,
      msg: null,
    };
  }

  componentDidMount() {
    this.mounted = true;
    this.login();
  }

  componentWillUnmount() {
    this.mounted = false;
  }

  getFromLink() {
    const from = sessionStorage.getItem("from");
    sessionStorage.removeItem("from");
    if (from === null) {
      return "/";
    }
    return resolveSigninRedirect(from);
  }

  async login() {
    try {
      const res = await Setting.signin();
      if (res.status === "ok") {
        Setting.showMessage("success", "Logged in successfully");

        const link = this.getFromLink();
        Setting.goToLink(link);
      } else if (this.mounted) {
        this.setState({
          msg: res.msg,
        });
      }
    } catch {
      if (this.mounted) {
        this.setState({msg: "Sign-in failed. Please try again."});
      }
    }
  }

  render() {
    return (
      <div style={{textAlign: "center"}}>
        {this.state.msg === null ? (
          <Spin
            size="large"
            tip="Signing in..."
            style={{paddingTop: "10%"}}
          />
        ) : (
          <div style={{display: "inline"}}>
            <Result
              status="error"
              title="Login Error"
              subTitle={this.state.msg}
              extra={[
                <Button type="primary" key="details">
                  Details
                </Button>,
                <Button key="help">Help</Button>,
              ]}
            />
          </div>
        )}
      </div>
    );
  }
}

export default withRouter(AuthCallback);
