package v86_wazero

import unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"

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
	V86FSServer       *unixfs_v86fs.Server
}
