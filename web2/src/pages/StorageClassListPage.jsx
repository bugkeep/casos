import React, {useState} from "react";
import i18next from "i18next";
import {Pencil, Plus, RefreshCw, Trash2} from "lucide-react";
import * as StorageClassBackend from "@/backend/StorageClassBackend";
import {runAction, useResource} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {Switch} from "@/components/ui/switch";
import {Label} from "@/components/ui/label";
import {MessageAlert} from "@/components/ui/alert";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {SimpleSelect} from "@/components/shared/simple-select";
import {KeyValueEditor, fromEntries, toEntries} from "@/components/shared/key-value-editor";

const RECLAIM_POLICIES = ["Delete", "Retain"].map((policy) => ({label: policy, value: policy}));
const VOLUME_BINDING_MODES = ["Immediate", "WaitForFirstConsumer"].map((mode) => ({label: mode, value: mode}));

const emptyForm = {
  name: "",
  provisioner: "",
  reclaimPolicy: "Delete",
  volumeBindingMode: "Immediate",
  allowVolumeExpansion: false,
  isDefault: false,
  parameters: [],
};

function StorageClassListPage() {
  const {
    data: storageClasses,
    loading,
    error,
    refresh,
  } = useResource(() => StorageClassBackend.getStorageClasses(), [], {initialData: []});

  const [dialogOpen, setDialogOpen] = useState(false);
  const [mode, setMode] = useState("add");
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);

  const isEdit = mode === "edit";

  function openAdd() {
    setMode("add");
    setEditing(null);
    setForm(emptyForm);
    setErrors({});
    setDialogOpen(true);
  }

  function openEdit(record) {
    setMode("edit");
    setEditing(record);
    setForm({
      name: record.name,
      provisioner: record.provisioner,
      reclaimPolicy: record.reclaimPolicy || "Delete",
      volumeBindingMode: record.volumeBindingMode || "Immediate",
      allowVolumeExpansion: Boolean(record.allowVolumeExpansion),
      isDefault: Boolean(record.isDefault),
      parameters: toEntries(record.parameters),
    });
    setErrors({});
    setDialogOpen(true);
  }

  async function handleSubmit() {
    const nextErrors = {};
    if (!form.name) {
      nextErrors.name = "Required";
    }
    if (!form.provisioner) {
      nextErrors.provisioner = "Required";
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    setSubmitting(true);
    const ok = isEdit
      ? // Provisioner, binding mode and parameters are immutable, so an update
      // resends the stored values and only carries the editable fields.
      await runAction(
        StorageClassBackend.updateStorageClass({
          name: editing.name,
          provisioner: editing.provisioner,
          reclaimPolicy: form.reclaimPolicy,
          volumeBindingMode: editing.volumeBindingMode,
          allowVolumeExpansion: Boolean(form.allowVolumeExpansion),
          isDefault: Boolean(form.isDefault),
          parameters: editing.parameters ?? {},
          resourceVersion: editing.resourceVersion,
        }),
        {successMessage: "Storage Class updated"}
      )
      : await runAction(
        StorageClassBackend.addStorageClass({
          name: form.name,
          provisioner: form.provisioner,
          reclaimPolicy: form.reclaimPolicy,
          volumeBindingMode: form.volumeBindingMode,
          allowVolumeExpansion: Boolean(form.allowVolumeExpansion),
          isDefault: Boolean(form.isDefault),
          parameters: fromEntries(form.parameters),
        }),
        {successMessage: "Storage Class created"}
      );
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(StorageClassBackend.deleteStorageClass(record.name), {
      successMessage: "Storage Class deleted",
    });
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {
      key: "name",
      title: i18next.t("general:Name"),
      dataIndex: "name",
      sortable: true,
      render: (value, record) => (
        <span className="flex items-center gap-2">
          <span className="font-medium">{value}</span>
          {record.isDefault ? <Badge variant="warning">Default</Badge> : null}
        </span>
      ),
    },
    {key: "provisioner", title: "Provisioner", dataIndex: "provisioner", sortable: true},
    {
      key: "reclaimPolicy",
      title: "Reclaim Policy",
      dataIndex: "reclaimPolicy",
      width: 150,
      sortable: true,
      render: (value) => <Badge variant={value === "Retain" ? "warning" : "info"}>{value}</Badge>,
    },
    {key: "volumeBindingMode", title: "Volume Binding Mode", dataIndex: "volumeBindingMode", width: 200},
    {
      key: "allowVolumeExpansion",
      title: "Expandable",
      dataIndex: "allowVolumeExpansion",
      width: 130,
      render: (value) => <Badge variant={value ? "success" : "muted"}>{value ? "Yes" : "No"}</Badge>,
    },
    {key: "createdAt", title: i18next.t("general:Created"), dataIndex: "createdAt", width: 190, sortable: true},
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 170,
      align: "right",
      render: (_, record) => (
        <div className="flex justify-end gap-2">
          <Button variant="outline" size="sm" onClick={() => openEdit(record)}>
            <Pencil />
            {i18next.t("general:Edit")}
          </Button>
          <ConfirmDialog
            title={`Delete Storage Class "${record.name}"?`}
            confirmText="Delete"
            onConfirm={() => handleDelete(record)}
          >
            <Button variant="outline" size="sm" className="text-destructive">
              <Trash2 />
            </Button>
          </ConfirmDialog>
        </div>
      ),
    },
  ];

  return (
    <PageContainer>
      {error ? <MessageAlert title="Failed to fetch Storage Classes" description={error} /> : null}

      <DataTable
        title={i18next.t("general:Storage Classes")}
        description={`${storageClasses?.length ?? 0} storage classes`}
        columns={columns}
        dataSource={storageClasses}
        rowKey="name"
        loading={loading}
        searchable
        emptyText="No Storage Classes found"
        toolbar={
          <>
            <Button variant="outline" size="sm" onClick={() => refresh()} loading={loading}>
              <RefreshCw />
              {i18next.t("general:Refresh")}
            </Button>
            <Button size="sm" onClick={openAdd}>
              <Plus />
              {i18next.t("general:Add")}
            </Button>
          </>
        }
      />

      <FormDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        title={isEdit ? "Edit Storage Class" : "Add Storage Class"}
        description={isEdit ? "Provisioner, volume binding mode and parameters are immutable after creation." : undefined}
        submitText={isEdit ? "Update" : "Create"}
        submitting={submitting}
        onSubmit={handleSubmit}
        size="lg"
      >
        <Field label={i18next.t("general:Name")} htmlFor="sc-name" required error={errors.name}>
          <Input
            id="sc-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-storage-class"
            disabled={isEdit}
          />
        </Field>

        <Field label="Provisioner" htmlFor="sc-provisioner" required error={errors.provisioner}>
          <Input
            id="sc-provisioner"
            value={form.provisioner}
            onChange={(event) => setForm((prev) => ({...prev, provisioner: event.target.value}))}
            placeholder="casos.io/local-path-provisioner"
            disabled={isEdit}
          />
        </Field>

        <div className="grid grid-cols-2 gap-3">
          <Field label="Reclaim Policy">
            <SimpleSelect
              value={form.reclaimPolicy}
              onChange={(next) => setForm((prev) => ({...prev, reclaimPolicy: next}))}
              options={RECLAIM_POLICIES}
            />
          </Field>
          <Field label="Volume Binding Mode">
            <SimpleSelect
              value={form.volumeBindingMode}
              onChange={(next) => setForm((prev) => ({...prev, volumeBindingMode: next}))}
              options={VOLUME_BINDING_MODES}
              disabled={isEdit}
            />
          </Field>
        </div>

        <div className="flex flex-wrap gap-8">
          <div className="flex items-center gap-2">
            <Switch
              id="sc-expansion"
              checked={form.allowVolumeExpansion}
              onCheckedChange={(next) => setForm((prev) => ({...prev, allowVolumeExpansion: next}))}
            />
            <Label htmlFor="sc-expansion">Allow Volume Expansion</Label>
          </div>
          <div className="flex items-center gap-2">
            <Switch
              id="sc-default"
              checked={form.isDefault}
              onCheckedChange={(next) => setForm((prev) => ({...prev, isDefault: next}))}
            />
            <Label htmlFor="sc-default">Set As Default</Label>
          </div>
        </div>

        <Field label="Parameters">
          <KeyValueEditor
            value={form.parameters}
            onChange={(parameters) => setForm((prev) => ({...prev, parameters}))}
            addLabel="Add Parameter"
            disabled={isEdit}
          />
        </Field>
      </FormDialog>
    </PageContainer>
  );
}

export default StorageClassListPage;
