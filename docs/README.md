# Boot-Operator Documentation

## Overview

Boot Operator is a Kubernetes-based project designed to automate the deployment of tools required for booting bare metal servers. It integrates web servers and Kubernetes controllers to manage the entire process of provisioning, booting, and configuring the server.

## Problem It Solves

When a bare metal server boots with a network boot method (e.g., PXE or HTTP boot), it typically contacts a DHCP or proxy DHCP server to obtain the necessary information for booting. The DHCP server then provides the IP address of a TFTP server (for PXE) or a web server (for HTTP boot), along with additional boot parameters.

Traditionally, managing these network boot servers requires manual configuration. Boot Operator automates this by incorporating the boot servers into Kubernetes deployments. By leveraging Kubernetes controllers, each machine's boot process is handled declaratively, making it simpler to manage and scale.

## Key Components

Boot Operator includes the following key components:

  - **IPXE Boot Server**  
    - Handles `/ipxe` requests  
    - Responds with an iPXE script, which the bare metal server uses to download the necessary OS components  
    - This endpoint is typically called directly by the server during boot and is commonly used in PXE boot scenarios


  - **HTTP Boot Server**  
    - Handles `/httpboot` requests  
    - Returns a JSON response containing the location of the UKI (Unified Kernel Image) that the server should download  
    - The DHCP server extension typically handles the response and sends the UKI image location to the server  
    - Common in modern cloud-native bare metal setups, especially for containers and minimal OS images


  - **Image Proxy Server**  
    - Handles `/image` requests
    - Extracts layers from OCI (Open Container Initiative) images, with support for multiple registries (e.g., GHCR, Docker Hub, and any OCI-compliant registry)
    - Downloads specific layers based on the requested URI and image specifications
    - Registry access is controlled via the `--allowed-registries` CLI flag (comma-separated list)
    - By default (when not specified), only **ghcr.io** is allowed
    - Example:
      - `wget http://SERVER_ADDRESS:30007/image?imageName=ghcr.io/ironcore-dev/os-images/gardenlinux&version=1443.10&layerName=application/vnd.ironcore.image.squashfs.v1alpha1.squashfs`

  - **Ignition Server**  
    - Handles `/ignition` requests  
    - Responds with Ignition configuration content tailored to the client machine, identified by its UUID in the request URL.

These servers leverage Kubernetes controllers and API objects to manage the boot process and serve requests from bare metal machines. The architecture and specifics of the controllers and API objects are described in the architecture section of the documentation.

## Registry Validation

Boot Operator enforces OCI registry restrictions at two levels:

1. **Controller level (early validation):** The PXE and HTTP boot controllers validate image references against the registry allow list during reconciliation. This means misconfigured or disallowed registries are rejected immediately when a `ServerBootConfiguration` is created, providing fast feedback before any machine attempts to boot.

2. **Image Proxy Server level (runtime enforcement):** The image proxy server also validates registry domains before proxying layer downloads, acting as a second line of defense.

Registry restrictions are configured via the `--allowed-registries` CLI flag on the manager binary.

### Default Behavior

By default (when `--allowed-registries` is not set), Boot Operator allows only **ghcr.io** registry. This provides a secure-by-default configuration with zero configuration needed for the common case.

### Custom Configuration

To allow additional registries or replace the default, use the `--allowed-registries` flag with a comma-separated list:

```bash
--allowed-registries=ghcr.io,registry.example.com,quay.io
```

**Important:** When you set `--allowed-registries`, it completely replaces the default. If you want to use ghcr.io along with other registries, you must explicitly include `ghcr.io` in your list.

### Registry Matching

- Docker Hub variants (`docker.io`, `index.docker.io`, `registry-1.docker.io`) are normalized to `docker.io` for consistent matching.
- All registry domain matching is case-insensitive.
- Registries not in the allow list are denied.
