/* eslint-env jest */

jest.mock("antd", () => ({
  Alert: () => null,
  Badge: () => null,
  Button: () => null,
  Drawer: () => null,
  Popconfirm: () => null,
  Select: () => null,
  Space: () => null,
  Spin: () => null,
  Table: () => null,
  Tag: () => null,
  Timeline: () => null,
  Tooltip: () => null,
  Typography: {Text: () => null, Title: () => null},
}));
jest.mock("@ant-design/icons", () => ({
  DeleteOutlined: () => null,
  HistoryOutlined: () => null,
  ReloadOutlined: () => null,
  RollbackOutlined: () => null,
  UpCircleOutlined: () => null,
}));
jest.mock("react-i18next", () => ({useTranslation: () => ({t: key => key})}));
jest.mock("./backend/HelmBackend", () => ({}));
jest.mock("./HelmInstallModal", () => () => null);

import {helmReleaseUpgradeTarget} from "./HelmReleasePage";

describe("helmReleaseUpgradeTarget", () => {
  test("preserves release identity and repository provenance", () => {
    expect(helmReleaseUpgradeTarget({
      name: "demo-release",
      namespace: "apps",
      chart: "demo-1.2.3",
      repoURL: "https://example.test/charts",
    })).toEqual({
      releaseName: "demo-release",
      namespace: "apps",
      chartName: "demo",
      repoURL: "https://example.test/charts",
      version: "1.2.3",
    });
  });

  test("does not fabricate an empty repository for legacy releases", () => {
    const target = helmReleaseUpgradeTarget({
      name: "legacy-release",
      namespace: "default",
      chart: "legacy-chart-0.9.0",
    });

    expect(target.repoURL).toBeUndefined();
    expect(target).not.toHaveProperty("_releaseName");
    expect(target).not.toHaveProperty("_namespace");
  });
});
