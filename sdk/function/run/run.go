//go:build !goscript

// Package function_run runs PureFunctions in the QuickJS-NG WASI reactor
// under wazero.
package function_run

import (
	"bytes"
	"context"
	_ "embed"
	"strconv"
	"strings"

	"github.com/aperturerobotics/fastjson"
	quickjs "github.com/aperturerobotics/go-quickjs-wasi-reactor/wazero-quickjs"
	"github.com/pkg/errors"
	bldr_util_qjsbytecode "github.com/s4wave/spacewave/bldr/util/qjsbytecode"
	s4wave_function "github.com/s4wave/spacewave/sdk/function"
	"github.com/sirupsen/logrus"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// Harness is the module that runs a PureFunction. The browser runner
// evaluates the same text.
//
//go:embed harness.js
var Harness string

// InputPlaceholder is the token in Harness that a runner replaces with the
// input in protobuf JSON form.
const InputPlaceholder = "__PURE_FUNCTION_INPUT__"

// harnessName is the module name of the harness.
const harnessName = "/harness.js"

// bytecodeOwner is the bytecode cache directory of PureFunction modules.
const bytecodeOwner = "pure-function"

// stackBytes is the reactor's shadow stack, which its linear memory holds
// besides the heap.
const stackBytes = 8 << 20

// memorySlackBytes is the linear memory a run may use beyond its heap limit
// and stack, for the allocator and the reactor's static data.
const memorySlackBytes = 16 << 20

// ErrInstructionBudget is returned when a run passes its interrupt checks.
var ErrInstructionBudget = errors.New("the function ran past its instruction budget")

// ErrMemoryBudget is returned when a run passes its memory limit.
var ErrMemoryBudget = errors.New("the function ran past its memory budget")

// FunctionError is an error the function threw or returned.
type FunctionError struct {
	// Message is the error's string form.
	Message string
}

// Error returns the error message.
func (e *FunctionError) Error() string {
	return e.Message
}

// Runner runs PureFunctions, each in a fresh QuickJS instance, so one run
// cannot change the next and every run spends its budget from the start.
type Runner struct {
	// le is the logger.
	le *logrus.Entry
	// compiled caches the native code wazero compiles from the reactor.
	compiled wazero.CompilationCache
	// bytecode caches function modules, nil to compile them every run.
	bytecode *bldr_util_qjsbytecode.Cache
}

// NewRunner returns a runner. compiled is shared with other users of the
// reactor, and nil keeps a cache of its own; bytecode caches function modules
// and nil compiles them every run.
func NewRunner(le *logrus.Entry, compiled wazero.CompilationCache, bytecode *bldr_util_qjsbytecode.Cache) *Runner {
	if compiled == nil {
		compiled = wazero.NewCompilationCache()
	}
	return &Runner{le: le, compiled: compiled, bytecode: bytecode}
}

// JSONMarshaler is a message with a protobuf JSON form, such as a generated
// protobuf message.
type JSONMarshaler interface {
	MarshalJSON() ([]byte, error)
}

// JSONUnmarshaler decodes a message from its protobuf JSON form.
type JSONUnmarshaler interface {
	UnmarshalJSON([]byte) error
}

// Run runs fn on in and stores its result in out.
func (r *Runner) Run(ctx context.Context, fn *s4wave_function.PureFunction, in JSONMarshaler, out JSONUnmarshaler) error {
	// Run the function on the input's protobuf JSON form.
	input, err := in.MarshalJSON()
	if err != nil {
		return errors.Wrap(err, "encode input")
	}
	output, err := r.RunJSON(ctx, fn, input)
	if err != nil {
		return err
	}

	// Decode the function's output.
	if err := out.UnmarshalJSON(output); err != nil {
		return errors.Wrap(err, "decode output")
	}
	return nil
}

// RunJSON runs fn on the protobuf JSON input and returns its output as JSON.
func (r *Runner) RunJSON(ctx context.Context, fn *s4wave_function.PureFunction, input []byte) ([]byte, error) {
	// Check the function and its input.
	if err := fn.Validate(); err != nil {
		return nil, err
	}
	if fastjson.ValidateBytes(input) != nil {
		return nil, errors.New("the input is not JSON")
	}

	// Create a runtime whose linear memory holds the heap limit and stack.
	budget := fn.GetBudget()
	memLimit := budget.GetMemoryLimit()
	memPages := (uint64(memLimit) + stackBytes + memorySlackBytes) >> 16
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		WithCompilationCache(r.compiled).
		WithCloseOnContextDone(true).
		WithMemoryLimitPages(uint32(min(memPages, 65536))))
	defer rt.Close(ctx)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		return nil, err
	}
	mod, err := quickjs.CompileQuickJS(ctx, rt)
	if err != nil {
		return nil, err
	}

	// Start an instance with no files, environment or real clock.
	var stdout, stderr bytes.Buffer
	qjs, err := quickjs.NewQuickJSWithModule(ctx, rt, mod, wazero.NewModuleConfig().
		WithStdout(&stdout).
		WithStderr(&stderr))
	if err != nil {
		return nil, err
	}
	defer qjs.Close(ctx)
	if err := qjs.Init(ctx, []string{"qjs", "--memory-limit", memoryLimitArg(memLimit)}); err != nil {
		return nil, errors.Wrap(err, "init quickjs")
	}

	// Register the function's module.
	src := []byte(fn.GetModule())
	if r.bytecode != nil {
		_, _, err = r.bytecode.LoadModule(ctx, r.le, qjs, bytecodeOwner, s4wave_function.ModuleName, src)
	} else {
		_, err = qjs.CompileModule(ctx, fn.GetModule(), s4wave_function.ModuleName)
	}
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, &FunctionError{Message: msg}
		}
		return nil, errors.Wrap(err, "load function module")
	}

	// Run the harness under the instruction budget.
	if err := qjs.SetInterruptBudget(ctx, int64(budget.GetInterruptLimit())); err != nil {
		return nil, err
	}
	harness := strings.Replace(Harness, InputPlaceholder, string(input), 1)
	runErr := qjs.EvalWithFilename(ctx, harness, harnessName, true)
	if runErr == nil {
		runErr = qjs.RunLoop(ctx)
	}

	// Report a canceled run or a spent budget before the harness's result.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	left, err := qjs.InterruptBudget(ctx)
	if err != nil {
		return nil, err
	}
	if left == 0 {
		return nil, ErrInstructionBudget
	}
	return parseResult(stdout.Bytes(), stderr.String(), runErr)
}

// parseResult reads the harness's last stdout line.
func parseResult(stdout []byte, stderr string, runErr error) ([]byte, error) {
	// A run that printed nothing failed in the harness itself.
	line := bytes.TrimSpace(stdout)
	if i := bytes.LastIndexByte(line, '\n'); i >= 0 {
		line = line[i+1:]
	}
	if len(line) == 0 {
		if strings.Contains(stderr, "out of memory") {
			return nil, ErrMemoryBudget
		}
		if runErr == nil {
			runErr = errors.New("the function returned no result")
		}
		return nil, errors.Wrap(runErr, strings.TrimSpace(stderr))
	}

	// Decode the result line: output is the function's output in protobuf
	// JSON form, or error is the string form of what it threw, with
	// outOfMemory set when that was the heap limit.
	var p fastjson.Parser
	res, err := p.ParseBytes(line)
	if err != nil {
		return nil, errors.Wrap(err, "decode result")
	}
	if res.GetBool("outOfMemory") {
		return nil, ErrMemoryBudget
	}
	if msg := res.GetStringBytes("error"); len(msg) != 0 {
		return nil, &FunctionError{Message: string(msg)}
	}
	output := res.Get("output")
	if output == nil {
		return nil, errors.New("the function returned no output")
	}
	return output.MarshalTo(nil), nil
}

// memoryLimitArg formats a heap limit for the reactor's --memory-limit
// option, which reads a bare number as KiB.
func memoryLimitArg(limit uint32) string {
	return strconv.FormatUint(uint64(limit), 10) + "b"
}
