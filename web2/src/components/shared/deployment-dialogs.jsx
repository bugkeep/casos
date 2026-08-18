import React, {useEffect, useState} from "react";
import {Link2} from "lucide-react";
import * as ServiceBackend from "@/backend/ServiceBackend";
import * as IngressBackend from "@/backend/IngressBackend";
import * as PodBackend from "@/backend/PodBackend";
import * as DeploymentBackend from "@/backend/DeploymentBackend";
import * as Setting from "@/Setting";
import {runAction} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {NumberInput} from "@/components/shared/number-input";
import {SearchSelect, SimpleSelect} from "@/components/shared/simple-select";

const SERVICE_TYPES = [
  {label: "ClusterIP — 集群内访问", value: "ClusterIP"},
  {label: "NodePort — 节点端口对外暴露", value: "NodePort"},
  {label: "LoadBalancer — 云负载均衡器", value: "LoadBalancer"},
];

// The container's own port is the only sensible default for the Service port;
// falling back to 80 keeps the form usable for images that declare none.
function defaultPorts(deploy) {
  const declared = Number(deploy?.ports?.find((port) => Number(port.port) > 0)?.port);
  const port = declared > 0 ? declared : 80;
  return {port, targetPort: port};
}

// A Service needs a selector that actually matches the deployment's pods. The
// deployment's own selector is authoritative; `app: <name>` is the convention
// this app creates deployments with, and is the fallback.
function defaultSelector(deploy) {
  const entries = Object.entries(deploy?.selector || {}).filter(([key, value]) => key && value);
  if (entries.length > 0) {
    return Object.fromEntries(entries);
  }
  return deploy?.name ? {app: deploy.name} : {};
}

/** Creates a Service in front of a Deployment. */
export function DeploymentExposeDialog({deploy, open, onClose}) {
  const [form, setForm] = useState({name: "", type: "ClusterIP", port: 80, targetPort: 80});
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!open || !deploy) {
      return;
    }
    const ports = defaultPorts(deploy);
    setForm({name: deploy.name, type: "ClusterIP", ...ports});
    setErrors({});
  }, [open, deploy]);

  async function handleSubmit() {
    if (!form.name) {
      setErrors({name: "Service name is required"});
      return;
    }
    setErrors({});
    setSubmitting(true);
    const ok = await runAction(
      ServiceBackend.addService({
        namespace: deploy.namespace,
        name: form.name,
        type: form.type,
        selector: defaultSelector(deploy),
        ports: [{name: "http", protocol: "TCP", port: Number(form.port), targetPort: String(form.targetPort)}],
      }),
      {successMessage: `Service "${form.name}" created`}
    );
    setSubmitting(false);
    if (ok) {
      onClose();
    }
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={(next) => (next ? null : onClose())}
      title={`Expose Deployment: ${deploy?.name ?? ""}`}
      submitText="Create Service"
      submitting={submitting}
      onSubmit={handleSubmit}
    >
      <Field label="Service Name" htmlFor="expose-name" required error={errors.name}>
        <Input
          id="expose-name"
          value={form.name}
          onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
          placeholder="my-deployment"
        />
      </Field>

      <Field label="Type" required>
        <SimpleSelect
          value={form.type}
          onChange={(next) => setForm((prev) => ({...prev, type: next}))}
          options={SERVICE_TYPES}
        />
      </Field>

      <div className="flex flex-wrap gap-6">
        <Field label="Port (Service 端口)">
          <NumberInput
            value={form.port}
            onChange={(next) => setForm((prev) => ({...prev, port: next}))}
            min={1}
            max={65535}
          />
        </Field>
        <Field label="Target Port (Pod 容器端口)">
          <NumberInput
            value={form.targetPort}
            onChange={(next) => setForm((prev) => ({...prev, targetPort: next}))}
            min={1}
            max={65535}
          />
        </Field>
      </div>
    </FormDialog>
  );
}

const DOMAIN_PATTERN = /^[a-zA-Z0-9*]([a-zA-Z0-9\-.*]*[a-zA-Z0-9])?$/;

/** Points a hostname at one of the Deployment's Services via an Ingress. */
export function DeploymentDomainDialog({deploy, services, open, onClose, onCreated}) {
  const [form, setForm] = useState({host: "", path: "/", service: ""});
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);
  const [serviceOptions, setServiceOptions] = useState([]);

  useEffect(() => {
    if (!open || !deploy) {
      return;
    }
    const options = (services ?? [])
      .filter((service) => service.namespace === deploy.namespace)
      .flatMap((service) =>
        (service.ports ?? []).map((port) => ({
          label: `${service.name}:${port.port}`,
          value: `${service.name}|${port.port}`,
          serviceName: service.name,
        }))
      );
    setServiceOptions(options);

    const preferred = options.find((option) => option.serviceName === deploy.name) ?? options[0];
    setForm({host: "", path: "/", service: preferred?.value ?? ""});
    setErrors({});
  }, [open, deploy, services]);

  const hasServices = serviceOptions.length > 0;

  async function handleSubmit() {
    const nextErrors = {};
    if (!form.host) {
      nextErrors.host = "Domain is required";
    } else if (!DOMAIN_PATTERN.test(form.host)) {
      nextErrors.host = "Enter a valid domain, e.g. erp.company.internal";
    }
    if (!form.service) {
      nextErrors.service = "Please select a service";
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    const [serviceName, servicePort] = form.service.split("|");
    setSubmitting(true);
    const ok = await runAction(
      IngressBackend.addIngress({
        namespace: deploy.namespace,
        name: `${deploy.name}-domain`,
        ingressClass: "",
        rules: [
          {
            host: form.host,
            path: form.path ?? "/",
            pathType: "Prefix",
            serviceName,
            servicePort: Number(servicePort),
          },
        ],
      }),
      {successMessage: `Domain "${form.host}" bound to ${deploy.name}. Configure HTTPS from the Ingresses page.`}
    );
    setSubmitting(false);

    if (ok) {
      onCreated?.();
      onClose();
    }
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={(next) => (next ? null : onClose())}
      title={
        <span className="flex items-center gap-2">
          <Link2 className="text-info size-4" />
          Bind Domain — {deploy?.name ?? ""}
        </span>
      }
      submitText="Bind Domain"
      submitting={submitting}
      submitDisabled={!hasServices}
      onSubmit={handleSubmit}
    >
      {!hasServices ? (
        <MessageAlert
          variant="warning"
          title="No Service found for this deployment"
          description="Use the Expose button first to create a Service, then bind a domain."
        />
      ) : null}

      <Field
        label="Domain"
        htmlFor="domain-host"
        required
        error={errors.host}
        hint="Configure HTTPS afterwards via Ingresses → HTTPS."
      >
        <Input
          id="domain-host"
          value={form.host}
          onChange={(event) => setForm((prev) => ({...prev, host: event.target.value}))}
          placeholder="erp.company.com"
          disabled={!hasServices}
        />
      </Field>

      <Field label="Service" required error={errors.service} hint="The service and port traffic is forwarded to.">
        <SearchSelect
          value={form.service}
          onChange={(next) => setForm((prev) => ({...prev, service: next}))}
          options={serviceOptions}
          placeholder="Select service:port"
          disabled={!hasServices}
        />
      </Field>

      <Field label="Path" htmlFor="domain-path" hint="URL path prefix to match. Leave / to route all traffic.">
        <Input
          id="domain-path"
          value={form.path}
          onChange={(event) => setForm((prev) => ({...prev, path: event.target.value}))}
          placeholder="/"
          disabled={!hasServices}
        />
      </Field>
    </FormDialog>
  );
}

/** Rolls a Deployment onto a different tag of the image it already runs. */
export function DeploymentUpdateImageDialog({deploy, open, onClose, onUpdated}) {
  const [tags, setTags] = useState([]);
  const [tagsLoading, setTagsLoading] = useState(false);
  const [selectedTag, setSelectedTag] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const colonIndex = (deploy?.image ?? "").lastIndexOf(":");
  const repo = colonIndex > 0 ? deploy.image.slice(0, colonIndex) : deploy?.image ?? "";
  const currentTag = colonIndex > 0 ? deploy.image.slice(colonIndex + 1) : "latest";

  useEffect(() => {
    if (!open || !deploy) {
      return;
    }
    setTags([]);
    setSelectedTag("");
    setTagsLoading(true);
    PodBackend.getDockerHubImageTags((deploy.image ?? "").split(":")[0])
      .then((res) => {
        if (res.status === "ok") {
          setTags(res.data ?? []);
        } else {
          Setting.showMessage("error", res.msg);
        }
      })
      .catch((error) => Setting.showMessage("error", error.message))
      .finally(() => setTagsLoading(false));
  }, [open, deploy]);

  async function handleSubmit() {
    if (!selectedTag) {
      Setting.showMessage("error", "Please select a version");
      return;
    }
    const newImage = `${repo}:${selectedTag}`;
    setSubmitting(true);
    const ok = await runAction(DeploymentBackend.updateDeployment({...deploy, image: newImage}), {
      successMessage: `Updated to ${newImage}`,
    });
    setSubmitting(false);
    if (ok) {
      onClose();
      onUpdated?.();
    }
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={(next) => (next ? null : onClose())}
      title={deploy ? `Update Image — ${deploy.name}` : "Update Image"}
      submitText="Update"
      submitting={submitting}
      onSubmit={handleSubmit}
    >
      {deploy ? (
        <>
          <p className="text-muted-foreground flex items-center gap-2 text-sm">
            Image <span className="text-foreground font-medium">{repo}</span> · current
            <Badge variant="info">{currentTag}</Badge>
          </p>

          <Field label="Version">
            <SearchSelect
              value={selectedTag}
              onChange={setSelectedTag}
              options={tags.map((tag) => ({label: tag === currentTag ? `${tag} (current)` : tag, value: tag}))}
              placeholder={tagsLoading ? "Loading…" : "Select a version to update to"}
              emptyText={tagsLoading ? "Loading…" : "No tags found"}
              disabled={tagsLoading}
            />
          </Field>
        </>
      ) : null}
    </FormDialog>
  );
}
