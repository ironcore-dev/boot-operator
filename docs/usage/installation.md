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
| `controllerManager.replicas`       | Number of replicas for the manager deployment                               | `1`                            |
| `controllerManager.manager.image.repository` | Image repository for the manager container                                  | `registry/boot-operator`      |
| `controllerManager.manager.image.tag`        | Image tag for the manager container                                         | `"v0.1.0"`                     |
| `controllerManager.manager.args`             | Arguments for the manager container                                         | `--probe-image=probe-image`, `--probe-os-image=probe-os-image`, `--registry-url=registry-url` |
| `controllerManager.manager.resources`        | Resource requests and limits for the manager container                      | `{cpu: 500m, memory: 128Mi}` (limits), `{cpu: 10m, memory: 64Mi}` (requests) |
| `controllerManager.manager.livenessProbe`    | Liveness probe configuration for the manager container                      | `{initialDelaySeconds: 15, periodSeconds: 20, httpGet: {path: /healthz, port: 8081}}` |
| `controllerManager.manager.readinessProbe`   | Readiness probe configuration for the manager container                     | `{initialDelaySeconds: 5, periodSeconds: 10, httpGet: {path: /readyz, port: 8081}}` |
| `controllerManager.manager.containerSecurityContext`  | Security context for the manager container                                  | `{allowPrivilegeEscalation: false, capabilities: {drop: ["ALL"]}}` |
| `controllerManager.podSecurityContext`            | Security context for the manager pod                                        | `{runAsNonRoot: true, seccompProfile: {type: RuntimeDefault}}` |
| `controllerManager.terminationGracePeriodSeconds` | Termination grace period for the manager pod                                | `10`                           |
| `controllerManager.serviceAccountName`         | Service account name for the manager pod                                    | `boot-operator-controller-manager` |
| `controllerManager.nodeSelector`               | Node selector for the manager pod                                           | `{kubernetes.io/os: linux, kubernetes.io/arch: arm64}` |
| `controllerManager.tolerations`                | Tolerations for the manager pod                                             | `[{key: node-role.kubernetes.io/control-plane, effect: NoSchedule}]` |

- **rbac**: Enable or disable RBAC.
- **crd**: Enable or disable CRDs.
- **metrics**: Enable or disable metrics export.
- **webhook**: Enable or disable webhooks.
- **prometheus**: Enable or disable Prometheus ServiceMonitor.
- **certmanager**: Enable or disable cert-manager injection.
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