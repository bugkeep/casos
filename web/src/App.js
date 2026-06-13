import React, {Component} from "react";
import {ConfigProvider, Layout} from "antd";
import ManagementPage from "./ManagementPage";

const {Footer} = Layout;

class App extends Component {
  renderFooter() {
    return (
      <Footer style={{textAlign: "center"}}>
        Casos — Control Plane ©{new Date().getFullYear()}
      </Footer>
    );
  }

  render() {
    return (
      <ConfigProvider>
        <Layout id="parent-area">
          <ManagementPage />
          {this.renderFooter()}
        </Layout>
      </ConfigProvider>
    );
  }
}

export default App;
