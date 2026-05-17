package s4wave_apt

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

func TestAptBuildSpecTypeIDAndBlock(t *testing.T) {
	if AptBuildSpecTypeID != "spacewave-vm/apt/build-spec" {
		t.Fatalf("AptBuildSpecTypeID = %q", AptBuildSpecTypeID)
	}

	sourceRef, err := block.BuildBlockRef([]byte("busybox-source"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef: %v", err)
	}
	spec := &AptBuildSpec{
		SourcePackage: "busybox",
		SourceRef:     &bucket.ObjectRef{RootRef: sourceRef},
		BuildConfig: &AptBuildConfig{
			Env: map[string]string{
				"DEB_HOST_ARCH": "i386",
			},
			CrossCompilePrefix: "i686-linux-gnu-",
		},
		Architectures: []string{"i386"},
		BuildDeps:     []string{"zlib"},
	}
	if got := spec.GetBlockTypeId(); got != AptBuildSpecTypeID {
		t.Fatalf("AptBuildSpec block type = %q, want %q", got, AptBuildSpecTypeID)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	data, err := spec.MarshalBlock()
	if err != nil {
		t.Fatalf("MarshalBlock: %v", err)
	}
	var decoded AptBuildSpec
	if err := decoded.UnmarshalBlock(data); err != nil {
		t.Fatalf("UnmarshalBlock: %v", err)
	}
	if !decoded.EqualVT(spec) {
		t.Fatalf("decoded build spec does not match original: got=%s want=%s", decoded.String(), spec.String())
	}
}

func TestAptBuildSpecValidateRejectsMissingCoreFields(t *testing.T) {
	sourceRef, err := block.BuildBlockRef([]byte("busybox-source"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef: %v", err)
	}
	if err := (&AptBuildSpec{}).Validate(); err == nil {
		t.Fatal("expected missing source_package error")
	}
	if err := (&AptBuildSpec{SourcePackage: "busybox"}).Validate(); err == nil {
		t.Fatal("expected missing source_ref error")
	}
	if err := (&AptBuildSpec{
		SourcePackage: "busybox",
		SourceRef:     &bucket.ObjectRef{RootRef: sourceRef},
	}).Validate(); err == nil {
		t.Fatal("expected missing architectures error")
	}
	if err := (&AptBuildSpec{
		SourcePackage: "busybox",
		SourceRef:     &bucket.ObjectRef{RootRef: sourceRef},
		Architectures: []string{"i386"},
		BuildConfig: &AptBuildConfig{
			Env: map[string]string{"": "i386"},
		},
	}).Validate(); err == nil {
		t.Fatal("expected missing build_config env key error")
	}
}
