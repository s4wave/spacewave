//go:build !js

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/s4wave/spacewave/core/debug/bundler"
	s4wave_debug "github.com/s4wave/spacewave/sdk/debug"
)

// isTypeScript returns true if the file extension indicates TypeScript.
func isTypeScript(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".ts" || ext == ".tsx"
}

// RunEval evaluates JavaScript code in the page context.
func (a *ClientArgs) RunEval(c *cli.Context) error {
	ctx := c.Context

	var code string
	var isModule bool

	if a.EvalFilePath != "" && isTypeScript(a.EvalFilePath) {
		// TypeScript: bundle with Vite first.
		bundled, err := a.bundleTypeScript(ctx, a.EvalFilePath)
		if err != nil {
			return err
		}
		code = bundled
		isModule = true
	} else if a.EvalFilePath != "" {
		data, err := os.ReadFile(a.EvalFilePath)
		if err != nil {
			return errors.Wrapf(err, "read %s", a.EvalFilePath)
		}
		code = string(data)
	} else if c.NArg() > 0 {
		code = c.Args().First()
	} else {
		return errors.New("provide code as argument or use --file")
	}

	svc, err := a.BuildClient()
	if err != nil {
		return err
	}
	resp, err := svc.EvalJS(ctx, &s4wave_debug.EvalJSRequest{
		Code:     code,
		IsModule: isModule,
	})
	if err != nil {
		return err
	}
	if resp.GetError() != "" {
		msg := resp.GetError()
		if LooksLikeSyntaxError(msg) {
			msg += "\nhint: for complex code, use --file: spacewave-debug eval --file script.js"
		}
		return errors.Errorf("eval: %s", msg)
	}
	result := resp.GetResult()
	if result != "" {
		os.Stdout.WriteString(result)
		os.Stdout.WriteString("\n")
	}
	return nil
}

// bundleTypeScript bundles a TypeScript file and returns the JS code.
func (a *ClientArgs) bundleTypeScript(ctx context.Context, filePath string) (string, error) {
	// Find project root by walking up from cwd looking for bldr.yaml.
	projectRoot, err := findProjectRoot()
	if err != nil {
		return "", err
	}

	stateRoot := filepath.Join(projectRoot, ".bldr")
	distPath := filepath.Join(stateRoot, "src")
	workingPath := filepath.Join(stateRoot, "debug", "eval")

	// Check dist sources exist.
	if _, err := os.Stat(distPath); os.IsNotExist(err) {
		return "", errors.New("bldr dist sources not found at .bldr/src/; run 'bldr setup' first")
	}

	le := logrus.NewEntry(logrus.StandardLogger()).WithField("component", "eval-bundler")
	b := bundler.NewBundler(le, distPath, projectRoot, workingPath)
	defer b.Close()

	// Parse webPkgs from bldr.yaml.
	webPkgs, err := bundler.ParseBldrWebPkgs(projectRoot)
	if err != nil {
		le.WithError(err).Warn("failed to parse webPkgs from bldr.yaml")
	}

	// Merge CLI --web-pkgs.
	webPkgs = bundler.MergeWebPkgStrings(webPkgs, a.WebPkgs.Value())
	b.SetWebPkgs(webPkgs)

	le.Debugf("bundling %s", filePath)
	code, err := b.Bundle(ctx, filePath)
	if err != nil {
		return "", errors.Wrap(err, "bundle typescript")
	}
	le.Debugf("bundled %d bytes", len(code))
	return code, nil
}

// EvalCode evaluates JavaScript code and prints the result to stdout.
func (a *ClientArgs) EvalCode(ctx context.Context, code string) error {
	svc, err := a.BuildClient()
	if err != nil {
		return err
	}
	resp, err := svc.EvalJS(ctx, &s4wave_debug.EvalJSRequest{Code: code})
	if err != nil {
		return err
	}
	if resp.GetError() != "" {
		return errors.Errorf("eval: %s", resp.GetError())
	}
	result := resp.GetResult()
	if result != "" {
		os.Stdout.WriteString(result)
		os.Stdout.WriteString("\n")
	}
	return nil
}

// RunInfo returns information about the current page.
func (a *ClientArgs) RunInfo(c *cli.Context) error {
	ctx := c.Context

	svc, err := a.BuildClient()
	if err != nil {
		return err
	}
	resp, err := svc.GetPageInfo(ctx, &s4wave_debug.GetPageInfoRequest{})
	if err != nil {
		return err
	}
	w := os.Stdout
	w.WriteString("URL:         " + resp.GetUrl() + "\n")
	w.WriteString("Title:       " + resp.GetTitle() + "\n")
	w.WriteString("WebView ID:  " + resp.GetWebViewId() + "\n")
	w.WriteString("Document ID: " + resp.GetDocumentId() + "\n")
	return nil
}
