import React, {useState} from "react";
import i18next from "i18next";
import {Plus, RefreshCw, Trash2, Zap} from "lucide-react";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import {runAction, useResource} from "@/hooks/use-resource";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {StatusBadge} from "@/components/shared/status-badge";

const NAME_PATTERN = /^[a-z0-9][a-z0-9-]*[a-z0-9]$/;

function NamespaceListPage() {
  const {data: namespaces, loading, error, refresh} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: []});

  const [dialogOpen, setDialogOpen] = useState(false);
  const [name, setName] = useState("");
  const [nameError, setNameError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  function openDialog() {
    setName("");
    setNameError("");
    setDialogOpen(true);
  }

  async function handleCreate() {
    if (!name) {
      setNameError("Name is required");
      return;
    }
    if (!NAME_PATTERN.test(name)) {
      setNameError("Must be lowercase alphanumeric with hyphens");
      return;
    }
    setSubmitting(true);
    const ok = await runAction(NamespaceBackend.addNamespace({name}), {successMessage: "Namespace created"});
    setSubmitting(false);
    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(namespaceName) {
    const ok = await runAction(NamespaceBackend.deleteNamespace(namespaceName), {successMessage: "Namespace deleted"});
    if (ok) {
      refresh();
    }
  }

  async function handleForceDelete(namespaceName) {
    const ok = await runAction(NamespaceBackend.forceDeleteNamespace(namespaceName), {
      successMessage: "Finalizers cleared — namespace will be removed shortly",
    });
    if (ok) {
      // The API returns as soon as the finalizers are patched; the namespace
      // itself disappears a moment later, so the list is re-read on a delay.
      setTimeout(refresh, 1500);
    }
  }

  const columns = [
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {
      key: "status",
      title: i18next.t("general:Status"),
      dataIndex: "status",
      width: 130,
      sortable: true,
      render: (value) => <StatusBadge status={value} />,
    },
    {key: "createdAt", title: i18next.t("general:Created"), dataIndex: "createdAt", width: 190, sortable: true},
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 150,
      align: "right",
      render: (_, record) =>
        record.status === "Terminating" ? (
          <ConfirmDialog
            title={`Force-delete namespace "${record.name}"?`}
            description="This clears finalizers and forces immediate removal."
            confirmText="Force Delete"
            onConfirm={() => handleForceDelete(record.name)}
          >
            <Button variant="outline" size="sm" className="text-destructive">
              <Zap />
              Force Delete
            </Button>
          </ConfirmDialog>
        ) : (
          <ConfirmDialog
            title={`Delete namespace "${record.name}"?`}
            description="All resources in this namespace will be deleted."
            confirmText="Delete"
            onConfirm={() => handleDelete(record.name)}
          >
            <Button variant="outline" size="sm" className="text-destructive">
              <Trash2 />
              {i18next.t("general:Delete")}
            </Button>
          </ConfirmDialog>
        ),
    },
  ];

  return (
    <PageContainer>
      {error ? <MessageAlert title="Failed to fetch namespaces" description={error} /> : null}

      <DataTable
        title={i18next.t("general:Namespaces")}
        description={`${namespaces?.length ?? 0} namespaces`}
        columns={columns}
        dataSource={namespaces}
        rowKey="name"
        loading={loading}
        searchable
        emptyText="No namespaces found"
        toolbar={
          <>
            <Button variant="outline" size="sm" onClick={() => refresh()} loading={loading}>
              <RefreshCw />
              {i18next.t("general:Refresh")}
            </Button>
            <Button size="sm" onClick={openDialog}>
              <Plus />
              {i18next.t("general:Add")}
            </Button>
          </>
        }
      />

      <FormDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        title="Add Namespace"
        submitText="Create"
        submitting={submitting}
        onSubmit={handleCreate}
      >
        <Field label={i18next.t("general:Name")} htmlFor="namespace-name" required error={nameError}>
          <Input
            id="namespace-name"
            value={name}
            onChange={(event) => {
              setName(event.target.value);
              setNameError("");
            }}
            placeholder="my-namespace"
            autoFocus
          />
        </Field>
      </FormDialog>
    </PageContainer>
  );
}

export default NamespaceListPage;
