package v86_wazero

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// buildSharedModule emits the wasm module that re-exports the imported
func buildSharedModule(imports *wasmExternImports, opts HostRuntimeOptions) ([]byte, error) {
	if len(imports.Memories) > 1 {
		return nil, errors.Errorf("v86 host runtime supports one imported memory, got %d", len(imports.Memories))
	}
	if len(imports.Tables) > 1 {
		return nil, errors.Errorf("v86 host runtime supports one imported table, got %d", len(imports.Tables))
	}
	memoryMin := uint32(0)
	for _, memory := range imports.Memories {
		memoryMin = max(memoryMin, memory.Limits.Min)
	}
	if len(imports.Memories) != 0 {
		memoryMin = max(memoryMin, defaultInitialMemoryPages)
	}
	if opts.InitialMemoryPages != 0 {
		memoryMin = max(memoryMin, opts.InitialMemoryPages)
	}
	tableMin := uint32(0)
	for _, table := range imports.Tables {
		tableMin = max(tableMin, table.Limits.Min)
	}

	var sections [][]byte
	if len(imports.Tables) != 0 {
		tableSection := appendU32(nil, 1)
		tableSection = append(tableSection, 0x70)
		tableSection = appendLimits(tableSection, wasmLimits{Min: tableMin})
		sections = append(sections, wasmSection(4, tableSection))
	}
	if len(imports.Memories) != 0 {
		memorySection := appendU32(nil, 1)
		memorySection = appendLimits(memorySection, wasmLimits{Min: memoryMin})
		sections = append(sections, wasmSection(5, memorySection))
	}
	exportSection := appendExternExports(appendU32(nil, uint32(len(imports.Memories)+len(imports.Tables))), imports)
	sections = append(sections, wasmSection(7, exportSection))
	return wasmModule(sections...), nil
}

// buildEnvModule emits the goenv module: it imports the shared memory and
func buildEnvModule(compiled wazero.CompiledModule, imports *wasmExternImports) ([]byte, error) {
	type typeEntry struct {
		params  []api.ValueType
		results []api.ValueType
	}
	type functionImport struct {
		name      string
		typeIndex uint32
	}
	typeMap := map[string]uint32{}
	var types []typeEntry
	var funcs []functionImport
	for _, fn := range compiled.ImportedFunctions() {
		moduleName, name, _ := fn.Import()
		if moduleName != envModuleName {
			continue
		}
		key := signatureKey(fn.ParamTypes(), fn.ResultTypes())
		typeIndex, ok := typeMap[key]
		if !ok {
			typeIndex = uint32(len(types))
			typeMap[key] = typeIndex
			types = append(types, typeEntry{params: fn.ParamTypes(), results: fn.ResultTypes()})
		}
		funcs = append(funcs, functionImport{name: name, typeIndex: typeIndex})
	}

	typeSection := appendU32(nil, uint32(len(types)))
	for _, typ := range types {
		typeSection = append(typeSection, 0x60)
		typeSection = appendValueTypes(typeSection, typ.params)
		typeSection = appendValueTypes(typeSection, typ.results)
	}

	importCount := uint32(len(funcs) + len(imports.Memories) + len(imports.Tables))
	importSection := appendU32(nil, importCount)
	if len(imports.Memories) != 0 {
		importSection = appendName(importSection, sharedModuleName)
		importSection = appendName(importSection, imports.Memories[0].Name)
		importSection = append(importSection, 0x02)
		importSection = appendLimits(importSection, imports.Memories[0].Limits)
	}
	if len(imports.Tables) != 0 {
		importSection = appendName(importSection, sharedModuleName)
		importSection = appendName(importSection, imports.Tables[0].Name)
		importSection = append(importSection, 0x01, 0x70)
		importSection = appendLimits(importSection, imports.Tables[0].Limits)
	}
	for _, fn := range funcs {
		importSection = appendName(importSection, goenvModuleName)
		importSection = appendName(importSection, fn.name)
		importSection = append(importSection, 0x00)
		importSection = appendU32(importSection, fn.typeIndex)
	}

	exportCount := uint32(len(funcs) + len(imports.Memories) + len(imports.Tables))
	exportSection := appendU32(nil, exportCount)
	exportSection = appendExternExports(exportSection, imports)
	for i, fn := range funcs {
		exportSection = appendName(exportSection, fn.name)
		exportSection = append(exportSection, 0x00)
		exportSection = appendU32(exportSection, uint32(i))
	}

	return wasmModule(
		wasmSection(1, typeSection),
		wasmSection(2, importSection),
		wasmSection(7, exportSection),
	), nil
}

// appendExternExports appends the memory and table export entries that
// re-export every imported extern under its import name.
func appendExternExports(out []byte, imports *wasmExternImports) []byte {
	if len(imports.Memories) != 0 {
		out = appendName(out, imports.Memories[0].Name)
		out = append(out, 0x02)
		out = appendU32(out, 0)
	}
	if len(imports.Tables) != 0 {
		out = appendName(out, imports.Tables[0].Name)
		out = append(out, 0x01)
		out = appendU32(out, 0)
	}
	return out
}

// signatureKey builds a map key identifying a function signature.
func signatureKey(params, results []api.ValueType) string {
	var b strings.Builder
	for _, typ := range params {
		b.WriteByte(typ)
	}
	b.WriteByte(':')
	for _, typ := range results {
		b.WriteByte(typ)
	}
	return b.String()
}

// appendValueTypes writes a length-prefixed list of value types.
func appendValueTypes(out []byte, types []api.ValueType) []byte {
	out = appendU32(out, uint32(len(types)))
	for _, typ := range types {
		out = append(out, wasmValueType(typ))
	}
	return out
}

// wasmValueType maps an api value type to its binary encoding byte.
func wasmValueType(typ api.ValueType) byte {
	switch typ {
	case api.ValueTypeI32:
		return 0x7f
	case api.ValueTypeI64:
		return 0x7e
	case api.ValueTypeF32:
		return 0x7d
	case api.ValueTypeF64:
		return 0x7c
	default:
		panic("unsupported wasm value type")
	}
}

// wasmModule emits the wasm magic header followed by whole sections.
func wasmModule(sections ...[]byte) []byte {
	out := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	for _, section := range sections {
		out = append(out, section...)
	}
	return out
}

// wasmSection frames one section: id byte plus LEB128 length prefix.
func wasmSection(id byte, payload []byte) []byte {
	out := []byte{id}
	out = appendU32(out, uint32(len(payload)))
	out = append(out, payload...)
	return out
}

// appendName writes a length-prefixed wasm name string.
func appendName(out []byte, name string) []byte {
	out = appendU32(out, uint32(len(name)))
	return append(out, name...)
}

// appendLimits writes a limits flags/min/max triple.
func appendLimits(out []byte, limits wasmLimits) []byte {
	if limits.Max == nil {
		out = appendU32(out, 0)
		return appendU32(out, limits.Min)
	}
	out = appendU32(out, 1)
	out = appendU32(out, limits.Min)
	return appendU32(out, *limits.Max)
}

// appendU32 encodes one unsigned LEB128 value.
func appendU32(out []byte, value uint32) []byte {
	for {
		b := byte(value & 0x7f)
		value >>= 7
		if value != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if value == 0 {
			return out
		}
	}
}
