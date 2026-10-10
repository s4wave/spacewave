//go:build !js

package bldr_project_starlark

import (
	"context"
	"encoding/binary"
	"io"

	"github.com/pkg/errors"
)

// RunBounded is the body of the child that EvaluateFSBounded starts. It reads
// the memory budget and root file name from in, limits its own memory, and
// evaluates the project, reading every file through the parent. It writes the
// loaded files and the project config to out, or the evaluation error. It
// returns an error only when the child cannot do its work, and then writes
// nothing the parent trusts.
//
// RunBounded must not use the application bus: the child is a command of the
// executable, not an application instance.
func RunBounded(ctx context.Context, in io.Reader, out io.Writer) error {
	// Read the memory budget and the root file name.
	kind, start, err := readFrame(in)
	if err != nil {
		return errors.Wrap(err, "read the start frame")
	}
	if kind != frameStart || len(start) < 8 {
		return errors.Errorf("unexpected start frame %c", kind)
	}

	// Limit this process before it reads any project file.
	if err := limitMemory(binary.BigEndian.Uint64(start)); err != nil {
		return errors.Wrap(err, "limit memory")
	}

	// Evaluate the project, reading its files through the parent.
	readFile := func(name string) ([]byte, error) {
		// Ask the parent for the file.
		if err := writeFrame(out, frameRead, []byte(name)); err != nil {
			return nil, err
		}

		// Return the file bytes, or the error the parent met reading it.
		kind, body, err := readFrame(in)
		if err != nil {
			return nil, err
		}
		if kind == frameError {
			return nil, errors.New(string(body))
		}
		return body, nil
	}
	result, err := evaluate(ctx, readFile, string(start[8:]))
	if err != nil {
		return writeFrame(out, frameError, []byte(err.Error()))
	}

	// Send the loaded files, then the config that ends the evaluation.
	for _, name := range result.LoadedFiles {
		if err := writeFrame(out, frameLoaded, []byte(name)); err != nil {
			return err
		}
	}
	config, err := result.Config.MarshalVT()
	if err != nil {
		return err
	}
	return writeFrame(out, frameConfig, config)
}
