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
	fs := flag.NewFlagSet("changelog-notes", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var version string
	fs.StringVar(&version, "version", "", "release version to render")
	if err := fs.Parse(args); err != nil {
		return errors.Wrap(err, "parse flags")
	}
	if version == "" || fs.NArg() != 0 {
		return errors.New("usage: changelog-notes --version X.Y.Z")
	}

	cl, err := changelog.GetChangelog()
	if err != nil {
		return errors.Wrap(err, "load changelog")
	}
	notes, err := changelog.RenderReleaseMarkdown(cl, version)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(os.Stdout, notes); err != nil {
		return errors.Wrap(err, "write notes")
	}
	return nil
}
