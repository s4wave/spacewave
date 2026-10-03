package v86_wazero

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_tar "github.com/s4wave/spacewave/db/unixfs/tar"
	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
)

// TestV86FSServerRootTraversal drives the v86fs LocalSession through the exact
// root-boot path (mount -> lookup usr/bin/bash -> open -> read) against the real
// rootfs tar, isolating server correctness from the VM wire layer.
func TestV86FSServerRootTraversal(t *testing.T) {
	// Require the enabled V86 rootfs fixture before traversing its files.
	if !runV86WazeroBootTests() {
		t.Skip("set RUN_V86_WAZERO_BOOT=true to exercise the v86fs server against the real rootfs tar")
	}

	// Bound the V86 filesystem session lifetime.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Resolve the real V86 filesystem image.
	assets, err := ResolveAssets(ctx, OptionsFromEnv())
	if err != nil {
		t.Fatalf("resolve v86 assets: %v", err)
	}

	// Open the rootfs tar for filesystem cursor construction.
	rootfs, err := os.Open(assets.RootfsTar)
	if err != nil {
		t.Fatalf("open rootfs tar: %v", err)
	}
	defer rootfs.Close()

	// Parse the rootfs tar into a filesystem cursor.
	rootCursor, err := unixfs_tar.NewTarFSCursorFromReader(rootfs)
	if err != nil {
		t.Fatalf("parse rootfs tar: %v", err)
	}

	// Acquire the filesystem handle for the root mount.
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		t.Fatalf("build rootfs handle: %v", err)
	}
	defer rootHandle.Release()

	// Mount the rootfs in a local V86 filesystem session.
	server := unixfs_v86fs.NewServer(nil, nil)
	server.AddMount("", "/", rootHandle)
	session := unixfs_v86fs.NewLocalSession(ctx, server)
	defer session.Close()

	// Route guest filesystem requests through the local session.
	call := func(req *unixfs_v86fs.V86FsMessage) *unixfs_v86fs.V86FsMessage {
		// Handle the guest request and reject transport failures.
		t.Helper()
		reply, err := session.HandleMessage(ctx, req)
		if err != nil {
			t.Fatalf("handle message: %v", err)
		}

		// Require a typed filesystem response from the server.
		if er := reply.GetErrorReply(); er != nil {
			t.Fatalf("server returned error reply status=%d for %T", er.GetStatus(), req.GetBody())
		}

		return reply
	}

	// Mount the guest root and verify its directory metadata.
	mount := call(&unixfs_v86fs.V86FsMessage{
		Body: &unixfs_v86fs.V86FsMessage_MountRequest{
			MountRequest: &unixfs_v86fs.V86FsMountRequest{Name: ""},
		},
	}).GetMountReply()
	if mount == nil {
		t.Fatal("mount reply missing")
	}
	t.Logf("mount: status=%d root_inode_id=%d mode=%#o", mount.GetStatus(), mount.GetRootInodeId(), mount.GetMode())
	if mount.GetStatus() != 0 {
		t.Fatalf("mount status=%d", mount.GetStatus())
	}
	const sIFDIR = 0o040000
	if mount.GetMode()&0o170000 != sIFDIR {
		t.Fatalf("root mode %#o is not a directory (S_IFDIR missing)", mount.GetMode())
	}

	// Traverse the rootfs path to the guest bash executable.
	parent := mount.GetRootInodeId()
	for _, name := range []string{"usr", "bin", "bash"} {
		// Look up the next bash path component in the filesystem session.
		look := call(&unixfs_v86fs.V86FsMessage{
			Body: &unixfs_v86fs.V86FsMessage_LookupRequest{
				LookupRequest: &unixfs_v86fs.V86FsLookupRequest{ParentId: parent, Name: name},
			},
		}).GetLookupReply()
		if look == nil {
			t.Fatalf("lookup %q reply missing", name)
		}
		t.Logf("lookup %q: inode_id=%d mode=%#o size=%d", name, look.GetInodeId(), look.GetMode(), look.GetSize())
		if look.GetStatus() != 0 {
			t.Fatalf("lookup %q status=%d", name, look.GetStatus())
		}
		parent = look.GetInodeId()
	}

	// Open the guest bash executable through the filesystem session.
	open := call(&unixfs_v86fs.V86FsMessage{
		Body: &unixfs_v86fs.V86FsMessage_OpenRequest{
			OpenRequest: &unixfs_v86fs.V86FsOpenRequest{InodeId: parent},
		},
	}).GetOpenReply()
	if open == nil || open.GetStatus() != 0 {
		t.Fatalf("open /usr/bin/bash failed: %+v", open)
	}

	// Read the guest bash header through the filesystem session.
	read := call(&unixfs_v86fs.V86FsMessage{
		Body: &unixfs_v86fs.V86FsMessage_ReadRequest{
			ReadRequest: &unixfs_v86fs.V86FsReadRequest{HandleId: open.GetHandleId(), Offset: 0, Size: 4},
		},
	}).GetReadReply()
	if read == nil || read.GetStatus() != 0 {
		t.Fatalf("read /usr/bin/bash failed: %+v", read)
	}

	// Verify the guest bash header contains the ELF magic bytes.
	data := read.GetData()
	t.Logf("read %d bytes head=%v", len(data), data)
	if len(data) < 4 || data[0] != 0x7f || data[1] != 'E' || data[2] != 'L' || data[3] != 'F' {
		t.Fatalf("/usr/bin/bash does not start with ELF magic: %v", data)
	}

	// A missing child must return a typed LOOKUP reply with an ENOENT status,
	// not an error reply: the guest maps any non-LOOKUP_R reply to EIO and never
	// creates a negative dentry, which breaks ordinary missing-file probes.
	missing, err := session.HandleMessage(ctx, &unixfs_v86fs.V86FsMessage{
		Body: &unixfs_v86fs.V86FsMessage_LookupRequest{
			LookupRequest: &unixfs_v86fs.V86FsLookupRequest{ParentId: mount.GetRootInodeId(), Name: ".bashrc"},
		},
	})
	if err != nil {
		t.Fatalf("missing lookup returned transport error: %v", err)
	}
	if missing.GetErrorReply() != nil {
		t.Fatalf("missing lookup returned ERROR_REPLY status=%d; guest would map this to EIO", missing.GetErrorReply().GetStatus())
	}
	neg := missing.GetLookupReply()
	if neg == nil {
		t.Fatalf("missing lookup did not return a typed LOOKUP reply: %T", missing.GetBody())
	}
	const enoent = 2
	if neg.GetStatus() != enoent {
		t.Fatalf("missing lookup status=%d, want ENOENT(%d)", neg.GetStatus(), enoent)
	}
}
