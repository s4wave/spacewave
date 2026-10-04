package s4wave_vm

import (
	"testing"
)

func TestV86CleanIDs(t *testing.T) {
	// Prepare the expected VM type and operation identifiers.
	cases := []struct {
		name string
		got  string
		want string
	}{
		{name: "vm type", got: VmV86TypeID, want: "vm/v86"},
		{name: "v86 image type", got: V86ImageTypeID, want: "vm/image/v86"},
		{name: "create vm op", got: CreateVmV86OpId, want: "vm/v86/create"},
		{name: "set config op", got: SetV86ConfigOpId, want: "vm/v86/set-config"},
		{name: "set state op", got: SetV86StateOpId, want: "vm/v86/set-state"},
		{name: "create image op", got: CreateV86ImageOpId, want: "vm/image/v86/create"},
		{name: "set image metadata op", got: SetV86ImageMetadataOpId, want: "vm/image/v86/set-metadata"},
	}

	// Verify each VM identifier matches its public value.
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Fatalf("%s ID = %q, want %q", tc.name, tc.got, tc.want)
		}
	}

	// Verify VM blocks and operations expose the expected type identifiers.
	if got := (&VmV86{}).GetBlockTypeId(); got != VmV86TypeID {
		t.Fatalf("VmV86 block type = %q, want %q", got, VmV86TypeID)
	}
	if got := (&V86Image{}).GetBlockTypeId(); got != V86ImageTypeID {
		t.Fatalf("V86Image block type = %q, want %q", got, V86ImageTypeID)
	}
	if got := (&CreateVmV86Op{}).GetOperationTypeId(); got != CreateVmV86OpId {
		t.Fatalf("CreateVmV86Op type = %q, want %q", got, CreateVmV86OpId)
	}
	if got := (&SetV86ConfigOp{}).GetOperationTypeId(); got != SetV86ConfigOpId {
		t.Fatalf("SetV86ConfigOp type = %q, want %q", got, SetV86ConfigOpId)
	}
	if got := (&SetV86StateOp{}).GetOperationTypeId(); got != SetV86StateOpId {
		t.Fatalf("SetV86StateOp type = %q, want %q", got, SetV86StateOpId)
	}
	if got := (&CreateV86ImageOp{}).GetOperationTypeId(); got != CreateV86ImageOpId {
		t.Fatalf("CreateV86ImageOp type = %q, want %q", got, CreateV86ImageOpId)
	}
	if got := (&SetV86ImageMetadataOp{}).GetOperationTypeId(); got != SetV86ImageMetadataOpId {
		t.Fatalf("SetV86ImageMetadataOp type = %q, want %q", got, SetV86ImageMetadataOpId)
	}
}
