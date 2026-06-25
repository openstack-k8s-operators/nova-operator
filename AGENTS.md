# AGENTS.md - nova-operator

## Project overview

nova-operator is a Kubernetes operator that manages
[OpenStack Nova](https://docs.openstack.org/nova/latest/) and
[OpenStack Placement](https://docs.openstack.org/placement/latest/)
on OpenShift/Kubernetes. It is part of the
[openstack-k8s-operators](https://github.com/openstack-k8s-operators) project.

## Tech stack

| Layer | Technology |
|-------|------------|
| Language | Go (modules, multi-module workspace via `go.work`) |
| Scaffolding | [Kubebuilder v4](https://book.kubebuilder.io/) + [Operator SDK](https://sdk.operatorframework.io/) |
| CRD generation | controller-gen (DeepCopy, CRDs, RBAC, webhooks) |
| Config management | Kustomize |
| Packaging | OLM bundle |
| Testing | Ginkgo/Gomega + envtest (functional), KUTTL (integration) |
| Linting | golangci-lint (`.golangci.yaml`) |
| CI | Zuul (`zuul.d/`), Prow (`.ci-operator.yaml`), GitHub Actions |

## Directory structure and Custom Resources

The project directory structure and design choices are documented in
[doc/design.md](doc/design.md).

## Build commands

After modifying Go code, always run: `make generate manifests fmt vet`.

## Code style guidelines

- Follow standard openstack-k8s-operators conventions and lib-common patterns.
- Use `lib-common` modules for conditions, endpoints, TLS, storage, and other
  cross-cutting concerns rather than re-implementing them.
- CRD types are organized per service under `api/<service>/v1beta1/`
  (e.g. `api/nova/v1beta1/`, `api/placement/v1beta1/`).
- Controller logic goes in `internal/controller/<service>/`.
  Resource-building helpers go in `internal/<service>/` packages.
- Config templates are split per service under `templates/` (e.g.
  `templates/nova/`, `templates/placement/`) and mounted at runtime via
  the `OPERATOR_TEMPLATES` environment variable.
- Webhook logic is split between the kubebuilder markers in `api/` and
  the implementation in `internal/webhook/`.

## Testing

- Functional tests use the envtest framework with Ginkgo/Gomega and live in
  `test/functional/`.
- KUTTL integration tests live in `test/kuttl/`.
- Run all functional tests: `make test`.
- When adding a new field or feature, add corresponding test cases in
  `test/functional/` and update fixture data accordingly.

## Key dependencies

- [lib-common](https://github.com/openstack-k8s-operators/lib-common): shared modules for conditions, endpoints, database, TLS, secrets, etc.
- [infra-operator](https://github.com/openstack-k8s-operators/infra-operator): RabbitMQ and topology APIs.
- [mariadb-operator](https://github.com/openstack-k8s-operators/mariadb-operator): database provisioning.
- [keystone-operator](https://github.com/openstack-k8s-operators/keystone-operator): identity service registration.
- [gophercloud](https://github.com/gophercloud/gophercloud): Go OpenStack SDK.
- [Developer guide](https://github.com/openstack-k8s-operators/nova-operator/blob/main/doc/developer.md): local development setup, building, running, and debugging the operator.

## AI-assisted development

When an AI agent creates a commit on behalf of a developer, add a
`Co-authored-by` trailer to the commit message body for the AI tool that
authored the changes (e.g. `Co-authored-by: Cursor <cursoragent@cursor.com>`).
Use `Assisted-by` only when the AI provided review or guidance but did not write
the patch. Read this file before committing; do not wait for the user to request
attribution each time.

For interactive onboarding of this repository, see [doc/onboarding.md](doc/onboarding.md)
and the devskills `/onboarding-buddy` skill.
