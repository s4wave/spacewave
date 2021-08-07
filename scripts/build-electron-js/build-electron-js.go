package main

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
)

func execBuild() error {
	workDir, err := os.Getwd()
	if err != nil {
		return err
	}

	repoRoot := filepath.Join(workDir, "../../")
	repoRoot, err = filepath.Abs(repoRoot)
	if err != nil {
		return err
	}

	targetDir := path.Join(repoRoot, "target/electron")
	buildDir := path.Join(targetDir, "build")
	if _, err := os.Stat(buildDir); !os.IsNotExist(err) {
		err = os.RemoveAll(buildDir)
		if err != nil {
			return err
		}
	}

	err = os.MkdirAll(buildDir, 0755)
	if err != nil {
		return err
	}

	mainJsOut := path.Join(buildDir, "index.js")
	if _, err := os.Stat(mainJsOut); !os.IsNotExist(err) {
		err = os.Remove(mainJsOut)
		if err != nil {
			return err
		}
	}

	processErrs := func(res esbuild.BuildResult) error {
		if len(res.Errors) == 0 {
			return nil
		}
		for _, err := range res.Errors {
			os.Stderr.WriteString(err.Text + "\n")
		}
		return errors.Errorf("esbuild failed with %d errors", len(res.Errors))
	}

	// renderer bundle
	os.Stderr.WriteString("Generating main bundle...\n")
	res := esbuild.Build(esbuild.BuildOptions{
		Target:        esbuild.ES2020,
		AbsWorkingDir: repoRoot,
		Banner: map[string]string{
			"js": "// Built by build-electron-js",
		},
		Bundle: true,
		EntryPoints: []string{
			"src/electron/main/index.ts",
		},
		External: []string{"electron"},
		Format:   esbuild.FormatDefault,
		LogLevel: esbuild.LogLevelDebug,
		Outfile:  mainJsOut,
		Platform: esbuild.PlatformNode,
		Write:    true,
	})
	if err := processErrs(res); err != nil {
		return err
	}
	os.Stdout.WriteString("\n")

	// page bundle
	os.Stderr.WriteString("Generating renderer bundle...\n")
	sandboxOut := path.Join(buildDir, "sandbox")
	res = esbuild.Build(esbuild.BuildOptions{
		Target:        esbuild.ES2020,
		AbsWorkingDir: repoRoot,
		Banner: map[string]string{
			"js": "// Built by build-electron-js",
		},
		Bundle: true,
		Define: map[string]string{"BLDR_IS_ELECTRON": "true"},
		EntryPoints: []string{
			"src/sandbox/index.tsx",
		},
		External: []string{"electron"},
		Format:   esbuild.FormatDefault,
		Loader: map[string]esbuild.Loader{
			".woff":  esbuild.LoaderFile,
			".woff2": esbuild.LoaderFile,
		},
		LogLevel: esbuild.LogLevelDebug,
		Outdir:   sandboxOut,
		Platform: esbuild.PlatformBrowser,
		Write:    true,
	})
	if err := processErrs(res); err != nil {
		return err
	}
	os.Stdout.WriteString("\n")

	// service worker
	os.Stderr.WriteString("Generating service-worker bundle...\n")
	swOut := path.Join(buildDir, "service-worker.js")
	res = esbuild.Build(esbuild.BuildOptions{
		Target:        esbuild.ES2020,
		AbsWorkingDir: repoRoot,
		Banner: map[string]string{
			"js": "// Built by build-electron-js",
		},
		Bundle: true,
		EntryPoints: []string{
			"src/bldr/service-worker.ts",
		},
		External: []string{"electron"},
		Format:   esbuild.FormatDefault,
		LogLevel: esbuild.LogLevelDebug,
		Outfile:  swOut,
		Platform: esbuild.PlatformBrowser,
		Write:    true,
	})
	if err := processErrs(res); err != nil {
		return err
	}
	os.Stdout.WriteString("\n")

	srcDir := path.Join(repoRoot, "src")
	// electronRendererSrcDir := path.Join(srcDir, "electron/renderer")
	// indexHtmlPath := path.Join(electronRendererSrcDir, "index.html")
	indexHtmlPath := path.Join(srcDir, "index.html")
	ihtml, err := ioutil.ReadFile(indexHtmlPath)
	if err != nil {
		return err
	}

	electronMainSrcDir := path.Join(srcDir, "electron/main")
	// preload.js
	preloadJsPath := path.Join(electronMainSrcDir, "preload.js")
	pr, err := ioutil.ReadFile(preloadJsPath)
	if err != nil {
		return err
	}
	prOut := path.Join(buildDir, "preload.js")
	err = ioutil.WriteFile(prOut, pr, 0644)
	if err != nil {
		return err
	}

	// renderer index.html
	rendererHtmlOut := path.Join(buildDir, "index.html")
	err = ioutil.WriteFile(rendererHtmlOut, ihtml, 0644)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	err := execBuild()
	if err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
		return
	}
}
