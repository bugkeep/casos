<div align="center">

# CasOS

**A cloud operating system built on Kubernetes**

[![Build](https://github.com/casosorg/casos/workflows/Build/badge.svg?style=flat-square)](https://github.com/casosorg/casos/actions/workflows/build.yml)
[![Release](https://img.shields.io/github/v/release/casosorg/casos?style=flat-square&color=4f46e5)](https://github.com/casosorg/casos/releases/latest)
[![Go Report](https://goreportcard.com/badge/github.com/casosorg/casos?style=flat-square)](https://goreportcard.com/report/github.com/casosorg/casos)
[![License](https://img.shields.io/github/license/casosorg/casos?style=flat-square&color=22c55e)](https://github.com/casosorg/casos/blob/master/LICENSE)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20Windows-blue?style=flat-square)](https://github.com/casosorg/casos/releases/latest)
[![Discord](https://img.shields.io/discord/1022748306096537660?logo=discord&label=discord&color=5865F2&style=flat-square)](https://discord.gg/6ma4BAmV7P)

**Official Website: [https://www.casos.net](https://www.casos.net)**

**Live Demo: [https://demo.casos.net](https://demo.casos.net)**

</div>

---

## What is CasOS?

CasOS is a cloud operating system built on Kubernetes. It embeds the Kubernetes API server, controller manager, and scheduler, so you do **not** need an existing Kubernetes cluster or a separate control plane — CasOS is the platform itself. Run a single binary and get a fully functional cloud OS with a built-in web UI.

## Features

- Embedded Kubernetes stack (API server, controller manager, scheduler) — no external cluster needed
- Cluster resource management: Nodes, Namespaces, Pods, Services, ConfigMaps, ServiceAccounts, ClusterRoleBindings
- Dashboard with cluster overview
- DockerHub image browser
- Multi-language support (i18n)
- Authentication via [Casdoor](https://casdoor.org)

## Tech Stack

| Layer    | Technology                                |
|----------|-------------------------------------------|
| Backend  | Go 1.26+, Beego, MySQL (ORM)              |
| Frontend | React 18, Ant Design 6, recharts, i18next |
| Auth     | Casdoor (OAuth2 / OIDC)                   |

## Project Structure

```
casos/
├── main.go                  # Entry point
├── conf/app.conf            # Backend configuration
├── controllers/             # HTTP controllers (Beego routing)
├── object/                  # Business logic and data models
├── routers/                 # Route configuration and filters
├── proxy/                   # kube-proxy related logic
└── web/                     # React frontend
    └── src/
        ├── App.js
        ├── DashboardPage.js
        ├── ManagementPage.js
        ├── PodListPage.js
        ├── NodeListPage.js
        ├── NamespaceListPage.js
        ├── ServiceListPage.js
        ├── ConfigMapListPage.js
        ├── ServiceAccountListPage.js
        ├── ClusterRoleBindingListPage.js
        └── backend/         # API client helpers
```

## Prerequisites

- **Backend**: [Go](https://golang.org/dl/) 1.26+
- **Frontend**: [Node.js](https://nodejs.org/) 20+ and [Yarn](https://classic.yarnpkg.com/) 1.x
- A [Casdoor](https://casdoor.org) instance (for authentication)

Standalone releases are currently supported on **Linux** and **Windows** for
amd64 and arm64.

## Install

Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.sh | bash
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.ps1 | iex
```

The installers select the release binary for the current OS and architecture,
verify it against the published `SHA256SUMS`, install it for the current user,
and start CasOS at `http://127.0.0.1:9000`. On Linux, the default binary
location is `~/.local/bin/casos`, separate from the data directory.
On Windows, exit a running CasOS process before rerunning the installer to
upgrade it; the installer refuses to replace an executable that is in use.

Set `CASOS_VERSION` to an explicit release tag to select the downloaded binary
version. The installer script itself still comes from the branch or commit in
the script URL, so pin that URL as well when a fully reproducible historical
installation is required. `SHA256SUMS` detects download corruption; it is not a
substitute for release signing, provenance, or artifact attestation.

A standalone installation without `conf/app.conf` stores its persistent data in
the platform user-data directory:

| Platform | Default data directory |
|----------|------------------------|
| Linux | `$XDG_DATA_HOME/casos`, or `~/.local/share/casos` when `XDG_DATA_HOME` is unset |
| Windows | `%LOCALAPPDATA%\CasOS` |

Set `CASOS_DATA_DIR` before starting CasOS to use a different standalone data
directory. Keep that value stable across restarts so CasOS continues to use the
same database, Kubernetes state, TLS material, and sessions. A newly created
standalone data root and its managed subdirectories use mode `0700` on Unix;
existing explicitly managed directory permissions are preserved.

## Configuration

Edit `conf/app.conf` with your values:

```ini
appname       = casos
httpport      = 9000
httpBind      =
runmode       = dev

; Database
driverName    = sqlite
dataSourceName=
dbName        = casos
kineEndpoint  =

; Casdoor
casdoorEndpoint     = https://your-casdoor-instance
clientId            = <your-client-id>
clientSecret        = <your-client-secret>
casdoorOrganization = <your-org>
casdoorApplication  = <your-app>

; Optional control-plane SOCKS5 proxy
; Leave blank to use environment proxy settings or direct access.
; When set, the proxy is required for requests not matched by NO_PROXY.
socks5Proxy =

; Kubernetes control plane
apiserverPort = 6443
apiserverBind = 127.0.0.1
dataDir       = ./data
storageProvisionerEnabled = true
localPathRoot =
```

Source and configured builds keep the historical empty `httpBind` default and
listen on all interfaces. Standalone binaries built with `-tags embed` default
to `127.0.0.1`. Set `httpBind` explicitly to override the default for either
build type. The supported frontend development origin
`http://localhost:8001` may access the backend at `http://localhost:9000`;
other browser origins must match the effective backend origin.

When running from the repository, the checked-in `conf/app.conf` explicitly
sets `dataDir=./data`. SQLite therefore stores CasOS business data in
`data/casos.db` and Kubernetes state in `data/kine/state.db`. The `data`
directory is ignored by Git and is writable when running the development
command from the repository root. This configured development behavior is
separate from the standalone defaults documented above.

The built-in local-path provisioner defaults to disabled on Windows standalone
binaries because `%LOCALAPPDATA%\CasOS` is a Windows control-plane path, not a
valid storage root on Linux worker nodes. To enable it explicitly, set
`storageProvisionerEnabled=true` and set `localPathRoot` to an absolute POSIX
path available on the Linux workers, for example
`/var/lib/casos/local-path-provisioner`. Linux and configured/source builds keep
the existing enabled default; when `localPathRoot` is blank, they derive it from
`dataDir` as before.

> **Breaking change:** `dataDir` used to default to `/var/lib/casos`. It now
> defaults to `./data` for deployments that use `conf/app.conf`. That relative
> path is resolved against the working directory of the
> CasOS process, so starting CasOS from a different directory points it at a
> different — and most likely empty — data directory. Besides the two SQLite
> databases, `dataDir` also holds the control-plane TLS material and the key
> that encrypts stored SSH private keys, so a service that loses track of it
> can no longer decrypt existing machine credentials. Set `dataDir` to an
> absolute path such as `/var/lib/casos` for any system installation, make sure
> the service user can write to it, and move an existing `/var/lib/casos` to
> the new location before restarting. CasOS logs the directory it resolved as
> `casos data directory: <path>` on startup.

MySQL remains available by setting `driverName=mysql`, configuring
`dataSourceName` and `dbName`, and optionally setting a complete
`kineEndpoint`. Switching an existing installation from MySQL to SQLite starts
with empty SQLite databases; existing MySQL data is not imported automatically.

The control-plane proxy accepts `host:port`, `socks5://`, and `socks5h://`
addresses. When `socks5Proxy` is set, CasOS fails requests that cannot use the
configured proxy instead of silently falling back to direct access. Destinations
matched by the CasOS process `NO_PROXY` environment variable continue to use
direct access. When `socks5Proxy` is blank, CasOS follows `HTTP_PROXY`,
`HTTPS_PROXY`, and `NO_PROXY`, and uses direct access when no environment proxy
is configured.

**Upgrade notice:** the previous example default of `127.0.0.1:10808` has been
removed. Set `socks5Proxy` explicitly before upgrading if that local proxy is a
required control-plane dependency.

## Development

### Backend

```bash
go run main.go
```

### Frontend

```bash
cd web

# Install dependencies (first time only)
yarn install

# Start dev server — port 8001, proxies API to localhost:9000
yarn start
```

## Deployment

### Backend

```bash
# Build binary
go build -o casos .

# Run
./casos
```

### Frontend

```bash
cd web

# Production build (outputs to web/build/)
yarn build
```

Serve the `web/build/` directory with any static file server, or let the Go backend serve it directly.

### Lint

```bash
cd web

yarn lint:js    # ESLint
yarn lint:css   # Stylelint
yarn lint       # both
```

## License

[Apache 2.0](https://github.com/casosorg/casos/blob/master/LICENSE)
