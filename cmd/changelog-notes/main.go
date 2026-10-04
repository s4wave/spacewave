package main

import (
	"flag"
	"io"
	"os"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/changelog"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = io.WriteString(os.Stderr, err.Error()+"\n")
		os.Exit(1)
	}
}

func run(args []string) error {
	// Configure the release-note command and its flag output.
	fs := flag.NewFlagSet("changelog-notes", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	// Parse the required release version and reject extra arguments.
	var version string
	fs.StringVar(&version, "version", "", "release version to render")
	if err := fs.Parse(args); err != nil {
		return errors.Wrap(err, "parse flags")
	}
	if version == "" || fs.NArg() != 0 {
		return errors.New("usage: changelog-notes --version X.Y.Z")
	}

	// Load the changelog records used for the requested release.
	cl, err := changelog.GetChangelog()
	if err != nil {
		return errors.Wrap(err, "load changelog")
	}

	// Render the selected release notes as Markdown.
	notes, err := changelog.RenderReleaseMarkdown(cl, version)
	if err != nil {
		return err
	}

	// Write the rendered release notes to standard output.
	if _, err := io.WriteString(os.Stdout, notes); err != nil {
		return errors.Wrap(err, "write notes")
	}
	return nil
}
