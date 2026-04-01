# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
make build                          # Compile manager binary to bin/manager

# Test
make test                           # Run all unit tests with coverage
go test ./internal/controller/...   # Run tests in a specific package
go test -run TestFoo ./...          # Run a single test by name

# Lint & Format
make lint                           # Run golangci-lint
make lint-fix                       # Run golangci-lint with auto-fixes
make fmt                            # Format code with goimports
make vet                            # Run go vet

# Code Generation (run after changing CRD types or kubebuilder markers)
make manifests                      # Regenerate CRDs and RBAC manifests
make generate                       # Regenerate DeepCopy methods

# License
make add-license                    # Add SPDX headers to all Go files
make check-license                  # Verify SPDX headers are present

# E2E
make test-e2e                       # Run E2E tests against a Kind cluster
```

After modifying types in `api/v1alpha1/`, always run `make manifests generate`.

## Architecture

Boot-operator is a Kubernetes controller that automates network boot (HTTPBoot/PXE) for bare-metal servers. It integrates with [metal-operator](https://github.com/ironcore-dev/metal-operator) which manages physical server lifecycle.

### Data Flow

1. **metal-operator** creates a `ServerBootConfiguration` CR referencing a `Server` CR and an OCI boot image.
2. Boot-operator's `ServerBootConfigurationHTTPReconciler` or `ServerBootConfigurationPXEReconciler` detects this and:
   - Extracts the system UUID and network identifiers (IPs/MACs) from the `Server` CR.
   - Queries the OCI registry to resolve the image manifest for the target architecture.
   - Extracts layer digests (UKI for HTTPBoot; kernel/initrd/squashfs for iPXE).
   - Creates/updates an `HTTPBootConfig` or `IPXEBootConfig` CR with constructed download URLs.
3. Booting systems call the HTTP boot server endpoints:
   - `/httpboot` — returns the UKI URL, matched by requesting system IP against `HTTPBootConfig.NetworkIdentifiers`
   - `/ipxe/<uuid>` — returns an iPXE script with kernel/initrd/squashfs URLs
   - `/ignition/<uuid>` — returns the Ignition config for the system UUID

### Components

| Path | Purpose |
|------|---------|
| `api/v1alpha1/` | CRD type definitions: `IPXEBootConfig`, `HTTPBootConfig` |
| `internal/controller/` | 4 reconcilers: `IPXEBootConfigReconciler`, `HTTPBootConfigReconciler`, `ServerBootConfigurationPXEReconciler`, `ServerBootConfigurationHTTPReconciler` |
| `internal/oci/` | OCI manifest resolution (multi-arch image index support, arch selection by platform or legacy CNAME prefix) |
| `internal/registry/` | Allowlist-based registry validation; defaults to `ghcr.io` |
| `internal/uki/` | UKI-specific utilities |
| `server/` | `bootserver.go` (HTTP endpoints for boot), `imageproxyserver.go` (OCI layer proxy) |
| `cmd/main.go` | Operator entrypoint; flags wire together servers and controllers |
| `cmd/bootctl/` | CLI utility for boot operations |

### CRDs

**HTTPBootConfig**: Holds `SystemUUID`, `NetworkIdentifiers` (IPs + MACs), `UKIURL`, and optional `IgnitionSecretRef`. Status: `Ready | Pending | Error`.

**IPXEBootConfig**: Holds `SystemUUID`, `SystemIPs`, `KernelURL`, `InitrdURL`, `SquashfsURL`, and optional `IgnitionSecretRef`. Status: `Ready | Pending | Error`.

Both CRDs use field indexes (by SystemUUID, IPs, MACs) so the boot server can look up configs without listing all objects.

### Key Design Details

- **Owner references**: `ServerBootConfiguration` owns the `HTTPBootConfig`/`IPXEBootConfig` it creates; cascading delete is automatic.
- **Secret watchers**: Controllers watch `Secret` objects and requeue the owning boot config when the Ignition secret changes.
- **OCI arch resolution**: `internal/oci/manifest.go` supports two strategies — modern platform-based selection and legacy CNAME-prefix fallback (for garden-linux compatibility).
- **Registry allowlist**: The `--allowed-registries` flag (CSV) gates all OCI image pulls; defaults to `ghcr.io` if unset.
- **HTTP/2 disabled**: Explicitly disabled to mitigate CVE-2023-44487.
- **Leader election**: Enabled via `--leader-elect`; ID is `e9f0940b.ironcore.dev`.

### Key CLI Flags (`cmd/main.go`)

| Flag | Purpose |
|------|---------|
| `--architecture` | Target system arch for OCI manifest resolution (e.g. `amd64`) |
| `--controllers` | Enable/disable specific controllers |
| `--allowed-registries` | CSV of allowed OCI registries |
| `--default-httpboot-oci-image` | Default OCI image used when HTTPBoot config has none |
| `--ipxe-service-url` | Base URL of the iPXE image server |
| `--image-server-url` | Base URL of the image proxy server |
| `--boot-server-address` | Bind address for the HTTP boot server |

## Conventions

- All Go files require an SPDX header: `// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors` / `// SPDX-License-Identifier: Apache-2.0`
- Tests use Ginkgo/Gomega BDD style; suite setup is in `suite_test.go` next to the tests.
- Kubebuilder markers (`+kubebuilder:rbac:`, `+kubebuilder:object:root=true`, etc.) drive code and manifest generation — keep them accurate.
- `--default-httpboot-uki-url` is deprecated; use `--default-httpboot-oci-image` instead.
