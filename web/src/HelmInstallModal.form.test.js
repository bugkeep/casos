/* eslint-env jest */

import React, {act} from "react";
import {createRoot} from "react-dom/client";
import HelmInstallModal from "./HelmInstallModal";
import * as HelmBackend from "./backend/HelmBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";

let mockForm;
let mockWatchListeners;

jest.mock("antd", () => {
  const React = require("react");
  const component = tag => ({children}) => React.createElement(tag, null, children);
  const Form = component("form");
  Form.useForm = () => [mockForm];
  Form.useWatch = name => {
    const [value, setValue] = React.useState(() => mockForm?.getFieldValue(name));
    React.useEffect(() => {
      const listener = {name, setValue};
      mockWatchListeners.add(listener);
      return () => mockWatchListeners.delete(listener);
    }, [name]);
    return value;
  };
  Form.Item = component("div");
  const Typography = {Text: component("span")};
  return {
    Alert: ({message, description, closable, onClose}) => React.createElement(
      "div",
      {role: "alert"},
      message,
      description,
      closable && React.createElement("button", {onClick: onClose}, "close alert")
    ),
    Button: ({children, onClick, disabled}) => React.createElement("button", {onClick, disabled}, children),
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
    formValues = {};
    mockWatchListeners = new Set();
    touchedFields = new Set();
    mockForm = {
      getFieldValue: jest.fn(name => formValues[name]),
      isFieldTouched: jest.fn(name => touchedFields.has(name)),
      setFieldsValue: jest.fn(values => {
        Object.assign(formValues, values);
        for (const {name, setValue} of mockWatchListeners) {
          if (Object.prototype.hasOwnProperty.call(values, name)) {
            setValue(values[name]);
          }
        }
      }),
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

  test("a response from the previous chart does not replace the current chart values", async() => {
    jest.useFakeTimers();
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
      jest.advanceTimersByTime(500);
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
      jest.advanceTimersByTime(500);
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

  test("repo URL changes are debounced before loading chart values", async() => {
    jest.useFakeTimers();
    NamespaceBackend.getNamespaces.mockResolvedValue({status: "ok", data: [{name: "default"}]});
    const chart = {
      chartName: "legacy-chart",
      repoURL: "",
      version: "1.0.0",
      releaseName: "legacy-release",
      namespace: "default",
    };

    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          action="upgrade"
          chart={chart}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
    });

    const repoURLs = ["h", "ht", "https://example.test/charts"];
    for (const [index, repoURL] of repoURLs.entries()) {
      await act(async() => {
        mockForm.setFieldsValue({repoURL});
      });
      if (index < repoURLs.length - 1) {
        await act(async() => {
          jest.advanceTimersByTime(100);
        });
      }
    }

    expect(HelmBackend.getHelmChartValues).not.toHaveBeenCalled();
    await act(async() => {
      jest.advanceTimersByTime(499);
    });
    expect(HelmBackend.getHelmChartValues).not.toHaveBeenCalled();

    await act(async() => {
      jest.advanceTimersByTime(1);
    });
    expect(HelmBackend.getHelmChartValues).toHaveBeenCalledTimes(1);
    expect(HelmBackend.getHelmChartValues).toHaveBeenCalledWith(
      "legacy-chart",
      "https://example.test/charts",
      "1.0.0",
      expect.any(AbortSignal)
    );
  });

  test("changing repo URL aborts an in-flight values request", async() => {
    jest.useFakeTimers();
    NamespaceBackend.getNamespaces.mockResolvedValue({status: "ok", data: [{name: "default"}]});
    HelmBackend.getHelmChartValues.mockReturnValue(new Promise(() => {}));
    const chart = {
      chartName: "legacy-chart",
      repoURL: "https://example.test/old",
      version: "1.0.0",
      releaseName: "legacy-release",
      namespace: "default",
    };

    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          action="upgrade"
          chart={chart}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
    });
    await act(async() => {
      jest.advanceTimersByTime(500);
    });
    const firstSignal = HelmBackend.getHelmChartValues.mock.calls[0][3];

    await act(async() => {
      mockForm.setFieldsValue({repoURL: "https://example.test/new"});
    });

    expect(firstSignal.aborted).toBe(true);
  });

  test("a failed repo values reload preserves values edited by the user", async() => {
    jest.useFakeTimers();
    NamespaceBackend.getNamespaces.mockResolvedValue({status: "ok", data: [{name: "default"}]});
    HelmBackend.getHelmChartValues
      .mockResolvedValueOnce({status: "ok", data: "replicas: 1"})
      .mockRejectedValueOnce(new Error("load failed"));
    const chart = {
      chartName: "legacy-chart",
      repoURL: "https://example.test/old",
      version: "1.0.0",
      releaseName: "legacy-release",
      namespace: "default",
    };

    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          action="upgrade"
          chart={chart}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
    });
    await act(async() => {
      jest.advanceTimersByTime(500);
      await Promise.resolve();
    });
    const textarea = container.querySelector("textarea");
    await act(async() => {
      const setTextareaValue = Object.getOwnPropertyDescriptor(
        HTMLTextAreaElement.prototype,
        "value"
      ).set;
      setTextareaValue.call(textarea, "replicas: 3");
      textarea.dispatchEvent(new Event("input", {bubbles: true}));
    });

    await act(async() => {
      mockForm.setFieldsValue({repoURL: "https://example.test/new"});
    });
    await act(async() => {
      jest.advanceTimersByTime(500);
      await Promise.resolve();
    });

    expect(container.querySelector("textarea").value).toBe("replicas: 3");
  });

  test("a successful repo values reload clears the previous load error", async() => {
    jest.useFakeTimers();
    NamespaceBackend.getNamespaces.mockResolvedValue({status: "ok", data: [{name: "default"}]});
    HelmBackend.getHelmChartValues
      .mockRejectedValueOnce(new Error("load failed"))
      .mockResolvedValueOnce({status: "ok", data: "replicas: 2"});
    const chart = {
      chartName: "legacy-chart",
      repoURL: "https://example.test/bad",
      version: "1.0.0",
      releaseName: "legacy-release",
      namespace: "default",
    };

    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          action="upgrade"
          chart={chart}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
    });
    await act(async() => {
      jest.advanceTimersByTime(500);
      await Promise.resolve();
    });
    expect(container.textContent).toContain("load failed");

    await act(async() => {
      mockForm.setFieldsValue({repoURL: "https://example.test/good"});
    });
    await act(async() => {
      jest.advanceTimersByTime(500);
      await Promise.resolve();
    });

    expect(container.textContent).not.toContain("load failed");
    expect(container.querySelector("textarea").value).toBe("replicas: 2");
  });

  test("a values reload success does not clear an operation error", async() => {
    jest.useFakeTimers();
    let resolveValues;
    let rejectOperation;
    NamespaceBackend.getNamespaces.mockResolvedValue({status: "ok", data: [{name: "default"}]});
    HelmBackend.getHelmChartValues
      .mockResolvedValueOnce({status: "ok", data: "replicas: 1"})
      .mockReturnValueOnce(new Promise(resolve => {
        resolveValues = resolve;
      }));
    mockForm.validateFields.mockResolvedValue({
      releaseName: "demo-release",
      namespace: "default",
      version: "1.0.0",
    });
    HelmBackend.installHelmChartStream.mockReturnValue(new Promise((_resolve, reject) => {
      rejectOperation = reject;
    }));

    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          chart={{chartName: "demo", repoURL: "https://example.test/charts", version: "1.0.0"}}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
    });
    await act(async() => {
      jest.advanceTimersByTime(0);
      await Promise.resolve();
    });

    const installButton = [...container.querySelectorAll("button")]
      .find(button => button.textContent === "helm:Install");
    await act(async() => {
      installButton.click();
      await Promise.resolve();
    });
    await act(async() => {
      mockForm.setFieldsValue({repoURL: "https://example.test/new"});
    });
    await act(async() => {
      jest.advanceTimersByTime(500);
    });
    await act(async() => {
      rejectOperation(new Error("upgrade failed"));
      await Promise.resolve();
    });
    expect(container.textContent).toContain("upgrade failed");

    await act(async() => {
      resolveValues({status: "ok", data: "replicas: 2"});
      await Promise.resolve();
    });

    expect(container.textContent).toContain("upgrade failed");
  });

  test("a repo values reload blocks editing and submission until it completes", async() => {
    jest.useFakeTimers();
    let resolveReload;
    NamespaceBackend.getNamespaces.mockResolvedValue({status: "ok", data: [{name: "default"}]});
    HelmBackend.getHelmChartValues
      .mockResolvedValueOnce({status: "ok", data: "replicas: 1"})
      .mockReturnValueOnce(new Promise(resolve => {
        resolveReload = resolve;
      }));
    const chart = {
      chartName: "legacy-chart",
      repoURL: "https://example.test/old",
      version: "1.0.0",
      releaseName: "legacy-release",
      namespace: "default",
    };

    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          action="upgrade"
          chart={chart}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
    });
    await act(async() => {
      jest.advanceTimersByTime(500);
      await Promise.resolve();
    });

    await act(async() => {
      mockForm.setFieldsValue({repoURL: "https://example.test/new"});
    });

    const upgradeButton = [...container.querySelectorAll("button")]
      .find(button => button.textContent === "helm:Upgrade");
    expect(container.querySelector("textarea")).toBeNull();
    expect(upgradeButton.disabled).toBe(true);

    await act(async() => {
      jest.advanceTimersByTime(500);
    });
    expect(upgradeButton.disabled).toBe(true);

    await act(async() => {
      resolveReload({status: "ok", data: "replicas: 2"});
      await Promise.resolve();
    });

    expect(container.querySelector("textarea").value).toBe("replicas: 2");
    expect(upgradeButton.disabled).toBe(false);
  });

  test("changing the target version reloads values for that version", async() => {
    jest.useFakeTimers();
    NamespaceBackend.getNamespaces.mockResolvedValue({status: "ok", data: [{name: "default"}]});
    HelmBackend.getHelmChartValues.mockResolvedValue({status: "ok", data: "replicas: 1"});
    const chart = {
      chartName: "legacy-chart",
      repoURL: "https://example.test/charts",
      version: "1.0.0",
      releaseName: "legacy-release",
      namespace: "default",
    };

    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          action="upgrade"
          chart={chart}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
    });
    await act(async() => {
      jest.advanceTimersByTime(500);
      await Promise.resolve();
    });
    HelmBackend.getHelmChartValues.mockClear();

    await act(async() => {
      mockForm.setFieldsValue({version: "2.0.0"});
    });
    await act(async() => {
      jest.advanceTimersByTime(500);
      await Promise.resolve();
    });

    expect(HelmBackend.getHelmChartValues).toHaveBeenCalledWith(
      "legacy-chart",
      "https://example.test/charts",
      "2.0.0",
      expect.any(AbortSignal)
    );

    HelmBackend.getHelmChartValues.mockClear();
    await act(async() => {
      mockForm.setFieldsValue({version: ""});
    });
    await act(async() => {
      jest.advanceTimersByTime(500);
      await Promise.resolve();
    });
    expect(HelmBackend.getHelmChartValues).toHaveBeenCalledWith(
      "legacy-chart",
      "https://example.test/charts",
      "1.0.0",
      expect.any(AbortSignal)
    );
  });

  test("install version changes are debounced before loading values", async() => {
    jest.useFakeTimers();
    NamespaceBackend.getNamespaces.mockResolvedValue({status: "ok", data: [{name: "default"}]});
    HelmBackend.getHelmChartValues.mockResolvedValue({status: "ok", data: "replicas: 1"});
    const chart = {
      chartName: "demo",
      repoURL: "https://example.test/charts",
      version: "1.0.0",
    };

    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          chart={chart}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
    });
    await act(async() => {
      jest.advanceTimersByTime(0);
      await Promise.resolve();
    });
    HelmBackend.getHelmChartValues.mockClear();

    await act(async() => {
      mockForm.setFieldsValue({version: "2.0.0"});
    });
    await act(async() => {
      jest.advanceTimersByTime(499);
    });
    expect(HelmBackend.getHelmChartValues).not.toHaveBeenCalled();

    await act(async() => {
      jest.advanceTimersByTime(1);
      await Promise.resolve();
    });
    expect(HelmBackend.getHelmChartValues).toHaveBeenCalledWith(
      "demo",
      "https://example.test/charts",
      "2.0.0",
      expect.any(AbortSignal)
    );
  });

  test("a values load error cannot be dismissed to bypass the submit lock", async() => {
    jest.useFakeTimers();
    NamespaceBackend.getNamespaces.mockResolvedValue({status: "ok", data: [{name: "default"}]});
    HelmBackend.getHelmChartValues.mockRejectedValue(new Error("load failed"));

    await act(async() => {
      root.render(
        <HelmInstallModal
          open
          action="upgrade"
          chart={{
            chartName: "demo",
            repoURL: "https://example.test/charts",
            version: "1.0.0",
            releaseName: "demo-release",
            namespace: "default",
          }}
          onClose={jest.fn()}
          onInstalled={jest.fn()}
        />
      );
    });
    await act(async() => {
      jest.advanceTimersByTime(0);
      await Promise.resolve();
    });

    const upgradeButton = [...container.querySelectorAll("button")]
      .find(button => button.textContent === "helm:Upgrade");
    const closeAlertButton = [...container.querySelectorAll("button")]
      .find(button => button.textContent === "close alert");
    expect(upgradeButton.disabled).toBe(true);
    expect(closeAlertButton).toBeUndefined();
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
    await act(async() => {
      jest.advanceTimersByTime(0);
      await Promise.resolve();
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

    expect(HelmBackend.getHelmOperationTask).toHaveBeenCalledWith("42");
  });

  test("completion from a previous chart stream does not finish the current modal", async() => {
    jest.useFakeTimers();
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
    await act(async() => {
      jest.advanceTimersByTime(0);
      await Promise.resolve();
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
