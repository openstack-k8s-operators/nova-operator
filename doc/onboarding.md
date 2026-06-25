# nova-operator onboarding

Quick reference for developers new to this repository. Use with the devskills
**`/onboarding-buddy`** skill for interactive walkthroughs.

For design decisions (CR strategy, Secret naming, cells, external dataplane), see
[design.md](design.md). For dev setup and guidelines, see
[developer.md](developer.md) and [AGENTS.md](../AGENTS.md).

## What this operator manages

nova-operator deploys **OpenStack Nova** and **OpenStack Placement** control-plane
services on Kubernetes/OpenShift. It is part of
[openstack-k8s-operators](https://github.com/openstack-k8s-operators).

**Today (main branch):**

- **Nova** — compute control plane (API, scheduler, conductor, metadata, noVNC proxy, cells, …)
- **Placement** — resource inventory API (`PlacementAPI` CR); formerly a separate
  placement-operator, now unified in this binary and OLM bundle

**In progress:** **Cyborg** (accelerator service) on the `add-cyborg` branch — same
per-service layout as Placement (`api/cyborg/`, `internal/cyborg/`, …).

One **manager process** (`cmd/main.go`) registers reconcilers for all services.

## Custom Resources

### Nova family

| Kind | Role |
|------|------|
| `Nova` | Top-level CR — orchestrates the full Nova control plane for openstack-operator |
| `NovaAPI` | nova-api service |
| `NovaScheduler` | nova-scheduler |
| `NovaConductor` | nova-conductor (per cell) |
| `NovaMetadata` | nova-metadata |
| `NovaNoVNCProxy` | noVNC proxy |
| `NovaCell` | Cell mapping and cell-level resources |
| `NovaCompute` | Config for compute nodes (not deployed by this operator — see [design.md](design.md)) |

### Placement

| Kind | Role |
|------|------|
| `PlacementAPI` | placement-api service — no top-level "Placement" CR like `Nova` |

**High-level design** (details in [design.md](design.md)):

- `Nova` hides deployment complexity from openstack-operator.
- Service-level CRs can be deployed **in isolation** for testing; dependencies arrive via Secrets.
- Nova uses **Cells v2** by default (superconductors, per-cell or global metadata).
- Placement is consumed by nova-scheduler via a **Keystone endpoint**; it is not reconciled
  as part of the `Nova` CR.

## Directory map

```
nova-operator/
├── api/
│   ├── nova/v1beta1/          # Nova CRD Go types + kubebuilder markers
│   └── placement/v1beta1/     # PlacementAPI types
├── cmd/main.go                # Manager setup, scheme registration, controller wiring
├── internal/
│   ├── controller/nova/       # Nova reconcilers (one file per CR kind)
│   ├── controller/placement/  # PlacementAPI reconciler
│   ├── nova/                  # Deployment/Job/Secret builders for Nova services
│   ├── placement/             # Builders for Placement
│   └── webhook/               # Defaulting and validation webhooks
├── templates/
│   ├── nova/                  # Config templates → mounted via OPERATOR_TEMPLATES
│   └── placement/
├── config/                    # Kustomize: CRDs, RBAC, manager, webhooks, samples
├── test/
│   ├── functional/nova/       # Ginkgo + envtest
│   ├── functional/placement/
│   └── kuttl/                 # Integration tests
├── bundle/                    # OLM bundle (generated)
└── doc/                       # design.md, developer.md, onboarding.md (this file)
```

**Pattern for each service:** `api/<svc>/` → `internal/controller/<svc>/` →
`internal/<svc>/` (resource builders) → `templates/<svc>/`.

## Request path — when a CR changes

1. User or openstack-operator applies a CR (e.g. `Nova`).
2. API server stores it; controller-runtime **watch** enqueues the object.
3. The matching **reconciler** in `internal/controller/nova/` runs `Reconcile()`.
4. Reconciler reads the CR spec and builds Deployments, Services, Secrets, Jobs
   using helpers in `internal/nova/`.
5. Config **Secrets** are populated from templates in `templates/nova/` (runtime path
   from `OPERATOR_TEMPLATES` on the operator pod).
6. Reconciler creates/updates children via the controller-runtime **client**.
7. **Status** and **conditions** (lib-common) reflect progress on the CR.
8. **Webhooks** in `internal/webhook/` validate/default before persist (when enabled).

For a code-level trace of a specific controller, use **`/explain-flow`** on the
reconciler file or `internal/controller/<service>/`.

## Dependencies on other operators

| Dependency | Used for |
|------------|----------|
| [lib-common](https://github.com/openstack-k8s-operators/lib-common) | Conditions, endpoints, TLS, secrets, common helpers |
| [mariadb-operator](https://github.com/openstack-k8s-operators/mariadb-operator) | Database instances |
| [keystone-operator](https://github.com/openstack-k8s-operators/keystone-operator) | Service registration / endpoints |
| [infra-operator](https://github.com/openstack-k8s-operators/infra-operator) | RabbitMQ, memcached, topology |

## Testing

| Layer | Location | Command |
|-------|----------|---------|
| Functional (envtest) | `test/functional/nova/`, `test/functional/placement/` | `make test` |
| Focused tests | same | `make test GINKGO_ARGS="--focus 'pattern'"` |
| Integration (KUTTL) | `test/kuttl/` | KUTTL CLI against a real cluster |

When adding a field or feature: update types, run `make generate manifests fmt vet`, add
functional test coverage, update fixtures.

## Common first tasks

1. Read [design.md](design.md) (Nova + Placement decisions).
2. Pick one service CR (e.g. `NovaScheduler`) — open its type in `api/`, controller in
   `internal/controller/nova/`, and builder under `internal/nova/`.
3. Run `make test GINKGO_ARGS="--focus 'NovaScheduler'"` (or similar).
4. Trace `cmd/main.go` to see how controllers register.
5. Use `/explain-flow` on the reconciler for a detailed code walkthrough.
