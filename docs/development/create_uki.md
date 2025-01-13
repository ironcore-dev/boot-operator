### How to Generate a UKI Image for HTTPBoot with Gardenlinux

#### Step 1: Prerequisites
- Ensure you have the `ukify` tool installed on your system. This tool is essential for creating the UKI image.
- You will need administrative or root privileges to execute most of the commands described.

#### Step 2: Download and Prepare Gardenlinux Release
1. Download the appropriate Gardenlinux release for your architecture. For example, a metal-based system with an AMD64 architecture, use the following command:
   ```bash
   wget https://github.com/gardenlinux/gardenlinux/releases/download/1443.10/metal-gardener_prod_pxe-amd64-1443.10-8d098305.tar.xz
   ```
2. Extract the downloaded `.tar.xz` file:
   ```bash
   tar -xvf metal-gardener_prod_pxe-amd64-1443.10-8d098305.tar.xz
   ```
3. Further extract the nested `*.pxe.tar.gz` which contains the kernel and initial RAM disk:
   ```bash
   tar -xzf <nested_tar_name>.pxe.tar.gz
   ```
   You should see files like `vmlinuz`, `initrd`, and `root.squashfs`.

#### Step 3: Obtain the Bootloader Stub
Download the EFI stub required for the UKI creation:
```bash
tbd
```

#### Step 4: Create the UKI Image
Construct the UKI image using the `ukify` command. Ensure to replace placeholders with actual paths and URLs:
```bash
ukify build --stub "/path/to/stub" --linux "/path/to/vmlinuz" --initrd "/path/to/initrd" --cmdline "@cmdline" --output "/path/to/output/test.uki"

# Create file with the name cmdline, with following content
# Use this as the sample command line, replace URLs and paths as necessary
initrd=/path/to/initrd gl.ovl=/:tmpfs gl.live=1 ip=dhcp console=ttyS0,115200 console=tty0 earlyprintk=ttyS0,115200 consoleblank=0 ignition.firstboot=1 ignition.config.url=IGNITION_URL ignition.platform.id=metal gl.url=SQUASHFS_URL
```

#### Step 5: Deploy the Image to a Server
Copy the created `test.uki` to an Nginx server configured to serve the files:
```bash
cp /path/to/output/test.uki /path/to/nginx/server/httpboot/test-uki.efi
# Also, ensure the squashfs file is accessible via HTTP
cp /path/to/root.squashfs /path/to/nginx/server/httpboot/squashfs
```
Ensure EFI files are served by NGINX with the correct content-type.
```bash
 application/efi efi;
```

#### Step 6: Configure HTTPBoot
Create a YAML configuration for the HTTPBoot client. Replace placeholders as required:
```yaml
apiVersion: boot.ironcore.dev/v1alpha1
kind: HTTPBootConfig
metadata:
  name: httpbootconfig-sample
  namespace: boot-operator-system
spec:
  ignitionSecretRef:
    name: ignition-http-sample
    namespace: boot-operator-system
  systemUUID: "generate-this-uuid"
  systemIPs:
    - "1.1.1.1"
    - "ip/mac-address-of-interfaces"
  ukiURL: "http://[your-server-ip-or-domain]/httpboot/test-uki.efi"
```

Apply this configuration to your cluster and ensure the metal machine is set to boot via HTTPBoot.