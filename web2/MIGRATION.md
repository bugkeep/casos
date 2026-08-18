# web → web2 migration notes

`web2/` is the shadcn/ui rewrite of the Ant Design frontend in `web/`. Both talk
to the same Beego backend and can run side by side.

**Status: complete.** All 37 route pages and every drawer, modal and editor they
depend on are ported; `web2/src` contains no Ant Design import. The remaining
mentions of antd in the source are comments naming what each component replaces.

## Stack

| | web (old) | web2 (new) |
|---|---|---|
| Build | CRA + craco | Vite 6 |
| UI kit | Ant Design 6 | shadcn/ui (Radix + Tailwind v4) |
| Icons | @ant-design/icons | lucide-react |
| Tables | antd `Table` | `@tanstack/react-table` via `DataTable` |
| Toasts | antd `message` | sonner, behind `Setting.showMessage` |
| Router | react-router-dom 5 | react-router-dom 5 (unchanged, so route code ports 1:1) |
| Language | JavaScript | JavaScript (JSX) |

## What was copied unchanged

- `src/backend/*.js` — 32 API clients, byte-for-byte. They are plain `fetch`
  wrappers with no UI dependency, so **backend integration is identical to the
  old frontend by construction**. Do not rewrite these during the port.
- `src/i18n.js`, `src/locales/{en,zh}/data.json`.

`src/Setting.js` was ported rather than copied: the antd `message` and `theme`
imports became sonner and a `.dark` class toggle. Every other exported function
kept its name and signature, so `Setting.*` call sites port unchanged.

## Dev loop

```bash
cd web2 && yarn install && yarn start
```

Vite serves on **8002** and proxies `/api`, `/k8s` and `/.well-known` to the Go
backend on `:9000`, websockets included (the pod terminal needs that). Because
the proxy handles it, `Setting.ServerUrl` stays empty and every request is
relative — the same code path used in production, where the backend serves the
built bundle itself.

## Component contract

Written against these, not against raw Radix. Page code should rarely import
from `components/ui/` directly except for `Button`, `Input`, `Badge`.

### `DataTable` — `components/shared/data-table.jsx`

The list-page workhorse. Columns are declared antd-style so column definitions
port across with almost no edits:

```js
{key, title, dataIndex, render(value, record, index), width, align,
 sortable, ellipsis, className, headerClassName}
```

`dataIndex` may be a dotted path; a column without one is a display column
(actions) and cannot sort. Table props: `dataSource`, `rowKey` (string or
function), `loading`, `title`, `description`, `toolbar`, `searchable`,
`pageSize` (0 disables pagination), `emptyText`, `onRowClick`, `expandable`,
`dense`. Pagination only renders once the rows overflow a page.

### Dialogs

- `FormDialog` + `Field` — create/edit modals. `FormDialog` owns the chrome and
  submit state; `Field` is label + control + error text.
- `ConfirmDialog` — replaces antd `Popconfirm`. Trigger is passed as children;
  the confirm button is destructive by default.
- `ResourceSheet` — replaces antd `Drawer` (logs, terminal, files, history).

### Data fetching — `hooks/use-resource.js`

```js
const {data, loading, error, refresh} = useResource(
  () => PodBackend.getPods(), [], {initialData: []});
```

Handles the `{status, data, msg}` envelope, toasts failures, and drops responses
that arrive after unmount or after a newer request. `runAction(promise, {...})`
is the mutation counterpart and resolves to a boolean so callers close a dialog
only on success.

### Other shared pieces

`StatusBadge` / `ReadyBadge` / `ColorTag` (`tagVariant()` maps antd colour names
to badge variants), `KeyValueEditor` + `StringListEditor` (+ `toEntries` /
`fromEntries`), `EnvVarEditor`, `RestartDeploymentsDialog`, `SimpleSelect` /
`SearchSelect`, `NumberInput`, `PasswordInput`, `StatCard`, `EmptyState`,
`Loading` / `AiDots`, `CodeText` / `CodeBlock` / `CopyButton`,
`DescriptionList`, `ResultScreen`, `Space`, `PageContainer` / `PageHeader`.

### Navigation

`src/nav.js` is the single description of the sidebar tree, read by both
`AppSidebar` and `BreadcrumbBar`. The old UI kept that mapping in two places and
they drifted. `src/routes.jsx` holds the route table; every page is lazy-loaded.

## Porting a page

1. Replace the antd class component with a function component.
2. `fetchX()` + loading/error state → `useResource`.
3. Mutations → `runAction`.
4. antd `Table` → `DataTable` (columns usually survive as-is).
5. antd `Modal` + `Form` → `FormDialog` + `Field` with plain `useState`.
6. antd `Popconfirm` → `ConfirmDialog`; `Drawer` → `ResourceSheet`;
   `Tag` → `Badge` / `StatusBadge`; `message` → `Setting.showMessage`.
7. Add the route to `src/routes.jsx`.

## Not carried over

- The Playwright suite in `web/tests` still targets the antd UI; its selectors do
  not match this DOM. Porting it is separate work.
- `-tags embed` builds still ship `web/build`. See `web2/assets_embed.go` for
  what to change when web2 becomes the shipped frontend.

## Serving from the Go backend

`web2/assets_disk.go` and `web2/assets_embed.go` mirror the pair in `web/`.
`routers/static_filter.go` serves `web2/build` when it exists and falls back to
`web/build`, so the two frontends can be swapped without a code change.
