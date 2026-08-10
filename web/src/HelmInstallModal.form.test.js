/* eslint-env jest */

import React, {act} from "react";
import {createRoot} from "react-dom/client";
import HelmInstallModal from "./HelmInstallModal";
import * as HelmBackend from "./backend/HelmBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import {helmTaskStorageKey} from "./helmTaskStorage";

let mockForm;

jest.mock("antd", () => {
  const React = require("react");
  const component = tag => ({children}) => React.createElement(tag, null, children);
  const Form = component("form");
  Form.useForm = () => [mockForm];
  Form.Item = component("div");
  const Typography = {Text: component("span")};
  return {
    Alert: component("div"),
    Button: ({children, onClick}) => React.createElement("button", {onClick}, children),
    Form,
    Input: component("input"),
    Modal: ({children, footer}) => React.createElement("div", null, children, footer),
    Select: component("select"),
    Spin: component("span"),
    Typography,
  };
});

jest.mock("react-i18next", () => ({
  useTranslation: () => ({t: key => key}),
}));

jest.mock("./backend/HelmBackend");
jest.mock("./backend/NamespaceBackend");

describe("HelmInstallModal form initialization", () => {
  let container;
  let root;
  let formValues;
  let touchedFields;
  let resolveNamespaces;

  beforeEach(() => {
    global.IS_REACT_ACT_ENVIRONMENT = true;
    Element.prototype.scrollIntoView = jest.fn();
    window.localStorage.clear();
    formValues = {};
    touchedFields = new Set();
    mockForm = {
      getFieldValue: jest.fn(name => formValues[name]),
      isFieldTouched: jest.fn(name => touchedFields.has(name)),
      setFieldsValue: jest.fn(values => Object.assign(formValues, values)),
      validateFields: jest.fn(),
    };
    NamespaceBackend.getNamespaces.mockReturnValue(new Promise(resolve => {
      resolveNamespaces = resolve;
    }));
    HelmBackend.getHelmChartValues.mockResolvedValue({status: "ok", data: ""});
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async() => {
    await act(async() => root.unmount());
    container.remove();
    jest.clearAllMocks();
    jest.useRealTimers();
  });

  test("a late namespace response does not overwrite a release name entered by the user", async() => {
    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          chart={{chartName: "casdoor-helm-charts", repoURL: "oci://example.test/casdoor", version: "1.0.0"}}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
    });

    formValues.releaseName = "e2e-casdoor-12345678";
    touchedFields.add("releaseName");

    await act(async() => {
      resolveNamespaces({status: "ok", data: [{name: "default"}]});
      await Promise.resolve();
    });

    expect(formValues.releaseName).toBe("e2e-casdoor-12345678");
    expect(formValues.namespace).toBe("default");
  });

  test("upgrade mode initializes the existing release identity and repository", async() => {
    NamespaceBackend.getNamespaces.mockResolvedValue({status: "ok", data: [{name: "apps"}, {name: "default"}]});

    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          action="upgrade"
          chart={{
            chartName: "demo",
            releaseName: "demo-release",
            namespace: "apps",
            repoURL: "https://example.test/charts",
            version: "1.0.0",
          }}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
      await Promise.resolve();
    });

    expect(formValues).toMatchObject({
      releaseName: "demo-release",
      namespace: "apps",
      repoURL: "https://example.test/charts",
      version: "1.0.0",
    });
  });

  test("upgrade mode submits the upgrade stream with a locked release identity", async() => {
    NamespaceBackend.getNamespaces.mockResolvedValue({status: "ok", data: [{name: "apps"}]});
    mockForm.validateFields.mockResolvedValue({
      releaseName: "other-release",
      namespace: "other-namespace",
      repoURL: "https://example.test/charts",
      version: "1.1.0",
    });
    HelmBackend.upgradeHelmChartStream.mockImplementation((_payload, onLine) => {
      onLine("TASK_ID:84");
      return new Promise(() => {});
    });

    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          action="upgrade"
          chart={{
            chartName: "demo",
            releaseName: "demo-release",
            namespace: "apps",
            version: "1.0.0",
          }}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
    });
    const upgradeButton = [...container.querySelectorAll("button")]
      .find(button => button.textContent === "helm:Upgrade");
    expect(upgradeButton).toBeDefined();

    await act(async() => {
      upgradeButton.click();
      await Promise.resolve();
    });

    expect(HelmBackend.installHelmChartStream).not.toHaveBeenCalled();
    expect(HelmBackend.upgradeHelmChartStream).toHaveBeenCalledWith(
      expect.objectContaining({
        releaseName: "demo-release",
        namespace: "apps",
        chartName: "demo",
        repoURL: "https://example.test/charts",
        version: "1.1.0",
      }),
      expect.any(Function),
      expect.any(AbortSignal)
    );
  });

  test("upgrade mode does not resume a task for another release of the same chart", async() => {
    window.localStorage.setItem(helmTaskStorageKey("demo", "apps", "other-release"), JSON.stringify({
      schemaVersion: 1,
      taskId: 91,
      createdAt: Date.now(),
      chartName: "demo",
      namespace: "apps",
      releaseName: "other-release",
    }));
    NamespaceBackend.getNamespaces.mockResolvedValue({status: "ok", data: [{name: "apps"}]});
    HelmBackend.getHelmOperationTask.mockReturnValue(new Promise(() => {}));

    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          action="upgrade"
          chart={{
            chartName: "demo",
            releaseName: "demo-release",
            namespace: "apps",
            repoURL: "https://example.test/charts",
            version: "1.0.0",
          }}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
      await Promise.resolve();
    });

    expect(HelmBackend.getHelmOperationTask).not.toHaveBeenCalled();
    expect(formValues).toMatchObject({releaseName: "demo-release", namespace: "apps"});
  });

  test("a response from the previous chart does not replace the current chart values", async() => {
    let resolveOldValues;
    let resolveNewValues;
    NamespaceBackend.getNamespaces.mockResolvedValue({status: "ok", data: [{name: "default"}]});
    HelmBackend.getHelmChartValues
      .mockReturnValueOnce(new Promise(resolve => {
        resolveOldValues = resolve;
      }))
      .mockReturnValueOnce(new Promise(resolve => {
        resolveNewValues = resolve;
      }));

    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          chart={{chartName: "old-chart", repoURL: "oci://example.test/old", version: "1.0.0"}}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
    });
    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          chart={{chartName: "new-chart", repoURL: "oci://example.test/new", version: "2.0.0"}}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
    });

    await act(async() => {
      resolveNewValues({status: "ok", data: "current: true"});
      await Promise.resolve();
    });
    expect(container.querySelector("textarea").value).toBe("current: true");

    await act(async() => {
      resolveOldValues({status: "ok", data: "stale: true"});
      await Promise.resolve();
    });
    expect(container.querySelector("textarea").value).toBe("current: true");
  });

  test("a stalled install stream falls back to the persisted task", async() => {
    jest.useFakeTimers();
    NamespaceBackend.getNamespaces.mockResolvedValue({status: "ok", data: [{name: "default"}]});
    mockForm.validateFields.mockResolvedValue({
      releaseName: "demo-release",
      namespace: "default",
      version: "1.0.0",
    });
    HelmBackend.installHelmChartStream.mockImplementation((_payload, onLine) => {
      onLine("TASK_ID:42");
      return new Promise(() => {});
    });
    HelmBackend.getHelmOperationTask.mockReturnValue(new Promise(() => {}));

    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          chart={{chartName: "demo", repoURL: "oci://example.test/demo", version: "1.0.0"}}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
    });
    const installButton = [...container.querySelectorAll("button")]
      .find(button => button.textContent === "helm:Install");
    await act(async() => {
      installButton.click();
      await Promise.resolve();
    });

    await act(async() => {
      jest.advanceTimersByTime(60_000);
      await Promise.resolve();
    });

    expect(HelmBackend.installHelmChartStream).toHaveBeenCalled();
    expect(HelmBackend.upgradeHelmChartStream).not.toHaveBeenCalled();
    expect(HelmBackend.getHelmOperationTask).toHaveBeenCalledWith("42");
  });

  test("completion from a previous chart stream does not finish the current modal", async() => {
    let resolveOldStream;
    NamespaceBackend.getNamespaces.mockResolvedValue({status: "ok", data: [{name: "default"}]});
    mockForm.validateFields.mockResolvedValue({
      releaseName: "old-release",
      namespace: "default",
      version: "1.0.0",
    });
    HelmBackend.installHelmChartStream.mockReturnValue(new Promise(resolve => {
      resolveOldStream = resolve;
    }));

    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          chart={{chartName: "old-chart", repoURL: "oci://example.test/old", version: "1.0.0"}}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
    });
    const installButton = [...container.querySelectorAll("button")]
      .find(button => button.textContent === "helm:Install");
    await act(async() => {
      installButton.click();
      await Promise.resolve();
    });
    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          chart={{chartName: "new-chart", repoURL: "oci://example.test/new", version: "2.0.0"}}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
    });

    await act(async() => {
      resolveOldStream("DONE");
      await Promise.resolve();
    });

    expect([...container.querySelectorAll("button")]
      .some(button => button.textContent === "general:Done")).toBe(false);
  });
});
