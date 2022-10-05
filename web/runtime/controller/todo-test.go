package web_runtime_controller

// Various hardcoded demos to be removed later.

import (
	"context"
	"time"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	billy_util "github.com/go-git/go-billy/v5/util"
	"github.com/sirupsen/logrus"

	// _ embeds data
	_ "embed"
)

// buildExampleFS builds a test unixfs for testing the service worker.
func buildExampleFS(ctx context.Context, le *logrus.Entry) (ufs *unixfs.FS, utb *world_testbed.Testbed, err error) {
	objKey := "example/test/1"
	ufs, utb, err = unixfs_world.BuildTestbed(ctx, objKey, true)
	if err != nil {
		return nil, nil, err
	}

	handle, err := ufs.AddRootReference(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer handle.Release()

	// create test fs (backed by a block graph + Hydra world)
	bfs := unixfs.NewBillyFilesystem(ctx, handle, "", time.Now())

	// create test image
	err = billy_util.WriteFile(bfs, "test.png", demoPng, 0755)
	if err != nil {
		return nil, nil, err
	}

	// create test script
	/*
		err = billy_util.WriteFile(bfs, "test.js", []byte(getTestComponentJS()+"\n"), 0755)
		if err != nil {
			return nil, nil, err
		}
	*/

	// done
	return ufs, utb, nil
}

//go:embed test.png
var demoPng []byte
