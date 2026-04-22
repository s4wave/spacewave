//go:build !js

package web_pkg_vite

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	bldr_vite "github.com/aperturerobotics/bldr/web/bundler/vite"
	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	srpc "github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

type fakeViteBundlerClient struct {
	resp *bldr_vite.BuildWebPkgResponse
}

func (f *fakeViteBundlerClient) SRPCClient() srpc.Client { return nil }

func (f *fakeViteBundlerClient) Build(context.Context, *bldr_vite.BuildRequest) (*bldr_vite.BuildResponse, error) {
	return nil, nil
}

func (f *fakeViteBundlerClient) BuildWebPkg(context.Context, *bldr_vite.BuildWebPkgRequest) (*bldr_vite.BuildWebPkgResponse, error) {
	return f.resp, nil
}

func TestBuildWebPkgsViteKeepsRelativeSourceFiles(t *testing.T) {
	codeRootPath := t.TempDir()
	pkgRoot := filepath.Join(codeRootPath, "node_modules", "@aptre", "it-ws")
	outDir := filepath.Join(t.TempDir(), "out")

	client := &fakeViteBundlerClient{
		resp: &bldr_vite.BuildWebPkgResponse{
			Success: true,
			SourceFiles: []string{
				"node_modules/@aptre/it-ws/dist/src/duplex.js",
				filepath.Join(pkgRoot, "dist/src/socket.js"),
			},
		},
	}

	_, srcFiles, _, err := BuildWebPkgsVite(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		codeRootPath,
		[]*web_pkg.WebPkgRef{{
			WebPkgId:   "@aptre/it-ws",
			WebPkgRoot: pkgRoot,
		}},
		outDir,
		"/b/pkg/",
		false,
		client,
		filepath.Join(t.TempDir(), "cache"),
	)
	if err != nil {
		t.Fatal(err)
	}

	slices.Sort(srcFiles)
	expected := []string{
		"node_modules/@aptre/it-ws/dist/src/duplex.js",
		"node_modules/@aptre/it-ws/dist/src/socket.js",
	}
	if !slices.Equal(srcFiles, expected) {
		t.Fatalf("unexpected source files: got %v want %v", srcFiles, expected)
	}
}
