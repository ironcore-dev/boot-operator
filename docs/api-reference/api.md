# API Reference

## Packages
- [boot.ironcore.dev/v1alpha1](#bootironcoredevv1alpha1)


## boot.ironcore.dev/v1alpha1

Package v1alpha1 contains API Schema definitions for the settings.gardener.cloud API group

Package v1alpha1 contains API Schema definitions for the boot v1alpha1 API group

### Resource Types
- [HTTPBootConfig](#httpbootconfig)
- [IPXEBootConfig](#ipxebootconfig)



#### HTTPBootConfig



HTTPBootConfig is the Schema for the httpbootconfigs API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `boot.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `HTTPBootConfig` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[HTTPBootConfigSpec](#httpbootconfigspec)_ |  |  |  |
| `status` _[HTTPBootConfigStatus](#httpbootconfigstatus)_ |  |  |  |


#### HTTPBootConfigSpec



HTTPBootConfigSpec defines the desired state of HTTPBootConfig



_Appears in:_
- [HTTPBootConfig](#httpbootconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `systemUUID` _string_ | SystemUUID is the unique identifier (UUID) of the server. |  |  |
| `ignitionSecretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | IgnitionSecretRef is a reference to the secret containing Ignition configuration. |  |  |
| `networkIdentifiers` _string array_ | NetworkIdentifiers is a list of IP addresses and MAC Addresses assigned to the server. |  |  |
| `ukiURL` _string_ | UKIURL is the URL where the UKI (Unified Kernel Image) is hosted. |  |  |


#### HTTPBootConfigState

_Underlying type:_ _string_





_Appears in:_
- [HTTPBootConfigStatus](#httpbootconfigstatus)

| Field | Description |
| --- | --- |
| `Ready` | HTTPBootConfigStateReady indicates that the HTTPBootConfig has been successfully processed, and the next step (e.g., booting the server) can proceed.<br /> |
| `Pending` | HTTPBootConfigStatePending indicates that the HTTPBootConfig has not been processed yet.<br /> |
| `Error` | HTTPBootConfigStateError indicates that an error occurred while processing the HTTPBootConfig.<br /> |


#### HTTPBootConfigStatus



HTTPBootConfigStatus defines the observed state of HTTPBootConfig



_Appears in:_
- [HTTPBootConfig](#httpbootconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `state` _[HTTPBootConfigState](#httpbootconfigstate)_ |  |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions represent the latest available observations of the IPXEBootConfig's state |  |  |


#### IPXEBootConfig



IPXEBootConfig is the Schema for the ipxebootconfigs API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `boot.ironcore.dev/v1alpha1` | | |
| `kind` _string_ | `IPXEBootConfig` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[IPXEBootConfigSpec](#ipxebootconfigspec)_ |  |  |  |
| `status` _[IPXEBootConfigStatus](#ipxebootconfigstatus)_ |  |  |  |


#### IPXEBootConfigSpec



IPXEBootConfigSpec defines the desired state of IPXEBootConfig



_Appears in:_
- [IPXEBootConfig](#ipxebootconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `systemUUID` _string_ | SystemUUID is the unique identifier (UUID) of the server. |  |  |
| `systemIPs` _string array_ | SystemIPs is a list of IP addresses assigned to the server. |  |  |
| `image` _string_ | Image is deprecated and will be removed. |  |  |
| `kernelURL` _string_ | KernelURL is the URL where the kernel of the OS is hosted, eg. the URL to the Kernel layer of the OS OCI image. |  |  |
| `initrdURL` _string_ | InitrdURL is the URL where the Initrd (initial RAM disk) of the OS is hosted, eg. the URL to the Initrd layer of the OS OCI image. |  |  |
| `squashfsURL` _string_ | SquashfsURL is the URL where the Squashfs of the OS is hosted, eg.  the URL to the Squashfs layer of the OS OCI image. |  |  |
| `ipxeServerURL` _string_ | IPXEServerURL is deprecated and will be removed. |  |  |
| `ignitionSecretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | IgnitionSecretRef is a reference to the secret containing the Ignition configuration. |  |  |
| `ipxeScriptSecretRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#localobjectreference-v1-core)_ | IPXEScriptSecretRef is a reference to the secret containing the custom IPXE script. |  |  |


#### IPXEBootConfigState

_Underlying type:_ _string_





_Appears in:_
- [IPXEBootConfigStatus](#ipxebootconfigstatus)

| Field | Description |
| --- | --- |
| `Ready` | IPXEBootConfigStateReady indicates that the IPXEBootConfig has been successfully processed, and the next step (e.g., booting the server) can proceed.<br /> |
| `Pending` | IPXEBootConfigStatePending indicates that the IPXEBootConfig has not been processed yet.<br /> |
| `Error` | IPXEBootConfigStateError indicates that an error occurred while processing the IPXEBootConfig.<br /> |


#### IPXEBootConfigStatus



IPXEBootConfigStatus defines the observed state of IPXEBootConfig



_Appears in:_
- [IPXEBootConfig](#ipxebootconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `state` _[IPXEBootConfigState](#ipxebootconfigstate)_ | Important: Run "make" to regenerate code after modifying this file |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions represent the latest available observations of the IPXEBootConfig's state |  |  |


