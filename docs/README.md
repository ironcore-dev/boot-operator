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
    - Extracts layers from public OCI (Open Container Initiative) images, with current support for GHCR (GitHub Container Registry) only 
    - Downloads specific layers based on the requested URI and image specifications  
    - Example:
      - `wget http://SERVER_ADDRESS:30007/image?imageName=ghcr.io/ironcore-dev/os-images/gardenlinux&version=1443.10&layerName=application/vnd.ironcore.image.squashfs.v1alpha1.squashfs`

  - **Ignition Server**  
    - Handles `/ignition` requests  
    - Responds with Ignition configuration content tailored to the client machine, identified by its UUID in the request URL.

These servers leverage Kubernetes controllers and API objects to manage the boot process and serve requests from bare metal machines. The architecture and specifics of the controllers and API objects are described in the architecture section of the documentation.