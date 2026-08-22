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
	host, err := InstantiateHostRuntime(ctx, wasmPath, HostRuntimeOptions{})
	if err != nil {
		return nil, err
	}
	defer host.Close(ctx)
	return host.Report, nil
}

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

func formatImport(moduleName, name string, params, results []api.ValueType) string {
	return moduleName + "." + name + " " + valueTypes(params) + " -> " + valueTypes(results)
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

type wasmExternImports struct {
	Memories []wasmMemoryImport
	Tables   []wasmTableImport
}

type wasmMemoryImport struct {
	Module string
	Name   string
	Limits wasmLimits
}

func (i wasmMemoryImport) String() string {
	return i.Module + "." + i.Name + " " + i.Limits.String()
}

type wasmTableImport struct {
	Module string
	Name   string
	Limits wasmLimits
}

func (i wasmTableImport) String() string {
	return i.Module + "." + i.Name + " " + i.Limits.String()
}

type wasmLimits struct {
	Min uint32
	Max *uint32
}

func (l wasmLimits) String() string {
	maxText := "unbounded"
	if l.Max != nil {
		maxText = strconv.FormatUint(uint64(*l.Max), 10)
	}
	return "min=" + strconv.FormatUint(uint64(l.Min), 10) + " max=" + maxText
}

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
