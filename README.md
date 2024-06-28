# boot-operator

[![REUSE status](https://api.reuse.software/badge/github.com/ironcore-dev/boot-operator)](https://api.reuse.software/info/github.com/ironcore-dev/boot-operator)
[![Go Report Card](https://goreportcard.com/badge/github.com/ironcore-dev/boot-operator)](https://goreportcard.com/report/github.com/ironcore-dev/boot-operator)
[![GitHub License](https://img.shields.io/static/v1?label=License&message=Apache-2.0&color=blue)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://makeapullrequest.com)

The Boot Operator is a Kubernetes controller designed to streamline the management of Boot infrastructure such as HTTPBoot within Kubernetes environments. This operator simplifies network booting processes by automating HTTPBoot UKI URL generation and ignition content delivery based on Kubernetes Custom Resource Definitions (CRDs).

## Key Components
- __HTTP Boot Server__: Serves dynamic HTTP Boot Responses and ignition content through HTTP endpoints, tailored to individual machine specifications.

- __Reconciler__: Configures the HTTP Server based on the desired state specified in `HTTPBootConfig` CRs, ensuring the server's configuration aligns with cluster resources.

- __Translator (Optional)__: Converts `BootConfig` CustomResources from MetalAPI provided by `Ironcore` into the format expected by the HTTPBoot Operator, enhancing integration capabilities.


## HTTP Server Endpoints
- `/ignition/{UUID}`: Matches an `HTTPBootConfig` using the provided `{UUID}` (Spec.systemUUID) and serves the associated ignition content.

- `/httpboot`: Identifies the corresponding `HTTPBootConfig` based on the requester's system IP (Spec.SystemIP). It then returns the customized UKIURL associated with the `HTTPBootConfig` - nota bene: the webserver providing the UKIs should set the content-type to `application/efi` otherwise certain HTTPBoot clients might reject it.

## Getting Started

### Prerequisites
- go version v1.21.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/boot-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified. 
And it is required to have access to pull the image from the working environment. 
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/boot-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin 
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/boot-operator:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/boot-operator/<tag or branch>/dist/install.yaml
```

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)


## Roadmap
Looking ahead, the boot-Operator aims to introduce a range of enhancements to further empower Kubernetes-driven infrastructure provisioning:

- Configurable iPXE Scripts: Enable customization of iPXE script templates to accommodate diverse booting requirements.

- Custom Image Registry Support: Dynamically generate URLs for the kernel, initrd, and squashfs images from a specified image registry, facilitating streamlined updates and deployments.

- Expanded Endpoints: Introduce additional endpoints, such as `/ztp` for Zero Touch Provisioning of switches and `/certs` for certificate management, broadening the operator's utility.

- Enhanced Indexing: Implement indexing based on MAC addresses in addition to the existing SystemUUID and SystemIP, offering more granular control and identification of network boot targets.


## Contributing

We'd love to get feedback from you. Please report bugs, suggestions or post questions by opening a GitHub issue.

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

