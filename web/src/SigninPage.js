import React from "react";
import PasswordSigninPage from "./PasswordSigninPage";
import * as Setting from "./Setting";

class SigninPage extends React.Component {
  render() {
    const {logo, themeAlgorithm, site} = this.props;
    // App only fills in logo once the theme is switched, so fall back the same
    // way ManagementPage does.
    return <PasswordSigninPage logo={logo || Setting.getLogo(themeAlgorithm || [], site?.logoUrl)} />;
  }
}

export default SigninPage;
