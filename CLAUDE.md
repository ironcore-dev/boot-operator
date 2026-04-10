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

## Git and GitHub Rules

- **Never run `git push` under any circumstances.** The user pushes to GitHub. Do not push branches, force-push, or open/modify PRs without an explicit written instruction that contains the word **"push"** or **"create PR"**. Accepting edits does not constitute approval to push.
- **Never modify anything on `main` directly.** All work happens on a feature branch.
- **Do not create, close, or comment on GitHub issues or PRs** unless explicitly asked.

## Change Discipline

Boot-operator has a clear separation of concerns across its controllers. Different controllers are owned and reviewed by different people (HTTPBoot, iPXE, readiness, boot server, etc.). Keep changes scoped and minimal:

- **Touch only what the feature requires.** If a change is in the HTTPBoot controller, do not modify iPXE controller files unless they are directly affected. Unnecessary cross-controller edits increase review noise and risk unintended side effects.
- **Prefer small, focused PRs.** Each PR should do one thing and be easy to review end-to-end by the relevant owner.
- **Do not refactor or clean up code outside the scope of the task.** If you notice something worth improving elsewhere, note it but do not act on it without being asked.
- **No speculative abstractions.** Do not introduce helpers, interfaces, or generalizations for hypothetical future use. Build exactly what the current task requires.

## Status Conditions Pattern

When adding or modifying status conditions on `ServerBootConfiguration`:

- Use `retry.RetryOnConflict` + `client.MergeFrom` for all status patches to avoid lost-update races when two controllers patch the same object concurrently.
- Never read a child resource's `Status.State` immediately after a spec apply-patch — the child controller hasn't reconciled the new spec yet. Guard with `child.Status.ObservedGeneration < child.Generation` and return `nil` (write nothing) until the child's own status update re-triggers the reconcile via `Owns()`.
- New child CRD types that feed into a parent condition should expose `ObservedGeneration int64` in their status so converters can detect stale state.

## Documentation

- Keep `CLAUDE.md` and any user-facing docs (under `docs/`) accurate and up to date as features are added or changed.
- When implementing a new feature, update the relevant section of `CLAUDE.md` (Architecture, CRDs, Key CLI Flags, etc.) if the change introduces new concepts, flags, or behavior.
- If you notice documentation that is stale or incorrect relative to the current code, fix it.

## Tests

- **Reuse existing test infrastructure.** Each controller package has a `suite_test.go` with shared setup (namespace, manager, mock registry). Add new tests to the existing `*_test.go` files in the same package — do not create separate suites unless there is a clear reason.
- **Avoid duplication.** Before adding a helper or fixture, check whether an equivalent already exists in the suite. Reuse `SetupTest()`, `MockImageRef()`, and the shared mock registry rather than duplicating them.
- **Cover happy paths and key edge/failure cases**, but keep each test case focused. A test that clearly expresses one scenario is better than a large table-driven test that obscures intent.
- **Scope tests to the controller under test.** An HTTPBoot test should not implicitly depend on iPXE controller behavior, and vice versa.
- **Do not add test-only helpers to non-test packages** (e.g., `PushDualModeImage` in the mock registry). Test helpers belong in `_test.go` files or the `test/` package.

## Mandatory Post-Change Verification

**Once the entire implementation plan is complete, run all of the following commands and fix any failures before presenting the result for review. Do NOT run these after each individual file edit — run them once at the end when all changes are done:**

```bash
# 1. Regenerate manifests and DeepCopy (ONLY when api/v1alpha1/ types were changed)
make manifests generate

# 2. Format code
make fmt

# 3. Run tests
make test

# 4. Run linter (use lint-fix to auto-fix where possible)
make lint

# 5. Verify SPDX/license headers on all files
make check-license
```

### Rules

- **`make manifests generate`**: Run whenever any file under `api/v1alpha1/` is modified (type fields, kubebuilder markers, etc.). This regenerates CRD YAML and DeepCopy methods.
- **`make fmt`**: Always run — formats Go code with goimports.
- **`make test`**: Run once at the end of the complete implementation — must pass with zero failures. Do not run after each individual file edit.
- **`make lint`**: Always run. Prefer `make lint-fix` to auto-correct fixable issues; manually fix the rest.
- **`make check-license`**: Always run. For any **new** Go file created, first run `make add-license` to insert the SPDX header, then verify with `make check-license`. New non-Go files that are not covered by an existing `.reuse/dep5` entry also need SPDX coverage — add them to `.reuse/dep5` if they lack inline headers.

## References

### Essential Reading
- **Kubebuilder Book**: https://book.kubebuilder.io (comprehensive guide)
- **controller-runtime FAQ**: https://github.com/kubernetes-sigs/controller-runtime/blob/main/FAQ.md (common patterns and questions)
- **Good Practices**: https://book.kubebuilder.io/reference/good-practices.html (why reconciliation is idempotent, status conditions, etc.)
- **Logging Conventions**: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-instrumentation/logging.md#message-style-guidelines (message style, verbosity levels)

### API Design & Implementation
- **API Conventions**: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md
- **Operator Pattern**: https://kubernetes.io/docs/concepts/extend-kubernetes/operator/
- **Markers Reference**: https://book.kubebuilder.io/reference/markers.html

### Tools & Libraries
- **controller-runtime**: https://github.com/kubernetes-sigs/controller-runtime
- **controller-tools**: https://github.com/kubernetes-sigs/controller-tools
- **Kubebuilder Repo**: https://github.com/kubernetes-sigs/kubebuilder