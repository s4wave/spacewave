package v86_wazero

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

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
	exportSection := appendU32(nil, uint32(len(imports.Memories)+len(imports.Tables)))
	if len(imports.Memories) != 0 {
		exportSection = appendName(exportSection, imports.Memories[0].Name)
		exportSection = append(exportSection, 0x02)
		exportSection = appendU32(exportSection, 0)
	}
	if len(imports.Tables) != 0 {
		exportSection = appendName(exportSection, imports.Tables[0].Name)
		exportSection = append(exportSection, 0x01)
		exportSection = appendU32(exportSection, 0)
	}
	sections = append(sections, wasmSection(7, exportSection))
	return wasmModule(sections...), nil
}

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
	if len(imports.Memories) != 0 {
		exportSection = appendName(exportSection, imports.Memories[0].Name)
		exportSection = append(exportSection, 0x02)
		exportSection = appendU32(exportSection, 0)
	}
	if len(imports.Tables) != 0 {
		exportSection = appendName(exportSection, imports.Tables[0].Name)
		exportSection = append(exportSection, 0x01)
		exportSection = appendU32(exportSection, 0)
	}
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

func appendValueTypes(out []byte, types []api.ValueType) []byte {
	out = appendU32(out, uint32(len(types)))
	for _, typ := range types {
		out = append(out, wasmValueType(typ))
	}
	return out
}

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

func wasmModule(sections ...[]byte) []byte {
	out := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	for _, section := range sections {
		out = append(out, section...)
	}
	return out
}

func wasmSection(id byte, payload []byte) []byte {
	out := []byte{id}
	out = appendU32(out, uint32(len(payload)))
	out = append(out, payload...)
	return out
}

func appendName(out []byte, name string) []byte {
	out = appendU32(out, uint32(len(name)))
	return append(out, name...)
}

func appendLimits(out []byte, limits wasmLimits) []byte {
	if limits.Max == nil {
		out = appendU32(out, 0)
		return appendU32(out, limits.Min)
	}
	out = appendU32(out, 1)
	out = appendU32(out, limits.Min)
	return appendU32(out, *limits.Max)
}

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
