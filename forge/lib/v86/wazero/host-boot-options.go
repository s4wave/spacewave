package v86_wazero

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
}
