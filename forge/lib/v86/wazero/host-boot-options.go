package v86_wazero

import unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"

const (
	// The guest must boot through /sbin/init so busybox inittab gives ttyS0
	// a controlling terminal with job control; bash as PID 1 breaks Ctrl-C/SIGINT.
	DefaultV86FSRootCmdline  = "rw init=/sbin/init root=v86fs rootfstype=v86fs rootflags= console=ttyS0"
	DefaultHost9PRootCmdline = "rw init=/sbin/init root=host9p rootfstype=9p rootflags=trans=virtio,cache=loose console=ttyS0"
)

// HostBootOptions configures the minimal CPU bootstrap before the run loop.
type HostBootOptions struct {
	EnableJIT         bool
	MemorySize        uint32
	MinimumMemorySize uint32
	BIOS              []byte
	VGABIOS           []byte
	Kernel            []byte
	Initrd            []byte
	Cmdline           string
	Host9PFS          *Host9PFS
	V86FSServer       *unixfs_v86fs.Server
}

func (o HostBootOptions) kernelCmdline() string {
	if o.Cmdline != "" {
		return o.Cmdline
	}
	if o.V86FSServer != nil {
		return DefaultV86FSRootCmdline
	}
	return DefaultHost9PRootCmdline
}
