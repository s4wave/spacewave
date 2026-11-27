package kernel

import "testing"

// Guest memory addresses of the xattr tests' arguments.
const (
	xattrPathPtr  = 2048
	xattrNamePtr  = 2100
	xattrValuePtr = 2200
	xattrBufPtr   = 2300
)

// xattrTestProcess returns a process with the file /tmp/f, its path written
// at xattrPathPtr and the name user.key at xattrNamePtr.
func xattrTestProcess() (*Process, *memMemory) {
	// Create the file and write the arguments.
	p, mem := fsTestProcess()
	openTestFile(p, "f", nil)
	mem.writeCString(xattrPathPtr, "/tmp/f")
	mem.writeCString(xattrNamePtr, "user.key")
	return p, mem
}

// setTestXattr sets the attribute name of /tmp/f to value with flags and
// returns the syscall result.
func setTestXattr(p *Process, mem *memMemory, name, value string, flags uint32) uint64 {
	mem.writeCString(xattrNamePtr, name)
	mem.Write(xattrValuePtr, []byte(value))
	return p.sysSetxattr(xattrPathPtr, xattrNamePtr, xattrValuePtr, uint32(len(value)), flags)
}

// TestSetxattrCreateAndReplace checks setxattr honors XATTR_CREATE and
// XATTR_REPLACE.
func TestSetxattrCreateAndReplace(t *testing.T) {
	// Create and recreate an attribute, replace a missing attribute and replace
	// the existing attribute.
	p, mem := xattrTestProcess()
	tests := []struct {
		name, value string
		flags       uint32
		want        uint64
	}{
		{name: "user.key", value: "one", flags: xattrCreate},
		{name: "user.key", value: "one", flags: xattrCreate, want: negErrno(EEXIST)},
		{name: "user.missing", value: "one", flags: xattrReplace, want: negErrno(ENODATA)},
		{name: "user.key", value: "two", flags: xattrReplace},
	}
	for _, tc := range tests {
		if res := setTestXattr(p, mem, tc.name, tc.value, tc.flags); res != tc.want {
			t.Fatalf("setxattr(%s, %#x) = %#x, want %#x", tc.name, tc.flags, res, tc.want)
		}
	}

	// Check the stored value.
	node := p.kernel.vfs.lookup("/tmp/f")
	if got := string(node.xattrs["user.key"]); got != "two" {
		t.Fatalf("user.key = %q, want two", got)
	}
}

// TestGetxattrSizes checks getxattr reports the value's size, fails with
// ERANGE for a short buffer and copies the value into one that fits.
func TestGetxattrSizes(t *testing.T) {
	// Set the attribute.
	p, mem := xattrTestProcess()
	if res := setTestXattr(p, mem, "user.key", "value", 0); res != 0 {
		t.Fatalf("setxattr = %#x, want 0", res)
	}

	// Read it with no buffer, a short buffer and a buffer that fits.
	for _, tc := range []struct {
		size uint32
		want uint64
	}{{0, 5}, {4, negErrno(ERANGE)}, {5, 5}} {
		if res := p.sysGetxattr(xattrPathPtr, xattrNamePtr, xattrBufPtr, tc.size); res != tc.want {
			t.Fatalf("getxattr with %d bytes = %#x, want %#x", tc.size, res, tc.want)
		}
	}
	if got, _ := mem.Read(xattrBufPtr, 5); string(got) != "value" {
		t.Fatalf("getxattr read %q, want value", got)
	}
}

// TestListxattrSortsNames checks listxattr reports the list's size, fails with
// ERANGE for a short buffer and writes the names sorted, each NUL-terminated.
func TestListxattrSortsNames(t *testing.T) {
	// Set two attributes out of order.
	p, mem := xattrTestProcess()
	for _, name := range []string{"user.b", "user.a"} {
		if res := setTestXattr(p, mem, name, "x", 0); res != 0 {
			t.Fatalf("setxattr(%s) = %#x, want 0", name, res)
		}
	}

	// List them with no buffer, a short buffer and a buffer that fits.
	const want = "user.a\x00user.b\x00"
	for _, tc := range []struct {
		size uint32
		want uint64
	}{{0, uint64(len(want))}, {uint32(len(want) - 1), negErrno(ERANGE)}, {uint32(len(want)), uint64(len(want))}} {
		if res := p.sysListxattr(xattrPathPtr, xattrBufPtr, tc.size); res != tc.want {
			t.Fatalf("listxattr with %d bytes = %#x, want %#x", tc.size, res, tc.want)
		}
	}
	if got, _ := mem.Read(xattrBufPtr, uint32(len(want))); string(got) != want {
		t.Fatalf("listxattr wrote %q, want %q", got, want)
	}
}
