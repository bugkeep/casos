const assert = require("assert");
const {ACCESS_URL_TYPES} = require("./access-url-diagnostics");
const {
  findExpectedDeploymentAccessTargets,
} = require("./app-store-access-url-deployment-probe");

const releaseContext = {
  releaseName: "demo-release",
  namespace: "default",
};

const selectorMatchedTargets = findExpectedDeploymentAccessTargets({
  deployments: [
    {
      namespace: "default",
      name: "demo-worker",
      selector: {
        "app.kubernetes.io/instance": "demo-release",
        "app.kubernetes.io/name": "demo",
      },
    },
  ],
  services: [
    {
      namespace: "default",
      name: "demo-http",
      type: "NodePort",
      selector: {
        "app.kubernetes.io/instance": "demo-release",
        "app.kubernetes.io/name": "demo",
      },
      ports: [{nodePort: 31080}],
    },
  ],
  nodeIP: "192.168.250.2",
  releaseContext,
});

assert.deepStrictEqual(
  selectorMatchedTargets,
  [
    {
      deploymentName: "demo-worker",
      namespace: "default",
      serviceName: "demo-http",
      serviceType: "NodePort",
      accessUrlType: ACCESS_URL_TYPES.NODEPORT,
      accessUrl: "http://192.168.250.2:31080",
      nodePort: 31080,
    },
  ],
  "Deployment Access URL expectations should follow service selectors, not only same-name services"
);

const unrelatedTargets = findExpectedDeploymentAccessTargets({
  deployments: [
    {
      namespace: "default",
      name: "demo-worker",
      selector: {"app.kubernetes.io/instance": "demo-release"},
    },
  ],
  services: [
    {
      namespace: "default",
      name: "other-http",
      type: "NodePort",
      selector: {"app.kubernetes.io/instance": "other-release"},
      ports: [{nodePort: 31081}],
    },
  ],
  nodeIP: "192.168.250.2",
  releaseContext,
});

assert.deepStrictEqual(
  unrelatedTargets,
  [],
  "NodePort services for other releases should not create Deployment Access URL expectations"
);

const domainTargets = findExpectedDeploymentAccessTargets({
  deployments: [
    {
      namespace: "default",
      name: "demo-worker",
      selector: {
        app: "demo-worker",
        "app.kubernetes.io/instance": "demo-release",
      },
    },
  ],
  services: [
    {
      namespace: "default",
      name: "demo-web",
      type: "ClusterIP",
      selector: {
        app: "demo-worker",
      },
      ports: [{port: 9898}],
    },
  ],
  ingresses: [
    {
      namespace: "default",
      name: "demo-release-ingress",
      rules: [
        {
          host: "demo-release.casos.invalid",
          path: "/",
          serviceName: "demo-web",
          servicePort: 9898,
        },
        {
          host: "demo-release.casos.invalid",
          path: "/api",
          serviceName: "demo-web",
          servicePort: 9898,
        },
      ],
    },
  ],
  nodeIP: "",
  releaseContext,
});

assert.deepStrictEqual(
  domainTargets,
  [
    {
      deploymentName: "demo-worker",
      namespace: "default",
      serviceName: "demo-web",
      serviceType: "ClusterIP",
      accessUrlType: ACCESS_URL_TYPES.DOMAIN,
      accessUrl: "http://demo-release.casos.invalid",
      ingressName: "demo-release-ingress",
      ingressHost: "demo-release.casos.invalid",
    },
    {
      deploymentName: "demo-worker",
      namespace: "default",
      serviceName: "demo-web",
      serviceType: "ClusterIP",
      accessUrlType: ACCESS_URL_TYPES.DOMAIN,
      accessUrl: "http://demo-release.casos.invalid/api",
      ingressName: "demo-release-ingress",
      ingressHost: "demo-release.casos.invalid",
    },
  ],
  "Deployment Access URL expectations should follow Ingress rule serviceName lookups for domain links"
);

const unrelatedDomainTargets = findExpectedDeploymentAccessTargets({
  deployments: [
    {
      namespace: "default",
      name: "demo-worker",
      selector: {
        app: "demo-worker",
        "app.kubernetes.io/instance": "demo-release",
      },
    },
  ],
  services: [
    {
      namespace: "default",
      name: "other-web",
      type: "ClusterIP",
      selector: {app: "other-worker"},
      ports: [{port: 9898}],
    },
  ],
  ingresses: [
    {
      namespace: "default",
      name: "other-ingress",
      rules: [
        {
          host: "other-release.casos.invalid",
          path: "/",
          serviceName: "other-web",
          servicePort: 9898,
        },
      ],
    },
  ],
  nodeIP: "",
  releaseContext,
});

assert.deepStrictEqual(
  unrelatedDomainTargets,
  [],
  "Ingress rules for unrelated services should not create Deployment Access URL expectations"
);

const noNodeIpTargets = findExpectedDeploymentAccessTargets({
  deployments: [
    {
      namespace: "default",
      name: "demo-worker",
      selector: {"app.kubernetes.io/instance": "demo-release"},
    },
  ],
  services: [
    {
      namespace: "default",
      name: "demo-http",
      type: "NodePort",
      selector: {"app.kubernetes.io/instance": "demo-release"},
      ports: [{nodePort: 31080}],
    },
  ],
  nodeIP: "",
  releaseContext,
});

assert.deepStrictEqual(
  noNodeIpTargets,
  [],
  "Deployment Access URL expectations need the same node IP evidence used by the UI"
);

console.log("app store access URL probe checks passed");
