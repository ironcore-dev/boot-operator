#!ipxe

set ipxe-svc {{.IPXEServerURL}}
set kernel-url {{.KernelURL}}
set initrd-url {{.InitrdURL}}
set squashfs-url {{.SquashfsURL}}

echo Loading kernel...
kernel ${kernel-url} initrd=initrd gl.ovl=/:tmpfs gl.url=${squashfs-url} gl.live=1 ip=any ignition.firstboot=1 ignition.config.url=${ipxe-svc}/ignition/${uuid} ignition.platform.id=metal console=ttyS0,115200 console=tty0 console=ttyAMA0 earlyprintk=ttyS0,115200 SYSTEMD_SULOGIN_FORCE=1 consoleblank=0
echo Loading initrd...
initrd ${initrd-url}
echo Booting...
boot
