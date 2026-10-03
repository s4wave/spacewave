package v86_wazero

import (
	"context"
	"testing"
)

func TestHostRuntimeIODispatch(t *testing.T) {
	// Create a HostRuntime with an empty I/O port registry.
	host := &HostRuntime{ioPorts: newIOPorts()}
	ctx := context.Background()

	// Verify unregistered I/O reads return all bits set at each width.
	if got := host.readIO(ctx, 0x60, 8); got != 0xff {
		t.Fatalf("default 8-bit io read = %#x, want 0xff", got)
	}
	if got := host.readIO(ctx, 0x60, 16); got != 0xffff {
		t.Fatalf("default 16-bit io read = %#x, want 0xffff", got)
	}
	if got := host.readIO(ctx, 0x60, 32); got != 0xffffffff {
		t.Fatalf("default 32-bit io read = %#x, want 0xffffffff", got)
	}

	// Register an I/O reader that checks the dispatched port.
	host.RegisterIORead(0x60, 8, func(_ context.Context, port uint16) uint32 {
		if port != 0x60 {
			t.Fatalf("read port = %#x, want 0x60", port)
		}
		return 0x1234
	})

	// Verify the I/O reader result is masked to the requested width.
	if got := host.readIO(ctx, 0x60, 8); got != 0x34 {
		t.Fatalf("registered 8-bit io read = %#x, want 0x34", got)
	}

	// Capture the port and value dispatched to the I/O writer.
	var wrotePort uint16
	var wroteValue uint32
	host.RegisterIOWrite(0x61, 16, func(_ context.Context, port uint16, value uint32) {
		wrotePort = port
		wroteValue = value
	})

	// Dispatch an I/O write with a value wider than the registered port.
	host.writeIO(ctx, 0x61, 0x12345678, 16)

	// Verify the I/O writer receives the registered port and masked value.
	if wrotePort != 0x61 || wroteValue != 0x5678 {
		t.Fatalf("registered 16-bit io write = (%#x, %#x), want (0x61, 0x5678)", wrotePort, wroteValue)
	}
}

func TestHostRuntimeMmapDispatch(t *testing.T) {
	// Create a HostRuntime with an empty memory-mapped I/O registry.
	host := &HostRuntime{mmapBlocks: make(map[uint32]mmapBlock)}

	// Verify unregistered memory-mapped reads return all bits set.
	if got := host.readMmap(0xf0000000, 8); got != 0xff {
		t.Fatalf("default 8-bit mmap read = %#x, want 0xff", got)
	}
	if got := host.readMmap(0xf0000000, 32); got != 0xffffffff {
		t.Fatalf("default 32-bit mmap read = %#x, want 0xffffffff", got)
	}

	// Register byte and word readers for the first memory-mapped block.
	host.RegisterMmap(
		0xf0000000,
		0x20000,
		func(addr uint32) uint32 {
			if addr != 0xf0000004 {
				t.Fatalf("mmap read addr = %#x, want 0xf0000004", addr)
			}
			return 0x123
		},
		nil,
		func(uint32) uint32 { return 0x12345678 },
		nil,
	)

	// Verify memory-mapped reads dispatch to the requested width.
	if got := host.readMmap(0xf0000004, 8); got != 0x23 {
		t.Fatalf("registered 8-bit mmap read = %#x, want 0x23", got)
	}
	if got := host.readMmap(0xf0000004, 32); got != 0x12345678 {
		t.Fatalf("registered 32-bit mmap read = %#x, want 0x12345678", got)
	}

	// Capture byte and word writes to the second memory-mapped block.
	var writes []uint32
	host.RegisterMmap(
		0xf0020000,
		0x20000,
		nil,
		func(addr uint32, value uint32) {
			writes = append(writes, addr, value)
		},
		nil,
		func(addr uint32, value uint32) {
			writes = append(writes, addr, value)
		},
	)

	// Dispatch halfword and word writes through the memory-mapped block.
	host.writeMmap(0xf0020000, 0x1122, 16)
	host.writeMmap(0xf0020004, 0x33445566, 32)

	// Verify halfword writes split into bytes and word writes remain intact.
	want := []uint32{0xf0020000, 0x22, 0xf0020001, 0x11, 0xf0020004, 0x33445566}
	if len(writes) != len(want) {
		t.Fatalf("mmap writes = %#v, want %#v", writes, want)
	}
	for i := range want {
		if writes[i] != want[i] {
			t.Fatalf("mmap writes = %#v, want %#v", writes, want)
		}
	}
}
