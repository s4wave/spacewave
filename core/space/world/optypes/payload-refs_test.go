//go:build !tinygo

package optypes

import (
	"slices"
	"sort"
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	git_world "github.com/s4wave/spacewave/db/git/world"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
)

// payloadRefsGolden records, for each payload operation type the static table
// resolves, a fixture operation and the payload roots GetPayloadRefs names for
// it under PayloadRefsVersion. Each fixture sets every block reference the
// operation carries to a distinct named root, and refs lists the names of the
// roots GetPayloadRefs returns, in order.
//
// A saved replay cursor reuses an operation's payload roots only under the
// same version, so when what GetPayloadRefs names changes, bump the version
// and record the new refs here together.
var payloadRefsGolden = map[string]struct {
	op      func(ref func(name string) *block.BlockRef) world.PayloadOperation
	refs    []string
	version uint32
}{
	unixfs_world.FsWriteAtOpTypeID: {
		op: func(ref func(string) *block.BlockRef) world.PayloadOperation {
			return &unixfs_world.FsWriteAtOp{ObjectKey: "fs", BlobRef: ref("blob")}
		},
		refs:    []string{"blob"},
		version: 1,
	},
	unixfs_world.FsMknodWithContentOpTypeID: {
		op: func(ref func(string) *block.BlockRef) world.PayloadOperation {
			return &unixfs_world.FsMknodWithContentOp{ObjectKey: "fs", BlobRef: ref("blob")}
		},
		refs:    []string{"blob"},
		version: 1,
	},
	git_world.GitInitOpTypeID: {
		op: func(ref func(string) *block.BlockRef) world.PayloadOperation {
			return &git_world.GitInitOp{
				ObjectKey: "repo",
				RepoRef: &bucket.ObjectRef{
					RootRef:          ref("repo-root"),
					TransformConfRef: ref("repo-transform-conf"),
				},
			}
		},
		refs:    []string{"repo-root"},
		version: 1,
	},
	manifest_world.StoreManifestOpTypeID: {
		op: func(ref func(string) *block.BlockRef) world.PayloadOperation {
			return &manifest_world.StoreManifestOp{
				ObjectKey: "manifest",
				ManifestRef: &bldr_manifest.ManifestRef{
					ManifestRef: &bucket.ObjectRef{
						RootRef:          ref("manifest-root"),
						TransformConfRef: ref("manifest-transform-conf"),
					},
				},
			}
		},
		refs:    []string{"manifest-root"},
		version: 1,
	},
}

// TestPayloadRefsGolden checks that every payload operation type in the static
// table has a golden fixture, and that what its GetPayloadRefs names changes
// only together with its PayloadRefsVersion.
func TestPayloadRefsGolden(t *testing.T) {
	ids := make([]string, 0, len(schema))
	for id := range schema {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var payloadIDs []string
	for _, id := range ids {
		if _, ok := schema[id]().(world.PayloadOperation); ok {
			payloadIDs = append(payloadIDs, id)
		}
	}
	for id := range payloadRefsGolden {
		if !slices.Contains(payloadIDs, id) {
			t.Errorf("%s: golden fixture for a type the table does not resolve as a payload operation", id)
		}
	}

	for _, id := range payloadIDs {
		golden, ok := payloadRefsGolden[id]
		if !ok {
			t.Errorf("%s: payload operation has no golden fixture in payloadRefsGolden", id)
			continue
		}

		names := make(map[string]string)
		ref := func(name string) *block.BlockRef {
			r, err := block.BuildBlockRef([]byte(id+"/"+name), nil)
			if err != nil {
				t.Fatal(err)
			}
			names[r.MarshalString()] = name
			return r
		}
		op := golden.op(ref)
		if got := op.TypeID(); got != id {
			t.Errorf("%s: fixture builds a %s operation", id, got)
			continue
		}
		var refs []string
		for _, r := range op.GetPayloadRefs() {
			name, ok := names[r.MarshalString()]
			if !ok {
				name = "unknown " + r.MarshalString()
			}
			refs = append(refs, name)
		}

		version := op.PayloadRefsVersion()
		switch {
		case version == 0:
			t.Errorf("%s: PayloadRefsVersion is zero", id)
		case version != golden.version:
			t.Errorf("%s: PayloadRefsVersion is %d, golden records %d with refs %v; record the refs it names now, %v, under the new version", id, version, golden.version, golden.refs, refs)
		case !slices.Equal(refs, golden.refs):
			t.Errorf("%s: GetPayloadRefs names %v, golden records %v under the same version %d; bump PayloadRefsVersion and record the new refs", id, refs, golden.refs, version)
		}
	}
}
