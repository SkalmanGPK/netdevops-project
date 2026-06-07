# NetDevOps Mesh-Pinger

A production-inspired DevOps project demonstrating end-to-end infrastructure automation, container orchestration, and real-time network telemetry — built to reflect the intersection of network engineering and modern DevOps practices.

---

## What This Project Does

Mesh-Pinger deploys a **sidecar architecture** on a local Kubernetes cluster where every pod runs two containers side by side:

- **main-app** — a lightweight HTTP application (the monitored target)
- **pinger** — a Go-based network telemetry agent that continuously discovers all other pods in the mesh via the Kubernetes API and measures round-trip latency to each one

Every 5 seconds, each pinger sidecar queries the Kubernetes control plane for all pods matching the `app=mesh-pinger` label, filters out its own IP, and fires HTTP health probes to every peer — logging structured JSON telemetry including destination node, destination IP, RTT latency, and HTTP status code.

The result is a self-forming mesh network where every node has real-time visibility into pod-to-pod communication latency across worker nodes.

---

## Architecture

```
┌─────────────────────────────────┐     ┌─────────────────────────────────┐
│         Pod A (worker-1)        │     │         Pod B (worker-2)        │
│                                 │     │                                 │
│  ┌──────────┐  ┌─────────────┐  │     │  ┌──────────┐  ┌─────────────┐  │
│  │ main-app │  │   pinger    │  │────▶│  │ main-app │  │   pinger    │  │
│  │  :8081   │  │    :8090    │  │◀────│  │  :8081   │  │    :8090    │  │
│  └──────────┘  └──────┬──────┘  │     │  └──────────┘  └──────┬──────┘  │
│                       │         │     │                       │         │
└───────────────────────┼─────────┘     └───────────────────────┼─────────┘
                        │                                       │
                        ▼                                       ▼
                ┌───────────────┐                     Kubernetes API
                │  K8s API      │                     Pod Discovery
                │  (RBAC: list  │                     via label selector
                │   pods only)  │
                └───────────────┘
```

**Network flow per telemetry cycle:**

1. Pinger authenticates to the Kubernetes API using a mounted service account token
2. Lists all pods matching `app=mesh-pinger` across all namespaces
3. Filters out its own pod IP (avoids self-probing)
4. Fires `GET /health` to every peer on port 8090
5. Records RTT and logs structured JSON telemetry to stdout

---

## Stack

| Layer | Technology | Purpose |
|---|---|---|
| Application | Go 1.24 | Network telemetry agent (sidecar) |
| Containerization | Docker (multi-stage build) | Minimal, hardened runtime image |
| Infrastructure | Terraform + kind provider | Declarative local Kubernetes cluster |
| Orchestration | Kubernetes | Deployment, scheduling, pod lifecycle |
| Security | Kubernetes RBAC | Least-privilege service account |
| CI/CD | Jenkins (containerized) | Automated build, load, and deploy pipeline |

---

## Project Structure

```
.
├── network-pinger/
│   ├── main.go          # Telemetry agent — service discovery, probing, graceful shutdown
│   ├── env.go           # Local .env loader for development
│   ├── Dockerfile       # Multi-stage build: golang:1.24-alpine → alpine:3.19
│   ├── go.mod
│   └── go.sum
├── terraform/
│   └── main.tf          # kind cluster + RBAC + Deployment (single apply)
├── k8s-manifests/
│   └── test-pod.yaml    # Standalone pod for isolated debugging
├── jenkins/
│   └── Dockerfile       # Custom Jenkins image with Docker CLI, kubectl, kind
├── Jenkinsfile          # CI/CD pipeline: Build → Load → Deploy
└── README.md
```

---

## Security Hardening

The deployment applies defense-in-depth at every layer:

**Pod level**
- `runAsNonRoot: true` — no container runs as root
- `runAsUser: 10001` — explicit non-privileged UID

**Container level**
- `allowPrivilegeEscalation: false`
- `readOnlyRootFilesystem: true`
- `capabilities.drop: ["ALL"]` — all Linux capabilities dropped

**RBAC (least privilege)**
- Dedicated `ServiceAccount` bound to a `ClusterRole` with a single permission: `list pods`
- No read, write, create, or delete access to any other resource

---

## Go Application Highlights

**Exponential backoff with jitter** — all network and API operations retry with exponential backoff, capped at 30 seconds, with ±10% random jitter to prevent thundering herd on simultaneous pod restarts:

```go
jitter := time.Duration((rand.Float64() - 0.5) * 0.1 * float64(nextWait))
totalWait := nextWait + jitter
```

**Graceful shutdown** — SIGTERM handling via `signal.NotifyContext` drains the HTTP server cleanly within a 5-second timeout before the process exits, ensuring Kubernetes rolling updates complete without dropped requests.

**Structured logging** — all output is JSON via `log/slog`, making logs directly ingestible by Loki, Elasticsearch, or any structured log pipeline.

---

## Infrastructure (Terraform)

A single `terraform apply` provisions the complete environment in dependency order:

1. `kind_cluster` — 1 control-plane + 2 worker nodes
2. `kubernetes_service_account` — mesh-pinger-sa
3. `kubernetes_cluster_role` — list pods only
4. `kubernetes_cluster_role_binding` — binds SA to role
5. `kubernetes_deployment` — 3 replicas, both containers, all probes

The Kubernetes provider is dynamically configured using credentials output from the kind cluster resource, so no manual kubeconfig steps are needed.

---

## CI/CD Pipeline (Jenkins)

The `Jenkinsfile` defines a three-stage pipeline:

```
Build → Load into kind → Deploy
```

| Stage | What happens |
|---|---|
| **Build** | `docker build` compiles the Go binary and produces a minimal Alpine image |
| **Load into kind** | `kind load docker-image` pushes the image into all cluster nodes (bypasses registry) |
| **Deploy** | `kubectl rollout restart` triggers a rolling update; pipeline waits for completion |

Jenkins runs as a Docker container with Docker CLI, `kubectl`, and `kind` installed, connected to the same Docker network as the kind cluster.

---

## Getting Started

**Prerequisites:** Docker, Terraform, kind, kubectl, Go 1.24+

```bash
# 1. Clone the repository
git clone https://github.com/SkalmanGPK/netdevops-project.git
cd netdevops-project

# 2. Build the image
cd network-pinger
docker build -t mesh-pinger:v3 .
cd ..

# 3. Provision the cluster and deploy all resources
cd terraform
terraform init
terraform apply

# 4. Load the image into the kind cluster nodes
kind load docker-image mesh-pinger:v3 --name devops-cluster

# 5. Verify
kubectl get pods
kubectl logs -l app=mesh-pinger -c pinger --follow
```

You should see structured JSON telemetry with cross-node RTT measurements within a few seconds.

---

## Example Telemetry Output

```json
{"time":"2026-06-05T12:08:10Z","level":"INFO","msg":"Mesh Network Telemetry Sample",
  "destination_node":"devops-cluster-worker2",
  "destination_ip":"10.244.2.7",
  "rtt_latency":"424.761µs",
  "http_status_code":200}
```

---

## What This Project Demonstrates

- **Infrastructure as Code** — entire environment defined declaratively in Terraform, reproducible from scratch with a single command
- **Container orchestration** — multi-container pod design, rolling deployments, health probes (startup, readiness, liveness)
- **Network engineering** — pod-to-pod communication across nodes, IP routing within a CNI-managed overlay network, latency measurement at the application layer
- **Security** — RBAC least-privilege, non-root containers, dropped capabilities, read-only filesystems
- **Resilience patterns** — exponential backoff, jitter, graceful shutdown, context-aware cancellation
- **CI/CD automation** — end-to-end pipeline from code commit to running deployment
- **Observability** — structured JSON logging designed for log aggregation pipelines
