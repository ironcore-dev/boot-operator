#!ipxe

set ipxe-svc {{.IPXEServerURL}}
set kernel-url {{.KernelURL}}
set initrd-url {{.InitrdURL}}
set squashfs-url {{.SquashfsURL}}

:onerror
echo Boot failed, entering shell...
shell

kernel ${kernel-url} initrd=initrd gl.ovl=/:tmpfs gl.url=${squashfs-url} gl.live=1 ip=dhcp ignition.firstboot=1 ignition.config.url=${ipxe-svc}/ignition/${uuid} ignition.platform.id=metal console=ttyS0,115200 console=tty0 earlyprintk=ttyS0,115200 consoleblank=0 || goto onerror
initrd ${initrd-url} || goto onerror
boot || goto onerror
