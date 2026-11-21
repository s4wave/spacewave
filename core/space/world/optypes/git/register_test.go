//go:build !tinygo && !goscript

package optypes_git

import (
	"context"
	"testing"

	"github.com/aperturerobotics/common/typeid"

	s4wave_git "github.com/s4wave/spacewave/core/git"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
)

// TestRegistersGitTypes checks that importing the package makes the Space
// schema resolve every Git operation and block type.
func TestRegistersGitTypes(t *testing.T) {
	ctx := context.Background()
	lookupOp := optypes.BuildSpaceLookupOp(nil, nil, "space/local/test")

	// Resolve every declared Git type as an operation or a block type.
	for _, list := range [][]typeid.Type{git_world.GitTypes, s4wave_git.GitTypes, git_block.GitTypes} {
		for _, typ := range list {
			if _, isOp := typ.New().(world.Operation); isOp {
				op, err := lookupOp(ctx, typ.ID)
				if err != nil || op == nil {
					t.Fatalf("lookup op %s = %v, %v", typ.ID, op, err)
				}
				continue
			}
			blockType, err := optypes.LookupBlockType(ctx, typ.ID)
			if err != nil || blockType == nil {
				t.Fatalf("lookup block type %s = %v, %v", typ.ID, blockType, err)
			}
		}
	}
}
