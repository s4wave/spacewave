//go:build !js

package web_pkg_vite

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	srpc "github.com/aperturerobotics/starpc/srpc"
	bldr_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	web_pkg_external "github.com/s4wave/spacewave/bldr/web/pkg/external"
	"github.com/sirupsen/logrus"
)

type fakeViteBundlerClient struct {
	resp     *bldr_vite.BuildWebPkgResponse
	requests []*bldr_vite.BuildWebPkgRequest
}

func (f *fakeViteBundlerClient) SRPCClient() srpc.Client { return nil }

func (f *fakeViteBundlerClient) Build(context.Context, *bldr_vite.BuildRequest) (*bldr_vite.BuildResponse, error) {
	return nil, nil
}

func (f *fakeViteBundlerClient) BuildWebPkg(_ context.Context, req *bldr_vite.BuildWebPkgRequest) (*bldr_vite.BuildWebPkgResponse, error) {
	f.requests = append(f.requests, req)
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
		false,
		true,
		nil,
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

func TestBuildWebPkgsViteKeepsCjsWrappersOutsideOutDir(t *testing.T) {
	codeRootPath := t.TempDir()
	pkgRoot := filepath.Join(codeRootPath, "node_modules", "cjs-pkg")
	if err := os.MkdirAll(pkgRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(pkgRoot, "index.cjs"),
		[]byte("exports.alpha = 1;\nexports.beta = 2;\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(t.TempDir(), "out")
	client := &fakeViteBundlerClient{
		resp: &bldr_vite.BuildWebPkgResponse{Success: true},
	}

	_, _, _, err := BuildWebPkgsVite(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		codeRootPath,
		[]*web_pkg.WebPkgRef{{
			WebPkgId:   "cjs-pkg",
			WebPkgRoot: pkgRoot,
			Imports:    []string{"index.cjs"},
		}},
		outDir,
		"/b/pkg/",
		false,
		false,
		true,
		nil,
		client,
		filepath.Join(t.TempDir(), "cache"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("unexpected request count: got %d want 1", len(client.requests))
	}

	req := client.requests[0]
	if len(req.GetImports()) != 1 {
		t.Fatalf("unexpected imports: %v", req.GetImports())
	}
	wrapperPath := req.GetImports()[0]
	if !filepath.IsAbs(wrapperPath) {
		t.Fatalf("wrapper path is not absolute: %s", wrapperPath)
	}
	outPrefix := req.GetOutDir() + string(os.PathSeparator)
	if strings.HasPrefix(wrapperPath, outPrefix) {
		t.Fatalf("wrapper path %s is inside outDir %s", wrapperPath, req.GetOutDir())
	}
	expectedPrefix := filepath.Join(outDir, ".cjs-wrappers") + string(os.PathSeparator)
	if !strings.HasPrefix(wrapperPath, expectedPrefix) {
		t.Fatalf("wrapper path %s does not use wrapper dir prefix %s", wrapperPath, expectedPrefix)
	}
	if strings.Contains(filepath.ToSlash(wrapperPath), "/cjs-pkg/index.mjs") {
		t.Fatalf("wrapper path %s includes package id in entrypoint name", wrapperPath)
	}
}

func TestBuildWebPkgsVitePropagatesJavaScriptPolicy(t *testing.T) {
	codeRootPath := t.TempDir()
	pkgRoot := filepath.Join(codeRootPath, "node_modules", "policy-pkg")
	if err := os.MkdirAll(pkgRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	client := &fakeViteBundlerClient{
		resp: &bldr_vite.BuildWebPkgResponse{Success: true},
	}

	_, _, _, err := BuildWebPkgsVite(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		codeRootPath,
		[]*web_pkg.WebPkgRef{{
			WebPkgId:   "policy-pkg",
			WebPkgRoot: pkgRoot,
			Imports:    []string{"index.js"},
		}},
		filepath.Join(t.TempDir(), "out"),
		"/b/pkg/",
		true,
		false,
		true,
		[]string{"react", "policy-pkg"},
		client,
		filepath.Join(t.TempDir(), "cache"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("unexpected request count: got %d want 1", len(client.requests))
	}
	req := client.requests[0]
	if !req.GetIsRelease() {
		t.Fatal("request did not preserve release mode")
	}
	if req.GetJsMinification() {
		t.Fatal("request enabled JavaScript minification")
	}
	if !req.GetJsSourcemaps() {
		t.Fatal("request did not enable JavaScript sourcemaps")
	}
	if !slices.Equal(req.GetExternalPkgs(), []string{"react"}) {
		t.Fatalf("external packages = %v, want [react]", req.GetExternalPkgs())
	}
}

func TestBuildWebPkgsViteExternalizesBldrSingletonPeers(t *testing.T) {
	codeRootPath := t.TempDir()
	webPkgRefs := []*web_pkg.WebPkgRef{{
		WebPkgId:   "react",
		WebPkgRoot: filepath.Join(codeRootPath, "node_modules", "react"),
	}, {
		WebPkgId:   "react-dom",
		WebPkgRoot: filepath.Join(codeRootPath, "node_modules", "react-dom"),
	}, {
		WebPkgId:   "@aptre/bldr-react",
		WebPkgRoot: filepath.Join(codeRootPath, "node_modules", "@aptre", "bldr-react"),
	}}
	client := &fakeViteBundlerClient{
		resp: &bldr_vite.BuildWebPkgResponse{Success: true},
	}

	_, _, _, err := BuildWebPkgsVite(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		codeRootPath,
		webPkgRefs,
		filepath.Join(t.TempDir(), "out"),
		"/b/pkg/",
		false,
		false,
		true,
		web_pkg_external.BldrExternal,
		client,
		filepath.Join(t.TempDir(), "cache"),
	)
	if err != nil {
		t.Fatal(err)
	}
	requests := make(map[string]*bldr_vite.BuildWebPkgRequest, len(client.requests))
	for _, req := range client.requests {
		requests[req.GetPkgId()] = req
	}
	if len(requests) != len(webPkgRefs) {
		t.Fatalf("unexpected request count: got %d want %d", len(requests), len(webPkgRefs))
	}

	react := requests["react"]
	if react == nil {
		t.Fatal("missing react build request")
	}
	assertExternalPkgs(t, react, []string{"react-dom", "@aptre/bldr", "@aptre/bldr-react"}, []string{"react"})

	reactDOM := requests["react-dom"]
	if reactDOM == nil {
		t.Fatal("missing react-dom build request")
	}
	assertExternalPkgs(t, reactDOM, []string{"react"}, []string{"react-dom"})

	bldrReact := requests["@aptre/bldr-react"]
	if bldrReact == nil {
		t.Fatal("missing @aptre/bldr-react build request")
	}
	assertExternalPkgs(t, bldrReact, []string{"react", "react-dom", "@aptre/bldr"}, []string{"@aptre/bldr-react"})
}

func assertExternalPkgs(t *testing.T, req *bldr_vite.BuildWebPkgRequest, wantPresent, wantAbsent []string) {
	t.Helper()
	externalPkgs := req.GetExternalPkgs()
	for _, pkgID := range wantPresent {
		if !slices.Contains(externalPkgs, pkgID) {
			t.Fatalf("%s external packages = %v, missing %s", req.GetPkgId(), externalPkgs, pkgID)
		}
	}
	for _, pkgID := range wantAbsent {
		if slices.Contains(externalPkgs, pkgID) {
			t.Fatalf("%s external packages = %v, unexpectedly contains %s", req.GetPkgId(), externalPkgs, pkgID)
		}
	}
}
