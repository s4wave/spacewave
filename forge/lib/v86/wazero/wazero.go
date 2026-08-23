package v86_wazero

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// ImportReport records the host surface required by the v86 wasm artifact.
type ImportReport struct {
	Functions []string
	Memories  []string
	Tables    []string
	Exports   []string
}

// CompileImportReport compiles a wasm artifact and reports the host surface
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

// TryInstantiateEmscriptenV86 instantiates the artifact with default options
func TryInstantiateEmscriptenV86(ctx context.Context, wasmPath string) (*ImportReport, error) {
	host, err := InstantiateHostRuntime(ctx, wasmPath, HostRuntimeOptions{})
	if err != nil {
		return nil, err
	}
	defer host.Close(ctx)
	return host.Report, nil
}

// importReport summarizes the imports of a compiled v86 module and verifies
func importReport(compiled wazero.CompiledModule, wasmBytes []byte) (*ImportReport, error) {
	report := &ImportReport{}
	for _, fn := range compiled.ImportedFunctions() {
		moduleName, name, _ := fn.Import()
		report.Functions = append(report.Functions, formatImport(moduleName, name, fn.ParamTypes(), fn.ResultTypes()))
	}
	for _, mem := range compiled.ImportedMemories() {
		moduleName, name, _ := mem.Import()
		maxText := "unbounded"
		if max, hasMax := mem.Max(); hasMax {
			maxText = strconv.FormatUint(uint64(max), 10)
		}
		report.Memories = append(report.Memories,
			moduleName+"."+name+" min="+strconv.FormatUint(uint64(mem.Min()), 10)+" max="+maxText)
	}
	imports, err := parseWasmExternImports(wasmBytes)
	if err != nil {
		return nil, errors.Wrap(err, "parse v86 wasm table imports")
	}
	for _, table := range imports.Tables {
		report.Tables = append(report.Tables, table.String())
	}
	for name := range compiled.ExportedFunctions() {
		report.Exports = append(report.Exports, name)
	}
	return report, nil
}

// formatImport renders one imported function signature for the report.
func formatImport(moduleName, name string, params, results []api.ValueType) string {
	return moduleName + "." + name + " " + valueTypes(params) + " -> " + valueTypes(results)
}

// valueTypes renders a list of wasm value types as their short names.
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

// wasmExternImports collects the memory and table externs the v86 module
type wasmExternImports struct {
	Memories []wasmMemoryImport
	Tables   []wasmTableImport
}

// wasmMemoryImport describes one imported linear memory.
type wasmMemoryImport struct {
	Module string
	Name   string
	Limits wasmLimits
}

// String renders the memory import as module.name limits.
func (i wasmMemoryImport) String() string {
	return i.Module + "." + i.Name + " " + i.Limits.String()
}

// wasmTableImport describes one imported table.
type wasmTableImport struct {
	Module string
	Name   string
	Limits wasmLimits
}

// String renders the table import as module.name limits.
func (i wasmTableImport) String() string {
	return i.Module + "." + i.Name + " " + i.Limits.String()
}

// wasmLimits holds the min and optional max size of an imported memory or
type wasmLimits struct {
	Min uint32
	Max *uint32
}

// String renders the limits as min=N max=M or min=N max=unbounded.
func (l wasmLimits) String() string {
	maxText := "unbounded"
	if l.Max != nil {
		maxText = strconv.FormatUint(uint64(*l.Max), 10)
	}
	return "min=" + strconv.FormatUint(uint64(l.Min), 10) + " max=" + maxText
}

// parseWasmExternImports walks the wasm binary's import section and returns
func parseWasmExternImports(wasmBytes []byte) (*wasmExternImports, error) {
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
		return parseImportedExterns(section)
	}
	return &wasmExternImports{}, nil
}

// parseImportedExterns decodes one import-section payload into the externs
func parseImportedExterns(section []byte) (*wasmExternImports, error) {
	r := wasmReader{data: section}
	count, err := r.u32()
	if err != nil {
		return nil, err
	}
	imports := &wasmExternImports{}
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
			limits, err := r.limits()
			if err != nil {
				return nil, err
			}
			imports.Tables = append(imports.Tables, wasmTableImport{
				Module: moduleName,
				Name:   importName,
				Limits: limits,
			})
		case 2:
			limits, err := r.limits()
			if err != nil {
				return nil, err
			}
			imports.Memories = append(imports.Memories, wasmMemoryImport{
				Module: moduleName,
				Name:   importName,
				Limits: limits,
			})
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
	return imports, nil
}

// wasmReader is a little-endian LEB128 reader over an in-memory wasm image.
type wasmReader struct {
	data []byte
	pos  int
}

// remaining returns how many unread bytes are left.
func (r *wasmReader) remaining() int {
	return len(r.data) - r.pos
}

// byte consumes one byte.
func (r *wasmReader) byte() (byte, error) {
	if r.remaining() < 1 {
		return 0, errors.New("unexpected end of wasm")
	}
	v := r.data[r.pos]
	r.pos++
	return v, nil
}

// bytes consumes n raw bytes.
func (r *wasmReader) bytes(n int) ([]byte, error) {
	if n < 0 || r.remaining() < n {
		return nil, errors.New("unexpected end of wasm")
	}
	v := r.data[r.pos : r.pos+n]
	r.pos += n
	return v, nil
}

// name consumes a length-prefixed string.
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

// limits consumes a flags-prefixed min/max pair.
func (r *wasmReader) limits() (wasmLimits, error) {
	flags, err := r.u32()
	if err != nil {
		return wasmLimits{}, err
	}
	min, err := r.u32()
	if err != nil {
		return wasmLimits{}, err
	}
	if flags&1 == 0 {
		return wasmLimits{Min: min}, nil
	}
	max, err := r.u32()
	if err != nil {
		return wasmLimits{}, err
	}
	return wasmLimits{Min: min, Max: &max}, nil
}

// u32 consumes one unsigned LEB128 value.
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
