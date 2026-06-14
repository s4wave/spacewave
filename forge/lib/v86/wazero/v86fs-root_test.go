package v86_wazero

import (
	"archive/tar"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
)

func TestParseRootMode(t *testing.T) {
	cases := []struct {
		value string
		mode  RootMode
	}{
		{"", RootMode{Mode: rootModeRAM}},
		{"readonly", RootMode{Mode: rootModeReadonly}},
		{"ram", RootMode{Mode: rootModeRAM}},
		{"disk=/tmp/v86-root", RootMode{Mode: rootModeDisk, Arg: "/tmp/v86-root"}},
		{"volume=/tmp/root.img", RootMode{Mode: rootModeVolume, Arg: "/tmp/root.img"}},
		{"daemon=workspace", RootMode{Mode: rootModeDaemon, Arg: "workspace"}},
	}
	for _, c := range cases {
		got, err := ParseRootMode(c.value)
		if err != nil {
			t.Fatalf("parse %q: %v", c.value, err)
		}
		if got != c.mode {
			t.Fatalf("parse %q = %+v, want %+v", c.value, got, c.mode)
		}
	}

	for _, value := range []string{"bad", "disk", "disk=", "readonly=x", "ram=x"} {
		if _, err := ParseRootMode(value); err == nil {
			t.Fatalf("parse %q expected error", value)
		}
	}
}

func TestOpenV86RootModes(t *testing.T) {
	rootfsTar := writeV86RootTestTar(t)
	cases := []struct {
		name     string
		value    string
		writable bool
	}{
		{name: "readonly", value: "readonly"},
		{name: "ram", value: "ram", writable: true},
		{name: "disk", value: "disk=" + t.TempDir(), writable: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mode, err := ParseRootMode(c.value)
			if err != nil {
				t.Fatal(err)
			}
			server, release, err := OpenV86Root(mode, rootfsTar)
			if err != nil {
				t.Fatalf("open root: %v", err)
			}
			defer release()

			session := unixfs_v86fs.NewLocalSession(context.Background(), server)
			defer session.Close()
			rootID := assertV86RootServesIssue(t, session)
			if c.writable {
				assertV86RootWritable(t, session, rootID, c.name+".txt")
			}
			if mode.Mode == rootModeDisk {
				got, err := os.ReadFile(filepath.Join(mode.Arg, c.name+".txt"))
				if err != nil {
					t.Fatalf("read disk upper file: %v", err)
				}
				if string(got) != "guest-write" {
					t.Fatalf("disk upper file = %q, want guest-write", got)
				}
			}
		})
	}
}

func TestOpenV86RootDeferredModes(t *testing.T) {
	rootfsTar := writeV86RootTestTar(t)
	for _, value := range []string{"volume=/tmp/root.img", "daemon=workspace"} {
		mode, err := ParseRootMode(value)
		if err != nil {
			t.Fatalf("parse %q: %v", value, err)
		}
		server, release, err := OpenV86Root(mode, rootfsTar)
		if err == nil {
			if release != nil {
				release()
			}
			t.Fatalf("open %q returned server=%v without error", value, server)
		}
		if !strings.Contains(err.Error(), "not yet implemented") {
			t.Fatalf("open %q error = %v, want not yet implemented", value, err)
		}
	}
}

func assertV86RootServesIssue(t *testing.T, session *unixfs_v86fs.LocalSession) uint64 {
	t.Helper()
	mount := callV86Root(t, session, &unixfs_v86fs.V86FsMessage{
		Body: &unixfs_v86fs.V86FsMessage_MountRequest{
			MountRequest: &unixfs_v86fs.V86FsMountRequest{Name: ""},
		},
	}).GetMountReply()
	if mount == nil || mount.GetStatus() != 0 || mount.GetRootInodeId() == 0 {
		t.Fatalf("mount root failed: %+v", mount)
	}

	parent := mount.GetRootInodeId()
	for _, name := range []string{"etc", "issue"} {
		lookup := callV86Root(t, session, &unixfs_v86fs.V86FsMessage{
			Body: &unixfs_v86fs.V86FsMessage_LookupRequest{
				LookupRequest: &unixfs_v86fs.V86FsLookupRequest{ParentId: parent, Name: name},
			},
		}).GetLookupReply()
		if lookup == nil || lookup.GetStatus() != 0 || lookup.GetInodeId() == 0 {
			t.Fatalf("lookup %q failed: %+v", name, lookup)
		}
		parent = lookup.GetInodeId()
	}

	open := callV86Root(t, session, &unixfs_v86fs.V86FsMessage{
		Body: &unixfs_v86fs.V86FsMessage_OpenRequest{
			OpenRequest: &unixfs_v86fs.V86FsOpenRequest{InodeId: parent},
		},
	}).GetOpenReply()
	if open == nil || open.GetStatus() != 0 || open.GetHandleId() == 0 {
		t.Fatalf("open /etc/issue failed: %+v", open)
	}
	read := callV86Root(t, session, &unixfs_v86fs.V86FsMessage{
		Body: &unixfs_v86fs.V86FsMessage_ReadRequest{
			ReadRequest: &unixfs_v86fs.V86FsReadRequest{HandleId: open.GetHandleId(), Size: 64},
		},
	}).GetReadReply()
	if read == nil || read.GetStatus() != 0 || string(read.GetData()) != "spacewave\n" {
		t.Fatalf("read /etc/issue = %+v", read)
	}
	return mount.GetRootInodeId()
}

func assertV86RootWritable(t *testing.T, session *unixfs_v86fs.LocalSession, rootID uint64, name string) {
	t.Helper()
	create := callV86Root(t, session, &unixfs_v86fs.V86FsMessage{
		Body: &unixfs_v86fs.V86FsMessage_CreateRequest{
			CreateRequest: &unixfs_v86fs.V86FsCreateRequest{
				ParentId: rootID,
				Name:     name,
				Mode:     0o644,
			},
		},
	}).GetCreateReply()
	if create == nil || create.GetStatus() != 0 || create.GetInodeId() == 0 {
		t.Fatalf("create %q failed: %+v", name, create)
	}
	write := callV86Root(t, session, &unixfs_v86fs.V86FsMessage{
		Body: &unixfs_v86fs.V86FsMessage_WriteRequest{
			WriteRequest: &unixfs_v86fs.V86FsWriteRequest{
				InodeId: create.GetInodeId(),
				Data:    []byte("guest-write"),
			},
		},
	}).GetWriteReply()
	if write == nil || write.GetStatus() != 0 || write.GetBytesWritten() != uint32(len("guest-write")) {
		t.Fatalf("write %q failed: %+v", name, write)
	}
}

func callV86Root(t *testing.T, session *unixfs_v86fs.LocalSession, msg *unixfs_v86fs.V86FsMessage) *unixfs_v86fs.V86FsMessage {
	t.Helper()
	reply, err := session.HandleMessage(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
	if er := reply.GetErrorReply(); er != nil {
		t.Fatalf("v86fs returned error status=%d for %T", er.GetStatus(), msg.GetBody())
	}
	return reply
}

func writeV86RootTestTar(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rootfs.tar")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	if err := tw.WriteHeader(&tar.Header{Name: "etc/", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	body := []byte("spacewave\n")
	if err := tw.WriteHeader(&tar.Header{Name: "etc/issue", Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
