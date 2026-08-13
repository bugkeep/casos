import React, {Component} from "react";
import {Redirect, Route, Switch, withRouter} from "react-router-dom";
import {StyleProvider, legacyLogicalPropertiesTransformer} from "@ant-design/cssinjs";
import {ConfigProvider, FloatButton, Layout, Spin} from "antd";
import * as Setting from "./Setting";
import * as AccountBackend from "./backend/AccountBackend";
import * as SiteBackend from "./backend/SiteBackend";
import {getShadcnThemeComponents, getShadcnThemeToken} from "./shadcnTheme";
import ManagementPage from "./ManagementPage";
import AuthCallback from "./AuthCallback";
import SigninPage from "./SigninPage";

class App extends Component {
  constructor(props) {
    super(props);
    Setting.initServerUrl();

    let storageThemeAlgorithm = ["default"];
    try {
      const raw = localStorage.getItem("themeAlgorithm");
      if (raw) {storageThemeAlgorithm = JSON.parse(raw);}
    } catch {
      storageThemeAlgorithm = ["default"];
    }
    document.documentElement.setAttribute("data-theme", storageThemeAlgorithm.includes("dark") ? "dark" : "light");

    this.state = {
      account: undefined,
      auth: undefined,
      uri: null,
      themeAlgorithm: storageThemeAlgorithm,
      site: undefined,
      logo: null,
    };
  }

  componentDidMount() {
    this.refreshAuth();
    this.loadSite();
  }

  componentDidUpdate() {
    // eslint-disable-next-line no-restricted-globals
    const uri = location.pathname;
    if (this.state.uri !== uri) {
      this.setState({uri});
    }
  }

  loadSite() {
    SiteBackend.getBuiltInSite()
      .then((res) => {
        if (res && res.status === "ok" && res.data) {
          const site = res.data;
          this.setState({site});
          if (site.htmlTitle) {document.title = site.htmlTitle;}
          if (site.themeColor) {Setting.setThemeColor(site.themeColor);}
          this.updateFavicon(Setting.getFaviconUrl(this.state.themeAlgorithm, site.faviconUrl));
        }
      })
      .catch(() => {});
  }

  updateFavicon(url) {
    let link = document.querySelector("link[rel=\"icon\"]");
    if (!link) {
      link = document.createElement("link");
      link.rel = "icon";
      document.head.appendChild(link);
    }
    link.href = url;
  }

  refreshAuth = () => AccountBackend.getAuthStatus()
    .then((res) => {
      if (!res || res.status !== "ok") {
        this.setState({auth: {error: res?.msg || "Unable to read authentication status"}, account: null});
        return;
      }
      const auth = res.data;
      Setting.setAuthStatus(auth);
      if (auth.provider === "casdoor" && auth.casdoor) {
        Setting.initCasdoorSdk(auth.casdoor);
      }
      this.setState({auth, account: auth.authenticated ? auth.user : null});
    })
    .catch(() => this.setState({auth: {error: "Unable to reach the CasOS service"}, account: null}));

  handleAuthenticated = () => this.refreshAuth().then(() => {
    this.props.history.replace("/");
  });

  signout() {
    AccountBackend.signout().then((res) => {
      if (res.status === "ok") {
        Setting.setAuthStatus(null);
        this.setState({account: null, auth: {...this.state.auth, authenticated: false, user: null, csrfToken: ""}});
        Setting.showMessage("success", "Successfully signed out");
        Setting.goToLink("/");
      } else {
        Setting.showMessage("error", `Signout failed: ${res.msg}`);
      }
    });
  }

  onUpdateSite = () => {
    this.loadSite();
  };

  setLogoAndThemeAlgorithm = (nextThemeAlgorithm) => {
    this.setState({
      themeAlgorithm: nextThemeAlgorithm,
      logo: Setting.getLogo(nextThemeAlgorithm, this.state.site?.logoUrl),
    });
    localStorage.setItem("themeAlgorithm", JSON.stringify(nextThemeAlgorithm));
    document.documentElement.setAttribute("data-theme", nextThemeAlgorithm.includes("dark") ? "dark" : "light");
    this.updateFavicon(Setting.getFaviconUrl(nextThemeAlgorithm, this.state.site?.faviconUrl));
  };

  renderSigninIfNotSignedIn(component) {
    if (this.state.account === null) {
      sessionStorage.setItem("from", window.location.pathname);
      return <Redirect to="/signin" />;
    } else if (this.state.account === undefined) {
      return null;
    }
    return component;
  }

  renderContent() {
    const {auth} = this.state;
    if (auth === undefined) {
      return <div className="auth-loading"><Spin size="large" /></div>;
    }
    if (auth.error) {
      return <SigninPage mode="error" auth={auth} onAuthenticated={this.refreshAuth} />;
    }
    if (!auth.initialized) {
      return (
        <Layout id="parent-area">
          <Switch>
            <Route exact path="/setup" render={() => <SigninPage mode="setup" auth={auth} onAuthenticated={this.handleAuthenticated} />} />
            <Redirect to="/setup" />
          </Switch>
        </Layout>
      );
    }
    const authenticated = auth.authenticated;
    return (
      <Layout id="parent-area">
        <Switch>
          <Route exact path="/callback" render={() => auth.provider === "casdoor" ? <AuthCallback /> : <Redirect to="/signin" />} />
          <Route exact path="/signin" render={() => authenticated ? <Redirect to="/" /> : <SigninPage mode="signin" auth={auth} onAuthenticated={this.handleAuthenticated} />} />
          <Route exact path="/recover" render={() => authenticated ? <Redirect to="/" /> : <SigninPage mode="recover" auth={auth} onAuthenticated={this.handleAuthenticated} />} />
          <Route path="/" render={(props) => this.renderSigninIfNotSignedIn(
            <ManagementPage
              account={this.state.account}
              uri={this.state.uri}
              history={this.props.history}
              site={this.state.site}
              themeAlgorithm={this.state.themeAlgorithm}
              logo={this.state.logo}
              onSignout={this.signout.bind(this)}
              onUpdateSite={this.onUpdateSite}
              setLogoAndThemeAlgorithm={this.setLogoAndThemeAlgorithm}
              authProvider={auth.provider}
              onAuthChanged={this.refreshAuth}
              {...props}
            />
          )} />
        </Switch>
      </Layout>
    );
  }

  render() {
    const isDark = this.state.themeAlgorithm.includes("dark");
    const themeColor = Setting.getThemeColor();
    return (
      <React.Fragment>
        <ConfigProvider
          theme={{
            token: {
              ...getShadcnThemeToken(isDark),
              colorPrimary: themeColor,
              colorInfo: themeColor,
            },
            components: getShadcnThemeComponents(isDark),
            algorithm: Setting.getAlgorithm(this.state.themeAlgorithm),
          }}>
          <StyleProvider hashPriority="high" transformers={[legacyLogicalPropertiesTransformer]}>
            <React.Fragment>
              <FloatButton.BackTop />
              {this.renderContent()}
            </React.Fragment>
          </StyleProvider>
        </ConfigProvider>
      </React.Fragment>
    );
  }
}

export default withRouter(App);
