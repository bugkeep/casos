/* eslint-env jest */

jest.mock("../Setting", () => ({
  ServerUrl: "http://localhost:9000",
  getAcceptLanguage: () => "en",
}));

import {TextDecoder} from "util";
import {getHelmChartValues, installHelmChartStream} from "./HelmBackend";

global.TextDecoder = TextDecoder;

test("getHelmChartValues forwards the abort signal", async() => {
  global.fetch = jest.fn().mockResolvedValue({
    json: jest.fn().mockResolvedValue({status: "ok", data: "replicas: 1"}),
  });
  const controller = new AbortController();

  await getHelmChartValues("demo", "https://example.test/charts", "1.0.0", controller.signal);

  expect(global.fetch.mock.calls[0][1].signal).toBe(controller.signal);
});

function makeChunk(text) {
  return Uint8Array.from(Buffer.from(text, "utf8"));
}

function mockStreamResponse(chunks) {
  let index = 0;
  return {
    body: {
      getReader() {
        return {
          async read() {
            if (index >= chunks.length) {
              return {done: true};
            }
            return {done: false, value: makeChunk(chunks[index++])};
          },
        };
      },
    },
  };
}

describe("installHelmChartStream", () => {
  afterEach(() => {
    jest.resetAllMocks();
  });

  test("rejects with a compatibility code from a structured error event", async() => {
    global.fetch = jest.fn().mockResolvedValue(mockStreamResponse([
      "data: {\"type\":\"log\",\"message\":\"creating 1 resource(s)\"}\n\n",
      "data: {\"type\":\"error\",\"message\":\"install failed\",\"error\":{\"code\":\"RESOURCE_NOT_SERVED\",\"gvk\":\"cert-manager.io/v1, Kind=Certificate\"}}\n\n",
    ]));

    const onLine = jest.fn();
    await expect(installHelmChartStream({releaseName: "demo"}, onLine))
      .rejects.toMatchObject({
        message: "install failed",
        code: "RESOURCE_NOT_SERVED",
        gvk: "cert-manager.io/v1, Kind=Certificate",
      });

    expect(onLine).toHaveBeenCalledTimes(2);
    expect(onLine).toHaveBeenNthCalledWith(1, "creating 1 resource(s)");
    expect(onLine).toHaveBeenNthCalledWith(2, "install failed");
  });

  test("returns DONE when the server completes the install stream", async() => {
    global.fetch = jest.fn().mockResolvedValue(mockStreamResponse([
      "data: {\"type\":\"log\",\"message\":\"creating 1 resource(s)\"}\n\n",
      "data: {\"type\":\"done\"}\n\n",
    ]));

    const onLine = jest.fn();
    const status = await installHelmChartStream({releaseName: "demo"}, onLine);

    expect(status).toBe("DONE");
    expect(onLine).toHaveBeenCalledTimes(2);
    expect(onLine).toHaveBeenNthCalledWith(2, "DONE");
  });

  test("forwards the task id and abort signal", async() => {
    global.fetch = jest.fn().mockResolvedValue(mockStreamResponse([
      "data: {\"type\":\"log\",\"message\":\"TASK_ID:42\"}\n\n",
      "data: {\"type\":\"done\"}\n\n",
    ]));
    const signal = {aborted: false};
    const onLine = jest.fn();

    await installHelmChartStream({releaseName: "demo"}, onLine, signal);

    expect(global.fetch.mock.calls[0][1].signal).toBe(signal);
    expect(onLine).toHaveBeenNthCalledWith(1, "TASK_ID:42");
    expect(onLine).toHaveBeenNthCalledWith(2, "DONE");
  });
});
