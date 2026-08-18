import React, {useState} from "react";
import i18next from "i18next";
import {Pencil, Plus, RefreshCw, Trash2} from "lucide-react";
import * as ClusterRoleBindingBackend from "@/backend/ClusterRoleBindingBackend";
import {runAction, useResource} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {SimpleSelect} from "@/components/shared/simple-select";
import {SubjectBadges, SubjectsEditor, subjectsToRows} from "@/components/shared/subjects-editor";

const ROLE_REF_KINDS = [
  {label: "ClusterRole", value: "ClusterRole"},
  {label: "Role", value: "Role"},
];

const emptyForm = {name: "", roleRef: "", roleRefKind: "ClusterRole", subjects: []};

function ClusterRoleBindingListPage() {
  const {
    data: bindings,
    loading,
    error,
    refresh,
  } = useResource(() => ClusterRoleBindingBackend.getClusterRoleBindings(), [], {initialData: []});

  const [dialogOpen, setDialogOpen] = useState(false);
  const [mode, setMode] = useState("add");
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);

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
      roleRef: record.roleRef,
      roleRefKind: "ClusterRole",
      subjects: subjectsToRows(record.subjects),
    });
    setErrors({});
    setDialogOpen(true);
  }

  async function handleSubmit() {
    const nextErrors = {};
    if (!form.name) {
      nextErrors.name = "Required";
    }
    if (!form.roleRef) {
      nextErrors.roleRef = "Required";
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    const subjects = form.subjects.filter((subject) => subject && subject.name);
    setSubmitting(true);
    const ok =
      mode === "add"
        ? await runAction(
          ClusterRoleBindingBackend.addClusterRoleBinding({
            name: form.name,
            roleRef: form.roleRef,
            roleRefKind: form.roleRefKind || "ClusterRole",
            subjects,
          }),
          {successMessage: "ClusterRoleBinding created"}
        )
        : await runAction(
          ClusterRoleBindingBackend.updateClusterRoleBinding({
            name: editing.name,
            roleRef: editing.roleRef,
            subjects,
            resourceVersion: editing.resourceVersion,
          }),
          {successMessage: "ClusterRoleBinding updated"}
        );
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(ClusterRoleBindingBackend.deleteClusterRoleBinding(record.name), {
      successMessage: "ClusterRoleBinding deleted",
    });
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {
      key: "roleRef",
      title: "Role Ref",
      dataIndex: "roleRef",
      width: 230,
      sortable: true,
      render: (value) => <Badge variant="danger">{value}</Badge>,
    },
    {
      key: "subjects",
      title: "Subjects",
      dataIndex: "subjects",
      render: (subjects) => <SubjectBadges subjects={subjects} />,
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
            title={`Delete ClusterRoleBinding "${record.name}"?`}
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
      {error ? <MessageAlert title="Failed to fetch ClusterRoleBindings" description={error} /> : null}

      <DataTable
        title={i18next.t("general:ClusterRoleBindings")}
        description={`${bindings?.length ?? 0} bindings`}
        columns={columns}
        dataSource={bindings}
        rowKey="name"
        loading={loading}
        searchable
        emptyText="No ClusterRoleBindings found"
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
        title={mode === "add" ? "Add Cluster Role Binding" : "Edit Cluster Role Binding"}
        description={mode === "edit" ? "roleRef is immutable after creation — only subjects can be updated." : undefined}
        submitText={mode === "add" ? "Create" : "Update"}
        submitting={submitting}
        onSubmit={handleSubmit}
        size="lg"
      >
        <Field label={i18next.t("general:Name")} htmlFor="crb-name" required error={errors.name}>
          <Input
            id="crb-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-binding"
            disabled={mode === "edit"}
          />
        </Field>

        <div className="grid grid-cols-[40%_minmax(0,1fr)] gap-3">
          <Field label="Role Ref Kind">
            <SimpleSelect
              value={form.roleRefKind}
              onChange={(next) => setForm((prev) => ({...prev, roleRefKind: next}))}
              options={ROLE_REF_KINDS}
              disabled={mode === "edit"}
            />
          </Field>
          <Field label="Role Ref Name" htmlFor="crb-roleref" required error={errors.roleRef}>
            <Input
              id="crb-roleref"
              value={form.roleRef}
              onChange={(event) => setForm((prev) => ({...prev, roleRef: event.target.value}))}
              placeholder="cluster-admin"
              disabled={mode === "edit"}
            />
          </Field>
        </div>

        <Field label="Subjects">
          <SubjectsEditor value={form.subjects} onChange={(subjects) => setForm((prev) => ({...prev, subjects}))} />
        </Field>
      </FormDialog>
    </PageContainer>
  );
}

export default ClusterRoleBindingListPage;
