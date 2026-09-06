# Health Monitor

A device monitoring service written in Go that periodically probes a fleet of
network devices (switches, routers, access points) over both **gRPC** and
**REST**, collects health status and diagnostics, and exposes the results to
consumers via gRPC and REST APIs.

## Features

- **Dual-protocol device probing** — each device is monitored over gRPC or REST
  depending on its configuration.
- **Concurrent probe collector** — runs on a configurable interval with a
  bounded concurrency limit.
- **Capability discovery & diagnostics** — collects hardware, software, and
  firmware versions plus status and checksum when the device advertises support.
- **Dual API surface** — consumer-facing gRPC (`MonitorService`) and REST
  (`/api/v1`) APIs.
- **Structured logging** — `log/slog` throughout, including per-request gRPC/REST
  logging and probe lifecycle events.
- **Graceful shutdown** — responds to `SIGINT`/`SIGTERM`.

## Architecture

```
cmd/monitor            Application entry point; wires config, store, service, servers, collector
internal/config        YAML configuration loading with defaults
internal/model         Domain models (Device, Diagnostics, ProbeResult)
internal/store         Thread-safe in-memory store
internal/service       Business logic / orchestration, model <-> protobuf conversion
internal/server        REST + gRPC API servers (with request logging)
internal/probe         gRPC/REST device probers + background collector
tools/mockdevice       Development mock device (REST or gRPC)
proto/monitor/v1       Protobuf service definitions
configs/devices.yaml   Sample configuration
```

## Prerequisites

- Go 1.25+
- `protoc` and the Go protobuf plugins (only required to regenerate code)

## Getting Started

### 1. Build

```sh
make build
```

This produces `bin/monitor`.

### 2. Run

```sh
make run
```

or with a custom config:

```sh
./bin/monitor -config configs/devices.yaml
```

### 3. Run the mock device

To test end-to-end you can start a mock device (REST or gRPC):

```sh
go run ./tools/mockdevice -protocol rest -addr :8080
go run ./tools/mockdevice -protocol grpc -addr :50051
```

Then point the monitor at it via config or the `AddDevice` API.

## Configuration

Configuration is provided as YAML, with sensible defaults for anything omitted.

```yaml
server:
  grpc_addr: ":9090"    # gRPC API listen address
  rest_addr: ":8080"    # REST API listen address

collector:
  interval: 30s         # probe interval
  grpc_timeout: 5s      # per-device gRPC probe timeout
  rest_timeout: 5s      # per-device REST probe timeout
  max_concurrent: 10    # max simultaneous probes

devices:
  - id: switch-01
    name: Core Switch 01
    address: 192.168.1.10:50051
    protocol: grpc        # grpc | rest
  - id: router-01
    name: Edge Router 01
    address: 192.168.1.20:8080
    protocol: rest
```

## REST API

| Method | Path                              | Description                    |
|--------|-----------------------------------|--------------------------------|
| GET    | `/api/v1/devices`                 | List all devices               |
| GET    | `/api/v1/devices/{id}`            | Get one device                 |
| GET    | `/api/v1/devices/{id}/diagnostics`| Get a device's diagnostics     |
| POST   | `/api/v1/devices`                 | Register a new device          |
| DELETE | `/api/v1/devices/{id}`            | Remove a device                |

### Example

```sh
# List devices
curl -s localhost:8080/api/v1/devices

# Add a device
curl -s -X POST localhost:8080/api/v1/devices \
  -H "Content-Type: application/json" \
  -d '{"id":"ap-02","name":"Access Point 02","address":"192.168.1.31:8080","protocol":"rest"}'
```

## gRPC API

The consumer-facing service is `MonitorService` (default port `:9090`):

- `ListDevices`
- `GetDevice`
- `GetDiagnostics`
- `AddDevice`
- `RemoveDevice`

A standard gRPC health service and reflection are also registered, so tools like
`grpcurl` can discover the schema:

```sh
grpcurl -plaintext localhost:9090 list
```

## Device probing

### gRPC devices

gRPC devices are probed using the **standard gRPC health protocol**. Devices
that expose diagnostics advertise the `device.Diagnostics` service via the
health service; the monitor then calls `Collect` to retrieve hardware/software/
firmware versions, status, and checksum.

### REST devices

REST devices are probed by calling:

- `GET /health` — liveness + capability discovery (flexible JSON; bare `ok`/`up`
  bodies are also accepted)
- `GET /diagnostics` — full diagnostics payload where supported
- `GET /diagnostics/{cap}` — per-capability fallback for devices without a
  combined endpoint

## Logging

Logs use `log/slog` (structured, key-value) written to stdout. Include:

- REST access logs (method, path, status, latency)
- gRPC unary request logs (method, latency, error)
- Device add/remove and status transitions
- Probe cycle start/end, per-device results, and diagnostics collection status

## Development

```sh
make build    # build the monitor binary
make run      # build and run with the sample config
make vet      # run go vet
make tidy     # tidy go modules
make proto    # regenerate protobuf/gRPC code
make clean    # remove build artifacts
```

## License

This project is licensed under the [MIT License](LICENSE).
