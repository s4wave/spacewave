package unixfs_world

import (
	"context"
	"testing"
)

func TestLookupFsOpHandlesAllConcreteOps(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{name: "init", id: FsInitOpId},
		{name: "copy", id: FsCopyOpId},
		{name: "mknod", id: FsMknodOpId},
		{name: "mknod with content", id: FsMknodWithContentOpId},
		{name: "remove", id: FsRemoveOpId},
		{name: "rename", id: FsRenameOpId},
		{name: "set mod timestamp", id: FsSetModTimestampOpId},
		{name: "set permissions", id: FsSetPermissionsOpId},
		{name: "symlink", id: FsSymlinkOpId},
		{name: "truncate", id: FsTruncateOpId},
		{name: "write at", id: FsWriteAtOpId},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op, err := LookupFsOp(context.Background(), tt.id)
			if err != nil {
				t.Fatalf("LookupFsOp(%q): %v", tt.id, err)
			}
			if op == nil {
				t.Fatalf("LookupFsOp(%q) returned nil", tt.id)
			}
			if got := op.GetOperationTypeId(); got != tt.id {
				t.Fatalf("LookupFsOp(%q) returned op type %q", tt.id, got)
			}
		})
	}
}
