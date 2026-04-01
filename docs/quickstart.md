# Quickstart

Boot Operator is Kubernetes-native infrastructure for bare metal network boot: it serves HTTPBoot and iPXE (plus supporting endpoints like image proxy and ignition) based on declarative CRDs.

This quickstart gets you from zero to a running Boot Operator deployment and your first boot configuration.

By the end you will have:

- A controller deployment running in your cluster
- The Boot Operator APIs (CRDs) installed and ready to use
- A clear path to wiring your machines to HTTPBoot or iPXE and iterating from there

## Install

Choose one of:

- [Helm](./installation/helm.md)
- [Kustomize](./installation/kustomize.md)

## Next Steps

- Read the [Architecture](./architecture.md)
- Use the CLI: [bootctl](./usage/bootctl.md)
- Browse the [API Reference](./api-reference/api.md)
