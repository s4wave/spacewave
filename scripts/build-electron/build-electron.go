package main

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/pkg/errors"
)

// minify indicates components should be minified
const minify = false

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

	processErrs := func(res esbuild.BuildResult) error {
		if len(res.Errors) == 0 {
			return nil
		}
		for _, err := range res.Errors {
			os.Stderr.WriteString(err.Text + "\n")
		}
		return errors.Errorf("esbuild failed with %d errors", len(res.Errors))
	}

	// bruce
	banner := map[string]string{
		"js": "// Built by build-electron",
	}

	// main bundle
	os.Stderr.WriteString("Generating main bundle...\n")
	mainJsOut := path.Join(buildDir, "index.js")
	res := esbuild.Build(esbuild.BuildOptions{
		Target:            esbuild.ES2020,
		AbsWorkingDir:     repoRoot,
		Banner:            banner,
		Bundle:            true,
		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		EntryPoints: []string{
			"web/electron/main/index.ts",
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

	// preload bundle
	os.Stderr.WriteString("Generating preload bundle...\n")
	preloadJsOut := path.Join(buildDir, "preload.js")
	res = esbuild.Build(esbuild.BuildOptions{
		Target:            esbuild.ES2020,
		AbsWorkingDir:     repoRoot,
		Banner:            banner,
		Bundle:            true,
		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		EntryPoints: []string{
			"web/electron/main/preload.ts",
		},
		External: []string{"electron"},
		Format:   esbuild.FormatDefault,
		LogLevel: esbuild.LogLevelDebug,
		Outfile:  preloadJsOut,
		Platform: esbuild.PlatformNode,
		Write:    true,
	})
	if err := processErrs(res); err != nil {
		return err
	}
	os.Stdout.WriteString("\n")

	// renderer bundle
	os.Stderr.WriteString("Generating renderer bundle...\n")
	webEntrypointOut := path.Join(buildDir, "entrypoint")
	res = esbuild.Build(esbuild.BuildOptions{
		Target:            esbuild.ES2020,
		AbsWorkingDir:     repoRoot,
		Banner:            banner,
		Bundle:            true,
		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		Define:            map[string]string{"BLDR_IS_ELECTRON": "true"},
		EntryPoints: []string{
			"web/entrypoint/entrypoint.tsx",
		},
		External: []string{"electron"},
		Format:   esbuild.FormatDefault,
		/*
			Inject: []string{"web/electron/renderer/index.tsx"},
		*/
		Loader: map[string]esbuild.Loader{
			".woff":  esbuild.LoaderFile,
			".woff2": esbuild.LoaderFile,
		},
		LogLevel: esbuild.LogLevelDebug,
		Outdir:   webEntrypointOut,
		Platform: esbuild.PlatformBrowser,
		Write:    true,
	})
	if err := processErrs(res); err != nil {
		return err
	}
	os.Stdout.WriteString("\n")

	// service worker
	os.Stderr.WriteString("Generating service-worker bundle...\n")
	swOut := path.Join(buildDir, "sw.js")
	res = esbuild.Build(esbuild.BuildOptions{
		Target:        esbuild.ES2020,
		AbsWorkingDir: repoRoot,
		Banner: map[string]string{
			"js": "// Built by build-electron",
		},
		Bundle: true,
		EntryPoints: []string{
			"web/bldr/service-worker.ts",
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

	webSrcDir := path.Join(repoRoot, "web")
	indexHtmlPath := path.Join(webSrcDir, "index.html")
	ihtml, err := ioutil.ReadFile(indexHtmlPath)
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
