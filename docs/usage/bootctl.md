# bootctl

## Installation

Install the `bootctl` CLI from source without cloning the repository. Requires [Go](https://go.dev) to be installed.

```bash
go install https://github.com/ironcore-dev/boot-operator/cmd/bootctl@latest
```

## Commands

### move

The `bootctl move` command allows to move the boot Custom Resources which are `HTTPBootConfigs` and `IPXEBootConfigs` from one cluster to another.

> Warning!:
> Before running `bootctl move`, the user should take care of preparing the target cluster, including also installing
> all the required Custom Resources Definitions.

You can use:

```bash
bootctl move --source-kubeconfig="path-to-source-kubeconfig.yaml" --target-kubeconfig="path-to-target-kubeconfig.yaml"
```
to move the boot Custom Resources existing in all namespaces of the source cluster. In case you want to move the boot
Custom Resources defined in a single namespace, you can use the `--namespace` flag.

Secrets referred in the specification and ownership of a boot Custom Resource is also moved. If for a moved boot Custom Resource with an ownership there is no `ServerBootConfiguration` matching resource name on the target cluster, the owner won't be set and the move operation will succeed. To fail when such situation occurs set `--require-owners` to true. If a boot Custom Resource present on the source cluster exists on the target cluster with identical specification it and its secret won't be moved and no ownership of this object will be set on the target cluster. If the boot Custom Resource is absent on the target cluster but its secret is present, there will be no errors and the move operation will succeed. In case of any errors during the process there will be performed a cleanup and the target cluster will be restored to its previous state.

> Warning!: 
`bootctl move` has been designed and developed around the bootstrap use case described below, and currently this is
the only use case verified.
>
>If someone intends to use `bootctl move` outside of this scenario, it's recommended to set up a custom validation
pipeline of it before using the command on a production environment.
>
>Also, it is important to notice that move has not been designed for being used as a backup/restore solution and it has
several limitation for this scenario, like e.g. the implementation assumes the cluster must be stable while doing the
move operation, and possible race conditions happening while the cluster is upgrading, scaling up, remediating etc. has
never been investigated nor addressed.

#### Pivot

Pivoting is a process for moving the Custom Resources and install Custom Resource Definitions from a source cluster to
a target cluster.
 
This can now be achieved with the following procedure:

1. Use `make install` to install the boot Custom Resource Definitions into the target cluster
2. Use `bootctl move` to move the boot Custom Resources from a source cluster to a target cluster

#### Dry run

With `--dry-run` option you can dry-run the move action by only printing logs without taking any actual actions. Use
`--verbose` flag to enable verbose logging.
