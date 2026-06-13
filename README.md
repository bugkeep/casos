# casos

**casos** is an in-process Kubernetes control plane server built with Go. It embeds a full Kubernetes API server (via the k3s fork) and backs it with MySQL through [kine](https://github.com/k3s-io/kine), exposing a unified HTTP gateway that serves both the Kubernetes API and a React web UI.

## Architecture

```
                    ┌─────────────────────────────────────┐
                    │          Gateway (port 9000)         │
                    │  /k8s/*  → Kubernetes API server     │
                    │  /api/*  → Beego REST API            │
                    │  /*      → React static files        │
                    └────────────┬────────────────────────┘
                                 │
              ┌──────────────────┼──────────────────┐
              │                  │                  │
     ┌────────▼───────┐ ┌────────▼───────┐ ┌───────▼────────┐
     │  Kubernetes     │ │  Beego REST    │ │  React UI      │
     │  API Server     │ │  API (9090)    │ │  (web/build/)  │
     │  (6443)         │ │                │ │                │
     └────────┬───────┘ └────────┬───────┘ └────────────────┘
              │                  │
              └────────┬─────────┘
                       │
              ┌────────▼───────┐
              │   MySQL / kine  │
              └────────────────┘
```

## Features

- **Embedded Kubernetes control plane** — runs an API server in-process; no separate cluster required
- **MySQL backend** — uses kine to translate etcd protocol to MySQL, so no etcd deployment is needed
- **Unified gateway** — a single port routes Kubernetes API calls, REST API calls, and static assets
- **Beego REST API** — manages Kubernetes resources (Pods, ConfigMaps, etc.) through a conventional HTTP API
- **React web UI** — served directly from the gateway

## Prerequisites

- Go 1.21+
- MySQL 8.0+
- Node.js (to build the web UI)

## Configuration

Copy and edit `conf/app.conf`:

```ini
appname       = casos
httpport      = 9090        ; internal Beego port (not exposed directly)
runmode       = dev

; Database
driverName    = mysql
dataSourceName= user:pass@tcp(host:3306)/
dbName        = casos

; Unified gateway
gatewayPort   = 9000        ; public-facing port

; Control plane
apiserverPort = 6443
apiserverBind = 127.0.0.1
dataDir       = /var/lib/casos
```

## Running

```bash
# Build
go build -o casos .

# Run (reads conf/app.conf)
./casos
```

The server starts the Kubernetes API server, then Beego, then the gateway. Once ready:

- Web UI: `http://localhost:9000`
- REST API: `http://localhost:9000/api/`
- Kubernetes API: `https://localhost:9000/k8s/` (or directly at `https://localhost:6443`)

## Project Structure

```
casos/
├── conf/           # Application configuration (app.conf)
├── controllers/    # Beego controllers (pod, configmap, etc.)
├── object/         # Database models and initialization
├── proxy/          # Unified HTTP gateway
├── routers/        # Beego route registration
├── server/         # Kubernetes control plane bootstrap
├── util/           # Shared utilities
├── web/            # React frontend (build/ served by gateway)
└── main.go
```

## License

See [LICENSE](LICENSE).
