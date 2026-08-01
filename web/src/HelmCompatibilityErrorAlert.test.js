/* eslint-env jest */

import React from "react";
import {renderToStaticMarkup} from "react-dom/server";
import HelmCompatibilityErrorAlert from "./HelmCompatibilityErrorAlert";

jest.mock("antd", () => {
  const React = require("react");
  const Text = ({children}) => React.createElement("span", null, children);
  return {
    Alert: ({message, description}) => React.createElement("div", null, message, description),
    Typography: {Text},
  };
});

test("renders a friendly Helm compatibility message with GVK and details", () => {
  const html = renderToStaticMarkup(
    <HelmCompatibilityErrorAlert
      error={{
        message: "当前集群不支持 Chart 所需的资源类型",
        gvk: "cert-manager.io/v1, Kind=Certificate",
        detail: "no matches for kind Certificate",
      }}
      onClose={() => {}}
      t={key => ({
        "helm:Missing resource": "缺少资源",
        "helm:Compatibility details": "兼容性详情",
      })[key]}
    />
  );

  expect(html).toContain("当前集群不支持 Chart 所需的资源类型");
  expect(html).toContain("缺少资源");
  expect(html).toContain("cert-manager.io/v1, Kind=Certificate");
  expect(html).toContain("兼容性详情");
  expect(html).toContain("no matches for kind Certificate");
});
