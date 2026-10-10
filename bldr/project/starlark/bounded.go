//go:build !js

package bldr_project_starlark

import (
	"context"
	"encoding/binary"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
)

// BoundedCommand is the argument that makes the executable run RunBounded. The
// spacewave binary registers it as a hidden command.
const BoundedCommand = "eval-bldr-star"

// boundedChildEnv is set in the environment of the child. A binary that does
// not know BoundedCommand would run itself again, so it refuses to start a
// child while it is one.
const boundedChildEnv = "BLDR_STAR_EVAL_CHILD"

// childStderrLimit is the number of bytes of the child's stderr kept to explain
// its failure.
const childStderrLimit = 4096

// errChildExited marks a failure to exchange frames with the child, which
// means it exited or misbehaved.
var errChildExited = errors.New("evaluation process stopped")

// EvaluateFSBounded evaluates the .star file name within fsys, as EvaluateFS
// does, for a project that is not trusted. It runs the evaluation in a child of
// the running executable that cannot use more than memoryBytes of memory above
// its start, and kills the child when ctx ends, so a hostile project is refused
// and this process survives. The child reads project files through this
// process, each at most MaxFileSize.
func EvaluateFSBounded(ctx context.Context, fsys fs.FS, name string, memoryBytes uint64) (*Result, error) {
	// Refuse to start a child from a child.
	if os.Getenv(boundedChildEnv) != "" {
		return nil, errors.New("a bounded evaluation cannot start another")
	}

	// Run the executable as the child command, killed when ctx ends.
	exe, err := os.Executable()
	if err != nil {
		return nil, errors.Wrap(err, "find the executable")
	}
	cmd := exec.CommandContext(ctx, exe, BoundedCommand)
	cmd.Env = append(os.Environ(), boundedChildEnv+"=1")
	stderr := &headWriter{limit: childStderrLimit}
	cmd.Stderr = stderr

	// Talk to the child over its stdin and stdout.
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	// Start the child with the memory limit in force.
	release, err := startBounded(cmd, memoryBytes)
	if err != nil {
		return nil, errors.Wrap(err, "start the evaluation process")
	}
	defer release()

	// Serve the child's reads until it sends the config, then stop it.
	result, err := serveBounded(fsys, name, memoryBytes, in, out)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if err == nil {
		return result, nil
	}

	// Report why the child stopped when it did not fail by itself.
	if !errors.Is(err, errChildExited) {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	reason := strings.TrimSpace(string(stderr.buf))
	if strings.Contains(reason, "out of memory") {
		return nil, errors.Errorf("evaluation used more than %d MiB of memory", memoryBytes>>20)
	}
	if reason, _, _ = strings.Cut(reason, "\n"); reason != "" {
		return nil, errors.Errorf("evaluation process stopped: %s", reason)
	}
	return nil, err
}

// serveBounded starts the child's evaluation of name over in and out, answers
// its file reads from fsys, and returns the result it sends.
func serveBounded(fsys fs.FS, name string, memoryBytes uint64, in io.Writer, out io.Reader) (*Result, error) {
	// Send the memory budget and the root file name.
	start := binary.BigEndian.AppendUint64(nil, memoryBytes)
	if err := writeFrame(in, frameStart, append(start, name...)); err != nil {
		return nil, errors.Wrap(errChildExited, err.Error())
	}

	// Answer each frame until the child sends the config or an error.
	result := &Result{}
	for {
		kind, body, err := readFrame(out)
		if err != nil {
			return nil, errors.Wrap(errChildExited, err.Error())
		}
		switch kind {
		case frameRead:
			if err := replyRead(fsys, string(body), in); err != nil {
				return nil, errors.Wrap(errChildExited, err.Error())
			}
		case frameLoaded:
			result.LoadedFiles = append(result.LoadedFiles, string(body))
		case frameConfig:
			result.Config = &bldr_project.ProjectConfig{}
			if err := result.Config.UnmarshalVT(body); err != nil {
				return nil, err
			}
			return result, nil
		case frameError:
			return nil, errors.New(string(body))
		default:
			return nil, errors.Wrapf(errChildExited, "unexpected frame %c", kind)
		}
	}
}

// replyRead sends the child the contents of name in fsys, or the error that
// stopped the read.
func replyRead(fsys fs.FS, name string, in io.Writer) error {
	data, err := ReadFile(fsys, name)
	if err != nil {
		return writeFrame(in, frameError, []byte(err.Error()))
	}
	return writeFrame(in, frameData, data)
}
