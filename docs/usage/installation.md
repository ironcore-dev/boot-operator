# Helm Installation Guide

This guide will help you install the Boot Operator using Helm.

## Prerequisites

- Kubernetes cluster (v1.16+)
- Helm (v3.0.0+)

## Steps

1. **Install the Chart**

   Install the Boot Operator chart with the default values.

   ```sh
   helm install boot-operator dist/chart
   ```

   To customize the installation, you can override the default values using a `values.yaml` file or the `--set` flag.

   ```sh
   helm install boot-operator dist/chart -f /path/to/your/values.yaml
   ```

2. **Verify the Installation**

   Check the status of the Helm release to ensure that the Boot Operator is installed successfully.

   ```sh
   helm status boot-operator
   ```

   You should see output indicating that the Boot Operator pods are running.

## Configuration

The `values.yaml` file allows you to configure various aspects of the Boot Operator. Below are some of the key configurations:

### Controller Manager

| Key                                | Description                                                                 | Default Value                  |
|------------------------------------|-----------------------------------------------------------------------------|--------------------------------|
| `manager.enabled`                  | Install the controller manager deployment                                   | `true`                         |
| `manager.replicas`                 | Number of replicas for the manager deployment                               | `1`                            |
| `manager.image.repository`         | Image repository for the manager container                                  | `controller`                   |
| `manager.image.tag`                | Image tag for the manager container (defaults to `Chart.appVersion`)        | `""`                           |
| `manager.image.pullPolicy`         | Image pull policy for the manager container                                 | `IfNotPresent`                 |
| `manager.args`                     | Arguments for the manager container                                         | `[--leader-elect, --ipxe-service-url=ipxe-service-url]` |
| `manager.resources`                | Resource requests and limits for the manager container                      | `{cpu: 500m, memory: 128Mi}` (limits), `{cpu: 10m, memory: 64Mi}` (requests) |
| `manager.healthProbe.port`         | Port serving the `/healthz` and `/readyz` endpoints                         | `8081`                         |
| `manager.securityContext`          | Container-level security context                                            | `{allowPrivilegeEscalation: false, capabilities: {drop: ["ALL"]}}` |
| `manager.podSecurityContext`       | Pod-level security context                                                  | `{runAsNonRoot: true, seccompProfile: {type: RuntimeDefault}}` |
| `manager.terminationGracePeriodSeconds` | Termination grace period for the manager pod                           | `10`                           |
| `manager.tolerations`              | Tolerations for the manager pod                                             | `[]`                           |
| `manager.nodeSelector`             | Node selector for the manager pod                                           | `{}`                           |
| `manager.affinity`                 | Affinity rules for the manager pod                                          | `{}`                           |

- **serviceAccount**: Install and configure the manager ServiceAccount.
- **rbac**: Configure RBAC scope (`namespaced`) and optional CRD helper roles (`helpers.enabled`).
- **crd**: Install CRDs with the chart (`enabled`) and keep them on uninstall (`keep`).
- **metrics**: Enable or disable the metrics endpoint (`enabled`, `port`, `secure`).
- **webhook**: Enable or disable webhooks.
- **prometheus**: Enable or disable the Prometheus ServiceMonitor.
- **certManager**: Enable or disable cert-manager integration for TLS certificates.
- **networkPolicy**: Enable or disable NetworkPolicies.

Refer to the `values.yaml` file for more details on each configuration option.

## Uninstallation

To uninstall the Boot Operator, run the following command:

```sh
helm uninstall boot-operator
```

This will remove all the resources associated with the Boot Operator.

## Additional Information

For more detailed information, refer to the official documentation and Helm chart repository.