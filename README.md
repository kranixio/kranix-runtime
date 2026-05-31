# kranix-runtime

> Docker / Kubernetes runtime abstraction — the infrastructure driver layer.

`kranix-runtime` contains the actual drivers that communicate with container runtimes and cluster APIs. It abstracts over Docker, Kubernetes, Podman, and remote nodes so that `kranix-core` can orchestrate workloads without knowing which backend it is running on. The abstraction boundary is clean: core expresses *what* should happen, runtime decides *how* to make it happen on the target backend.

---

## What it does

- Implements the `RuntimeDriver` interface for each supported backend
- Manages local Docker containers, Compose stacks, and image lifecycle
- Talks directly to the Kubernetes API server for cluster workloads (**priority classes**, **spot/preemptible** hints, **cross-namespace NetworkPolicy**, **CronJob** vs **Deployment**)
- Supports remote node connections (SSH-based or agent-based)
- Handles ephemeral dev environments and local cloud simulation
- Reports observed state back to `kranix-core` for reconciliation
- **Runtime health scoring** — scores each backend and node 0–100 from latency and error rate
- **Automatic node draining** — cordons nodes and evicts workloads before maintenance
- **Multi-arch image support** — routes ARM vs x86 workloads to correct nodes via `kubernetes.io/arch`

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
| Kubernetes | Stable | Via `client-go` — Deployments (**or CronJobs** when `cronSchedule` is active), PriorityClass mapping, spot tolerations, optional cross-namespace NetworkPolicy |
| Podman | Stable | Rootless, daemonless runtime fully supported |
| Docker Compose | Stable | Compose v2 and v1 support |
| Remote node (SSH) | Beta | Agentless SSH connections to bare metal servers |
| Edge node agent | Alpha | Lightweight agent for remote nodes |

### Kubernetes placement & networking details

See **`internal/kubernetes/scheduling.go`**, **`networkpolicy.go`**, **`cronjob.go`**, and **`workload_pod.go`**:

| Concern | Behavior |
|---------|----------|
| **Cron** | Active **`spec.cronSchedule`** → **`batch/v1` CronJob** (`timeZone`, parallelism from **`replicas`**, **`concurrencyPolicy`**); otherwise **`Deployment`**. Lifecycle APIs resolve CronJob vs Deployment by name. |
| **Priority / preemption** | **`scheduling.workloadPriority`** maps to **`priorityClassName`** **`kranix-{critical\|high\|normal\|low}`**, with **`-np`** suffix when **`preemptionEnabled`** is **false**. **`priorityClassName`** overrides. Cluster admins must define matching **`PriorityClass`** objects for real preemption semantics. |
| **Spot / node loss** | **`spot.enabled`** adds tolerations; **`rescheduleOnNodeTermination`** adds **`NoExecute`** tolerations on **`node.kubernetes.io/not-ready`** / **`unreachable`** (bounded eviction wait) plus shorter **`terminationGracePeriodSeconds`** on the pod spec. |
| **Cross-namespace traffic** | When **`crossNamespaceTraffic.enabled`**, applies a **`NetworkPolicy`** restricting ingress/egress to same-namespace and explicitly allowed namespaces (labels **`kubernetes.io/metadata.name`**), with optional kube-dns and internet egress flags. |
| **Health scoring** | Backends tracked via **`internal/health/`** — latency + error rate → score 0–100. Kubernetes nodes scored from Ready/MemoryPressure/DiskPressure conditions. |
| **Node draining** | **`NodeOperations.DrainNode`** cordons the node, applies **`kranix.io/drain`** taint, and evicts non-DaemonSet pods with configurable grace period. |
| **Multi-arch routing** | **`scheduling.architecture`** or image tag hints (`arm64`, `amd64`) inject **`kubernetes.io/arch`** nodeSelector + required node affinity. Docker pulls use platform-specific image pulls. |

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

Kubernetes drivers also implement **`types.NodeOperations`** for node health and drain:

```go
type NodeOperations interface {
    ListBackendHealth(ctx context.Context) ([]BackendHealthReport, error)
    ListNodeHealth(ctx context.Context) ([]NodeHealthReport, error)
    DrainNode(ctx context.Context, req NodeDrainRequest) (*NodeDrainResult, error)
}
```

Retrieve via `registry.GetNodeOperations("kubernetes", cfg)`.

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
│   ├── kubernetes/              # Kubernetes driver (Deployment or CronJob)
│   │   ├── driver.go
│   │   ├── deploy.go
│   │   ├── workload_pod.go     # Shared pod spec for Deployments and CronJobs
│   │   ├── cronjob.go          # CronJob path when cron schedule is enabled
│   │   ├── networkpolicy.go   # Cross-namespace NetworkPolicy helpers
│   │   ├── scheduling.go      # PriorityClass + spot toleration merge
│   │   ├── pods.go
│   │   └── watch.go
│   ├── podman/                  # Podman driver
│   ├── compose/                 # Docker Compose driver
│   ├── remote/                  # Remote node driver (SSH)
│   ├── gpu/                     # GPU scheduling utilities
│   │   └── gpu.go
│   ├── ephemeral/               # Ephemeral environment lifecycle
│   │   └── lifecycle.go
│   ├── edge/                    # Edge node agent
│   │   └── agent.go
│   ├── cache/                   # Image caching layer
│   │   └── image.go
│   ├── metrics/                 # Resource usage metrics collector
│   │   └── collector.go
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

gpu:
  enabled: false                  # Enable GPU support
  default_vendor: "nvidia"        # nvidia | amd
  nvidia_device_path: "/dev/nvidia0"
  amd_device_path: "/dev/kfd"

ephemeral:
  enabled: false                  # Enable ephemeral environment lifecycle
  default_ttl: "2h"               # Default time-to-live for environments
  max_environments: 10            # Maximum concurrent ephemeral environments
  namespace_prefix: "ephem-"      # Prefix for ephemeral namespaces
  auto_teardown: true             # Automatically teardown expired environments
  teardown_on_merge: true         # Teardown when PR is merged
  teardown_on_close: true         # Teardown when PR is closed
  cleanup_interval: "5m"          # Interval for cleanup checks

edge_agent:
  enabled: false                  # Enable edge node agent
  node_id: ""                     # Auto-generated if empty
  node_name: ""                   # Auto-generated if empty
  ip_address: ""                  # Auto-detected if empty
  port: 50052                     # gRPC port for edge agent
  heartbeat_interval: "30s"       # Heartbeat interval to control plane
  auth_token: ""                  # Authentication token for control plane
```

---

## New Features

### GPU Workload Scheduling

kranix-runtime now supports GPU workload scheduling for both NVIDIA and AMD devices. The GPU support is integrated into both Docker and Kubernetes drivers:

**GPU Configuration:**
```yaml
gpu:
  enabled: true
  default_vendor: "nvidia"  # or "amd"
```

**Workload Spec with GPU:**
```yaml
resources:
  gpu:
    vendor: "nvidia"
    count: 2
    type: "A100"
    memory: "40Gi"
```

**Supported GPU Vendors:**
- NVIDIA: Uses `nvidia.com/gpu` resource type in Kubernetes and Docker device requests
- AMD: Uses `amd.com/gpu` resource type in Kubernetes and AMDGPU device requests

### Ephemeral Environment Lifecycle

Automatically create and teardown ephemeral environments per PR or branch:

**Ephemeral Configuration:**
```yaml
ephemeral:
  enabled: true
  default_ttl: "2h"
  max_environments: 10
  namespace_prefix: "ephem-"
  auto_teardown: true
  teardown_on_merge: true
  teardown_on_close: true
  cleanup_interval: "5m"
```

**Features:**
- Automatic environment creation on PR/branch triggers
- TTL-based expiration with configurable cleanup intervals
- Auto-teardown on PR merge or close events
- Max concurrent environment limits
- Namespace isolation with configurable prefixes

### Edge Node Agent

Lightweight binary that connects remote nodes to the control plane:

**Edge Agent Configuration:**
```yaml
edge_agent:
  enabled: true
  node_id: "edge-node-001"
  node_name: "production-edge"
  ip_address: "192.168.1.100"
  port: 50052
  heartbeat_interval: "30s"
  auth_token: "secure-token"
```

**Features:**
- gRPC-based communication with control plane
- Automatic node registration and heartbeat
- Workload deployment and management on edge nodes
- Resource discovery and reporting
- Support for GPU-equipped edge nodes

### Image Caching Layer

Accelerate image pulls by caching images across nodes:

**Image Cache Configuration:**
```yaml
image_cache:
  enabled: true
  cache_size_gb: 100
  max_cached_images: 50
  ttl: "168h"                     # 7 days
  prepull_images:
    - nginx:latest
    - postgres:14
  registry_mirrors:
    - https://mirror.gcr.io
```

**Features:**
- Local image caching to reduce registry pull times
- Configurable cache size and image count limits
- TTL-based expiration with automatic cleanup
- Prepull frequently used images on node startup
- Registry mirror support for faster pulls
- Cache hit rate tracking

### Resource Usage Metrics

Expose CPU, memory, GPU, network, and storage metrics per workload to kranix-core:

**Metrics Configuration:**
```yaml
metrics:
  enabled: true
  collection_interval: "30s"
  retention_period: "24h"
  expose_endpoint: true
  metrics_port: 9090
```

**Features:**
- CPU usage (cores and percentage)
- Memory usage (bytes and percentage)
- GPU metrics (utilization, memory, temperature, power)
- Network metrics (throughput, packets, errors)
- Storage metrics (I/O, disk usage)
- Configurable collection intervals
- Metrics endpoint for scraping

### Stabilized Drivers

**Podman Driver (Stable)**
- Full rootless mode support with automatic detection
- Daemonless architecture by design
- Automatic socket path resolution for rootless and system modes
- GPU resource support for NVIDIA GPUs
- Resource limits (CPU, memory)
- Port mapping

**Docker Compose Driver (Stable)**
- Automatic detection of Docker Compose v2 (`docker compose`) and v1 (`docker-compose`)
- Full stack management (up, down, restart, ps)
- Project-based isolation
- Volume and orphan cleanup on destroy
- Logs streaming with tail support
- Service status tracking

**Remote SSH Backend (Beta)**
- Agentless SSH connections to bare metal servers
- Automatic runtime detection (Docker or Podman) on remote hosts
- Secure SSH with known_hosts verification
- GPU and resource support on remote hosts
- Auto-connect on deploy
- Runtime-agnostic command execution

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
