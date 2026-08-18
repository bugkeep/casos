import React from "react";
import {Link, useHistory} from "react-router-dom";
import i18next from "i18next";
import {Pencil, Trash2} from "lucide-react";
import * as SiteBackend from "@/backend/SiteBackend";
import {runAction, useResource} from "@/hooks/use-resource";
import {Button} from "@/components/ui/button";
import {SimpleTooltip} from "@/components/ui/tooltip";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {PageContainer} from "@/components/shared/page-header";

const BUILT_IN_SITE = "site-built-in";

function SiteListPage() {
  const history = useHistory();
  const {data: sites, setData: setSites, loading} = useResource(() => SiteBackend.getGlobalSites(), [], {initialData: []});

  async function handleDelete(record) {
    const ok = await runAction(SiteBackend.deleteSite(record), {
      successMessage: i18next.t("general:Successfully deleted"),
    });
    if (ok) {
      setSites((previous) => previous.filter((site) => site.name !== record.name));
    }
  }

  const columns = [
    {
      key: "name",
      title: i18next.t("general:Name"),
      dataIndex: "name",
      width: 220,
      sortable: true,
      render: (value) => (
        <Link to={`/sites/${value}`} className="text-info font-medium hover:underline">
          {value}
        </Link>
      ),
    },
    {key: "displayName", title: i18next.t("general:Display name"), dataIndex: "displayName", width: 220, sortable: true},
    {
      key: "themeColor",
      title: i18next.t("site:Theme color"),
      dataIndex: "themeColor",
      width: 150,
      render: (value) => (
        <span className="flex items-center gap-2">
          <span className="size-4 rounded-sm border" style={{backgroundColor: value}} />
          <span className="font-mono text-xs">{value}</span>
        </span>
      ),
    },
    {
      key: "action",
      title: i18next.t("general:Action"),
      width: 140,
      align: "right",
      render: (_, record) => (
        <div className="flex justify-end gap-1">
          <SimpleTooltip title={i18next.t("general:Edit")}>
            <Button variant="ghost" size="icon-sm" onClick={() => history.push(`/sites/${record.name}`)} aria-label="Edit">
              <Pencil className="size-4" />
            </Button>
          </SimpleTooltip>
          <ConfirmDialog
            title={`${i18next.t("general:Sure to delete")}: ${record.name} ?`}
            confirmText={i18next.t("general:OK")}
            cancelText={i18next.t("general:Cancel")}
            onConfirm={() => handleDelete(record)}
            disabled={record.name === BUILT_IN_SITE}
          >
            <Button
              variant="ghost"
              size="icon-sm"
              className="text-muted-foreground hover:text-destructive"
              disabled={record.name === BUILT_IN_SITE}
              aria-label="Delete"
            >
              <Trash2 className="size-4" />
            </Button>
          </ConfirmDialog>
        </div>
      ),
    },
  ];

  return (
    <PageContainer>
      <DataTable
        testId="sites-table"
        title={i18next.t("general:Sites")}
        columns={columns}
        dataSource={sites}
        rowKey="name"
        loading={loading}
        searchable
        emptyText="No sites"
        toolbar={
          <SimpleTooltip title="Sites are provisioned by the server">
            <span>
              <Button size="sm" disabled>
                {i18next.t("general:Add")}
              </Button>
            </span>
          </SimpleTooltip>
        }
      />
    </PageContainer>
  );
}

export default SiteListPage;
