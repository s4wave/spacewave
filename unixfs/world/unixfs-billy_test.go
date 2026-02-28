//go:build unixfs_billy_test

package unixfs_world

import (
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/unixfs"
	billy_test "github.com/go-git/go-billy/v6/test"
	check "gopkg.in/check.v1"
)

// TestFsBilly runs the billyfs test suite.
// NOTE: not working properly
func TestFsBilly(t *testing.T) {
	tb, ufs := InitTestbed(t)
	ctx := tb.Context

	fsHandle, err := ufs.AddRootReference(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	ts := time.Now()
	fs := unixfs.NewBillyFilesystem(ctx, fsHandle, "", ts)
	suite := billy_test.NewFilesystemSuite(fs)
	_ = check.Suite(&suite)
	check.TestingT(t)
}
