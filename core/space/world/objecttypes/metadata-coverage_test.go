//go:build !tinygo && !sql_lite

package objecttypes

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
)

func TestKvObjectTypeMetadataCoverage(t *testing.T) {
	ctx := context.Background()
	for _, row := range []struct {
		typeID string
		lookup string
	}{
		{s4wave_kv_world.KvStoreTypeID, "LookupKvSetRootOp"},
	} {
		objType, err := LookupObjectType(ctx, row.typeID)
		if err != nil {
			t.Fatalf("LookupObjectType(%s): %v", row.typeID, err)
		}
		if objType == nil {
			t.Fatalf("LookupObjectType(%s) returned nil", row.typeID)
		}
		if got := objType.GetObjectTypeID(); got != row.typeID {
			t.Fatalf("LookupObjectType(%s) id = %s", row.typeID, got)
		}
		for _, optypesFile := range []string{
			"optypes.go",
			"optypes-tinygo.go",
			"optypes-goscript.go",
		} {
			if !fileCallsFunction(t, filepath.Join("..", "optypes", optypesFile), row.lookup) {
				t.Fatalf("%s does not register %s for %s", optypesFile, row.lookup, row.typeID)
			}
		}
	}
}

func fileCallsFunction(t *testing.T, path string, name string) bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", path, err)
	}
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		if found {
			return false
		}
		sel, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		found = sel.Sel.Name == name
		return true
	})
	return found
}
