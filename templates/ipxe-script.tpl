#!ipxe

set ipxe-svc {{.IPXEServerURL}}
set kernel-url {{.KernelURL}}
set initrd-url {{.InitrdURL}}
set squashfs-url {{.SquashfsURL}}


kernel ${kernel-url} initrd=rootfs.initrd gl.ovl=/:tmpfs gl.url=${squashfs-url} gl.live=1 ip=dhcp6 ignition.firstboot=1 ignition.config.url=${ipxe-svc}/ignition/${uuid} ignition.platform.id=metal console=ttyS0,115200 console=tty0 earlyprintk=ttyS0,115200 consoleblank=0
initrd ${initrd-url}
boot