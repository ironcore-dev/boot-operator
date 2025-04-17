#!ipxe

set ipxe-svc {{.IPXEServerURL}}

set base-url ${ipxe-svc}/ipxe
chain --replace --autofree ${base-url}/${uuid}