package v86_wazero

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/emscripten"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// ImportReport records the host surface required by the v86 wasm artifact.
type ImportReport struct {
	Functions []string
	Memories  []string
	Tables    []string
	Exports   []string
}

func CompileImportReport(ctx context.Context, wasmPath string) (*ImportReport, error) {
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, errors.Wrap(err, "read v86 wasm")
	}
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	compiled, err := r.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, errors.Wrap(err, "compile v86 wasm with wazero")
	}
	defer compiled.Close(ctx)

	return importReport(compiled, wasmBytes)
}

func TryInstantiateEmscriptenV86(ctx context.Context, wasmPath string) (*ImportReport, error) {
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, errors.Wrap(err, "read v86 wasm")
	}
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	compiled, err := r.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, errors.Wrap(err, "compile v86 wasm with wazero")
	}
	defer compiled.Close(ctx)

	report, err := importReport(compiled, wasmBytes)
	if err != nil {
		return nil, err
	}
	if len(report.Memories) != 0 || len(report.Tables) != 0 {
		return report, errors.Errorf(
			"v86 wasm needs a Go env module before it can instantiate in wazero: memories=[%s] tables=[%s]",
			strings.Join(report.Memories, ", "),
			strings.Join(report.Tables, ", "),
		)
	}
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	if _, err := emscripten.InstantiateForModule(ctx, r, compiled); err != nil {
		return report, errors.Wrap(err, "instantiate emscripten imports")
	}
	mod, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("v86"))
	if err != nil {
		return report, errors.Wrap(err, "instantiate v86 wasm module")
	}
	_ = mod
	return report, nil
}

func importReport(compiled wazero.CompiledModule, wasmBytes []byte) (*ImportReport, error) {
	report := &ImportReport{}
	for _, fn := range compiled.ImportedFunctions() {
		moduleName, name, _ := fn.Import()
		report.Functions = append(report.Functions, formatImport(moduleName, name, fn.ParamTypes(), fn.ResultTypes()))
	}
	for _, mem := range compiled.ImportedMemories() {
		moduleName, name, _ := mem.Import()
		max, hasMax := mem.Max()
		maxText := "unbounded"
		if hasMax {
			maxText = fmt.Sprint(max)
		}
		report.Memories = append(report.Memories, fmt.Sprintf("%s.%s min=%d max=%s", moduleName, name, mem.Min(), maxText))
	}
	tables, err := importedTablesFromWasm(wasmBytes)
	if err != nil {
		return nil, errors.Wrap(err, "parse v86 wasm table imports")
	}
	report.Tables = tables
	for name := range compiled.ExportedFunctions() {
		report.Exports = append(report.Exports, name)
	}
	return report, nil
}

func formatImport(moduleName, name string, params, results []api.ValueType) string {
	return fmt.Sprintf("%s.%s %s -> %s", moduleName, name, valueTypes(params), valueTypes(results))
}

func valueTypes(types []api.ValueType) string {
	if len(types) == 0 {
		return "()"
	}
	parts := make([]string, len(types))
	for i, typ := range types {
		parts[i] = api.ValueTypeName(typ)
	}
	return "(" + strings.Join(parts, ",") + ")"
}

func importedTablesFromWasm(wasmBytes []byte) ([]string, error) {
	r := wasmReader{data: wasmBytes}
	if len(wasmBytes) < 8 || string(wasmBytes[:4]) != "\x00asm" {
		return nil, errors.New("invalid wasm magic")
	}
	r.pos = 8
	for r.remaining() > 0 {
		sectionID, err := r.byte()
		if err != nil {
			return nil, err
		}
		size, err := r.u32()
		if err != nil {
			return nil, err
		}
		section, err := r.bytes(int(size))
		if err != nil {
			return nil, err
		}
		if sectionID != 2 {
			continue
		}
		return parseImportedTables(section)
	}
	return nil, nil
}

func parseImportedTables(section []byte) ([]string, error) {
	r := wasmReader{data: section}
	count, err := r.u32()
	if err != nil {
		return nil, err
	}
	var tables []string
	for range count {
		moduleName, err := r.name()
		if err != nil {
			return nil, err
		}
		importName, err := r.name()
		if err != nil {
			return nil, err
		}
		kind, err := r.byte()
		if err != nil {
			return nil, err
		}
		switch kind {
		case 0:
			if _, err := r.u32(); err != nil {
				return nil, err
			}
		case 1:
			if _, err := r.byte(); err != nil {
				return nil, err
			}
			min, maxText, err := r.limits()
			if err != nil {
				return nil, err
			}
			tables = append(tables, fmt.Sprintf("%s.%s min=%d max=%s", moduleName, importName, min, maxText))
		case 2:
			if _, _, err := r.limits(); err != nil {
				return nil, err
			}
		case 3:
			if _, err := r.byte(); err != nil {
				return nil, err
			}
			if _, err := r.byte(); err != nil {
				return nil, err
			}
		default:
			return nil, errors.Errorf("unknown wasm import kind %d", kind)
		}
	}
	return tables, nil
}

type wasmReader struct {
	data []byte
	pos  int
}

func (r *wasmReader) remaining() int {
	return len(r.data) - r.pos
}

func (r *wasmReader) byte() (byte, error) {
	if r.remaining() < 1 {
		return 0, errors.New("unexpected end of wasm")
	}
	v := r.data[r.pos]
	r.pos++
	return v, nil
}

func (r *wasmReader) bytes(n int) ([]byte, error) {
	if n < 0 || r.remaining() < n {
		return nil, errors.New("unexpected end of wasm")
	}
	v := r.data[r.pos : r.pos+n]
	r.pos += n
	return v, nil
}

func (r *wasmReader) name() (string, error) {
	n, err := r.u32()
	if err != nil {
		return "", err
	}
	b, err := r.bytes(int(n))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (r *wasmReader) limits() (uint32, string, error) {
	flags, err := r.u32()
	if err != nil {
		return 0, "", err
	}
	min, err := r.u32()
	if err != nil {
		return 0, "", err
	}
	if flags&1 == 0 {
		return min, "unbounded", nil
	}
	max, err := r.u32()
	if err != nil {
		return 0, "", err
	}
	return min, fmt.Sprint(max), nil
}

func (r *wasmReader) u32() (uint32, error) {
	var result uint32
	for shift := 0; shift < 35; shift += 7 {
		b, err := r.byte()
		if err != nil {
			return 0, err
		}
		result |= uint32(b&0x7f) << shift
		if b&0x80 == 0 {
			return result, nil
		}
	}
	return 0, errors.New("invalid wasm varuint32")
}
