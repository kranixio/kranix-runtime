# kranix-runtime

> Docker / Kubernetes runtime abstraction — the infrastructure driver layer.

`kranix-runtime` contains the actual drivers that communicate with container runtimes and cluster APIs. It abstracts over Docker, Kubernetes, Podman, and remote nodes so that `kranix-core` can orchestrate workloads without knowing which backend it is running on. The abstraction boundary is clean: core expresses *what* should happen, runtime decides *how* to make it happen on the target backend.

---

## What it does

- Implements the `RuntimeDriver` interface for each supported backend
- Manages local Docker containers, Compose stacks, and image lifecycle
- Talks directly to the Kubernetes API server for cluster workloads
- Supports remote node connections (SSH-based or agent-based)
- Handles ephemeral dev environments and local cloud simulation
- Reports observed state back to `kranix-core` for reconciliation

---

## Architecture position

```
kranix-core  ──►  kranix-runtime  ──►  Docker API
                                ──►  Kubernetes API
                                ──►  Remote node agents
```

`kranix-runtime` is driven exclusively by `kranix-core`. It has no HTTP API of its own and is never called directly by `kranix-api` or `kranix-cli`.

---

## Supported backends

| Backend | Status | Notes |
|---|---|---|
| Docker (local) | Stable | Via Docker Engine API |
| Kubernetes | Stable | Via `client-go` |
| Podman | Beta | Via Podman socket API |
| Docker Compose | Beta | Compose v2 spec |
| Remote node (SSH) | Alpha | SSH + Docker on remote host |

---

## The `RuntimeDriver` interface

All backends implement this interface, defined in `kranix-packages`:

```go
type RuntimeDriver interface {
    // Workload operations
    Deploy(ctx context.Context, spec *types.WorkloadSpec) (*types.WorkloadStatus, error)
    Destroy(ctx context.Context, workloadID string) error
    Restart(ctx context.Context, workloadID string) error

    // Observation
    GetStatus(ctx context.Context, workloadID string) (*types.WorkloadStatus, error)
    ListWorkloads(ctx context.Context, namespace string) ([]*types.WorkloadStatus, error)
    StreamLogs(ctx context.Context, podID string, opts *types.LogOptions) (<-chan string, error)

    // Lifecycle
    Ping(ctx context.Context) error
    Backend() string
}
```

`kranix-core` selects the appropriate driver at runtime based on the workload's target backend field.

---

## Project structure

```
kranix-runtime/
├── cmd/                         # Optional standalone runner
├── internal/
│   ├── docker/                  # Docker Engine API driver
│   │   ├── driver.go
│   │   ├── deploy.go
│   │   ├── logs.go
│   │   └── image.go
│   ├── kubernetes/              # Kubernetes driver (client-go)
│   │   ├── driver.go
│   │   ├── deploy.go
│   │   ├── pods.go
│   │   └── watch.go
│   ├── podman/                  # Podman driver
│   ├── compose/                 # Docker Compose driver
│   ├── remote/                  # Remote node driver (SSH)
│   └── registry/                # Driver registry — maps backend name to driver
├── pkg/
│   └── imageutil/               # Image pull, tag, push helpers
├── config/
└── tests/
    ├── unit/
    ├── integration/             # Requires Docker daemon or kind cluster
    └── fixtures/
```

---

## Getting started

### Prerequisites

- Go 1.22+
- Docker daemon (for Docker/Compose driver tests)
- `kind` or `minikube` (for Kubernetes driver tests)

### Build

```bash
git clone https://github.com/kranix-io/kranix-runtime
cd kranix-runtime
go mod download
go build ./...
```

### Run tests

```bash
# Unit tests only (no daemon required)
go test ./internal/... -short

# Integration: Docker driver
KRANE_RUNTIME_BACKEND=docker go test ./tests/integration/... -tags integration

# Integration: Kubernetes driver (requires kind cluster)
kind create cluster --name kranix-test
KRANE_RUNTIME_BACKEND=kubernetes \
KUBECONFIG=$(kind get kubeconfig-path --name kranix-test) \
go test ./tests/integration/... -tags integration
```

---

## Configuration

```yaml
runtime:
  default_backend: kubernetes    # docker | kubernetes | podman | compose

docker:
  host: "unix:///var/run/docker.sock"
  api_version: "1.45"

kubernetes:
  kubeconfig: ""                  # empty = in-cluster config
  context: ""                     # empty = current context
  default_namespace: "default"

podman:
  socket: "unix:///run/user/1000/podman/podman.sock"

remote:
  ssh_key_path: "~/.ssh/id_rsa"
  known_hosts_path: "~/.ssh/known_hosts"
```

---

## Adding a new backend

1. Create a new package under `internal/<backend>/`
2. Implement the `RuntimeDriver` interface
3. Register it in `internal/registry/registry.go`:

```go
func init() {
    registry.Register("mybackend", func(cfg *config.Config) (types.RuntimeDriver, error) {
        return mybackend.New(cfg)
    })
}
```

4. Add integration tests under `tests/integration/<backend>/`
5. Document it in this README under the supported backends table

---

## Connectivity

| Repo | Relationship |
|---|---|
| `kranix-core` | Core drives runtime via the `RuntimeDriver` interface |
| `kranix-packages` | Imports the `RuntimeDriver` interface and shared types |
| Docker API | Direct socket/HTTP connection |
| Kubernetes API | Via `client-go` using kubeconfig or in-cluster config |

---

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md). New drivers must pass all interface compliance tests in `tests/compliance/`. Integration tests are mandatory — unit tests with mocks are not sufficient for driver correctness.

## License

Apache 2.0 — see [LICENSE](./LICENSE).
