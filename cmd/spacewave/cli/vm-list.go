//go:build !js

package spacewave_cli

import (
	"context"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cli"
	protojson "github.com/aperturerobotics/protobuf-go-lite/json"
	"github.com/pkg/errors"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// vmCLIEntry is a VM of any type as the list and info commands show it.
type vmCLIEntry interface {
	// key returns the VM object key.
	key() string
	// listRow returns the row of the VM in the list table.
	listRow() []string
	// infoFields returns the labeled fields of the VM info.
	infoFields() [][2]string
	// writeJSON writes the VM as one JSON object.
	writeJSON(ms *protojson.MarshalState)
}

// vmListHeader is the header row of the VM list table.
var vmListHeader = []string{"KEY", "TYPE", "NAME", "STATE", "SOURCE"}

// v86VMCLIEntry is a v86 VM and the objects its edges link.
type v86VMCLIEntry struct {
	objectKey string
	vm        *s4wave_vm.VmV86
	edges     map[string]string
}

// v86VMEdgePreds records the graph edges of a v86 VM.
var v86VMEdgePreds = map[string]quad.IRI{
	"image":          s4wave_vm.PredV86Image,
	"kernelOverride": s4wave_vm.PredV86KernelOverride,
	"rootfsOverride": s4wave_vm.PredV86RootfsOverride,
	"biosOverride":   s4wave_vm.PredV86BiosOverride,
	"wasmOverride":   s4wave_vm.PredV86WasmOverride,
}

func (e *v86VMCLIEntry) key() string {
	return e.objectKey
}

func (e *v86VMCLIEntry) listRow() []string {
	return []string{
		e.objectKey,
		"v86",
		e.vm.GetName(),
		vmStateLabel(e.vm.GetObservedState()),
		e.edges["image"],
	}
}

func (e *v86VMCLIEntry) infoFields() [][2]string {
	cfg := e.vm.GetConfig()
	return [][2]string{
		{"Key", e.objectKey},
		{"Type", "v86"},
		{"Name", e.vm.GetName()},
		{"State", vmStateLabel(e.vm.GetObservedState())},
		{"Desired State", vmStateLabel(e.vm.GetState())},
		{"Image", e.edges["image"]},
		{"Memory MB", strconv.Itoa(int(cfg.GetMemoryMb()))},
		{"VGA Memory MB", strconv.Itoa(int(cfg.GetVgaMemoryMb()))},
		{"Networking", strconv.FormatBool(cfg.GetNetworking())},
		{"Serial", strconv.FormatBool(cfg.GetSerialEnabled())},
		{"Boot Args", cfg.GetBootArgs()},
		{"Runtime Plugin", cfg.GetRuntimePluginId()},
		{"Kernel Override", e.edges["kernelOverride"]},
		{"Rootfs Override", e.edges["rootfsOverride"]},
		{"BIOS Override", e.edges["biosOverride"]},
		{"WASM Override", e.edges["wasmOverride"]},
		{"Error", e.vm.GetErrorMessage()},
	}
}

func (e *v86VMCLIEntry) writeJSON(ms *protojson.MarshalState) {
	// Write the VM identity and state.
	ms.WriteObjectStart()
	var f bool
	writeJSONStringField(ms, &f, "objectKey", e.objectKey)
	writeJSONStringField(ms, &f, "type", s4wave_vm.VmV86TypeID)
	writeJSONStringField(ms, &f, "name", e.vm.GetName())
	writeJSONStringField(ms, &f, "state", e.vm.GetState().String())
	writeJSONStringField(ms, &f, "observedState", e.vm.GetObservedState().String())

	// Write the image the VM boots.
	writeJSONStringField(ms, &f, "imageObjectKey", e.edges["image"])

	// Open the VM config.
	cfg := e.vm.GetConfig()
	ms.WriteMoreIf(&f)
	ms.WriteObjectField("config")
	ms.WriteObjectStart()

	// Write the config fields and close the config.
	var cf bool
	writeJSONUint64Field(ms, &cf, "memoryMb", uint64(cfg.GetMemoryMb()))
	writeJSONUint64Field(ms, &cf, "vgaMemoryMb", uint64(cfg.GetVgaMemoryMb()))
	writeJSONBoolField(ms, &cf, "networking", cfg.GetNetworking())
	writeJSONBoolField(ms, &cf, "serialEnabled", cfg.GetSerialEnabled())
	writeJSONStringField(ms, &cf, "bootArgs", cfg.GetBootArgs())
	writeJSONStringField(ms, &cf, "runtimePluginId", cfg.GetRuntimePluginId())
	ms.WriteObjectEnd()

	// Open the overrides.
	ms.WriteMoreIf(&f)
	ms.WriteObjectField("overrides")
	ms.WriteObjectStart()

	// Write the kernel, rootfs, BIOS, and wasm overrides and close them.
	var of bool
	writeJSONStringField(ms, &of, "kernel", e.edges["kernelOverride"])
	writeJSONStringField(ms, &of, "rootfs", e.edges["rootfsOverride"])
	writeJSONStringField(ms, &of, "bios", e.edges["biosOverride"])
	writeJSONStringField(ms, &of, "wasm", e.edges["wasmOverride"])
	ms.WriteObjectEnd()

	// Write the error message and close the VM object.
	writeJSONStringField(ms, &f, "errorMessage", e.vm.GetErrorMessage())
	ms.WriteObjectEnd()
}

// wasiVmCLIEntry is a WasiVM and the root filesystem it runs on.
type wasiVmCLIEntry struct {
	objectKey string
	vm        *s4wave_vm.WasiVm
	rootfs    string
}

// wasiVmEdgePreds records the graph edges of a WasiVM.
var wasiVmEdgePreds = map[string]quad.IRI{
	"rootfs": s4wave_vm.PredWasiVmRootfs,
}

func (e *wasiVmCLIEntry) key() string {
	return e.objectKey
}

func (e *wasiVmCLIEntry) listRow() []string {
	return []string{
		e.objectKey,
		"wasivm",
		e.vm.GetName(),
		vmStateLabel(e.vm.GetObservedState()),
		e.rootfs,
	}
}

// initCommand returns the init command line, program first.
func (e *wasiVmCLIEntry) initCommand() []string {
	cfg := e.vm.GetConfig()
	initPath := cfg.GetInitPath()
	if initPath == "" {
		initPath = s4wave_vm.DefaultWasiVmInitPath
	}
	return append([]string{initPath}, cfg.GetInitArgs()...)
}

func (e *wasiVmCLIEntry) infoFields() [][2]string {
	env := e.vm.GetConfig().GetEnv()
	vars := make([]string, 0, len(env))
	for _, name := range slices.Sorted(maps.Keys(env)) {
		vars = append(vars, name+"="+env[name])
	}
	return [][2]string{
		{"Key", e.objectKey},
		{"Type", "wasivm"},
		{"Name", e.vm.GetName()},
		{"State", vmStateLabel(e.vm.GetObservedState())},
		{"Desired State", vmStateLabel(e.vm.GetState())},
		{"Root Filesystem", e.rootfs},
		{"Init", strings.Join(e.initCommand(), " ")},
		{"Environment", strings.Join(vars, " ")},
		{"Run Generation", strconv.FormatUint(e.vm.GetRunGeneration(), 10)},
		{"Error", e.vm.GetErrorMessage()},
	}
}

func (e *wasiVmCLIEntry) writeJSON(ms *protojson.MarshalState) {
	// Write the VM identity.
	ms.WriteObjectStart()
	var f bool
	writeJSONStringField(ms, &f, "objectKey", e.objectKey)
	writeJSONStringField(ms, &f, "type", s4wave_vm.WasiVmTypeID)
	writeJSONStringField(ms, &f, "name", e.vm.GetName())

	// Write the VM state and root filesystem.
	writeJSONStringField(ms, &f, "state", e.vm.GetState().String())
	writeJSONStringField(ms, &f, "observedState", e.vm.GetObservedState().String())
	writeJSONStringField(ms, &f, "rootfsObjectKey", e.rootfs)
	writeJSONUint64Field(ms, &f, "runGeneration", e.vm.GetRunGeneration())

	// Write the init command line.
	ms.WriteMoreIf(&f)
	ms.WriteObjectField("init")
	ms.WriteArrayStart()
	var af bool
	for _, arg := range e.initCommand() {
		ms.WriteMoreIf(&af)
		ms.WriteString(arg)
	}
	ms.WriteArrayEnd()

	// Write the environment.
	env := e.vm.GetConfig().GetEnv()
	ms.WriteMoreIf(&f)
	ms.WriteObjectField("env")
	ms.WriteObjectStart()
	var ef bool
	for _, name := range slices.Sorted(maps.Keys(env)) {
		writeJSONStringField(ms, &ef, name, env[name])
	}
	ms.WriteObjectEnd()

	// Write the error message and close the VM object.
	writeJSONStringField(ms, &f, "errorMessage", e.vm.GetErrorMessage())
	ms.WriteObjectEnd()
}

// vmStateLabel returns the lower case name of a VM state.
func vmStateLabel(state s4wave_vm.VmState) string {
	return strings.ToLower(strings.TrimPrefix(state.String(), "VmState_"))
}

// readVMs reads every VM in the Space, sorted by key.
func readVMs(c *cli.Context, statePath string, sessionIdx uint32, spaceID string) ([]vmCLIEntry, error) {
	var out []vmCLIEntry
	err := withVmWorldTx(c, statePath, sessionIdx, spaceID, false, func(tx world.WorldState) error {
		for _, typeID := range []string{s4wave_vm.VmV86TypeID, s4wave_vm.WasiVmTypeID} {
			keys, err := world_types.ListObjectsWithType(c.Context, tx, typeID)
			if err != nil {
				return errors.Wrap(err, "list VMs")
			}
			for _, key := range keys {
				vm, err := readVMFromTx(c.Context, tx, key, typeID)
				if err != nil {
					return err
				}
				out = append(out, vm)
			}
		}
		return nil
	})
	slices.SortFunc(out, func(a, b vmCLIEntry) int {
		return strings.Compare(a.key(), b.key())
	})
	return out, err
}

// readVM reads the VM at key.
func readVM(c *cli.Context, statePath string, sessionIdx uint32, spaceID, key string) (vmCLIEntry, error) {
	var out vmCLIEntry
	err := withVmWorldTx(c, statePath, sessionIdx, spaceID, false, func(tx world.WorldState) error {
		typeID, err := vmTypeID(c.Context, tx, key)
		if err != nil {
			return err
		}
		out, err = readVMFromTx(c.Context, tx, key, typeID)
		return err
	})
	return out, err
}

// readVMFromTx reads the VM at key of type typeID and the objects its edges
// link.
func readVMFromTx(ctx context.Context, tx world.WorldState, key, typeID string) (vmCLIEntry, error) {
	// Read a v86 VM and its image and override edges.
	if typeID == s4wave_vm.VmV86TypeID {
		vm, err := readVMBlock[s4wave_vm.VmV86](ctx, tx, key)
		if err != nil {
			return nil, err
		}
		edges, err := readVMEdges(ctx, tx, key, v86VMEdgePreds)
		if err != nil {
			return nil, err
		}
		return &v86VMCLIEntry{objectKey: key, vm: vm, edges: edges}, nil
	}

	// Read the WasiVM and its root filesystem edge.
	vm, err := readVMBlock[s4wave_vm.WasiVm](ctx, tx, key)
	if err != nil {
		return nil, err
	}
	edges, err := readVMEdges(ctx, tx, key, wasiVmEdgePreds)
	if err != nil {
		return nil, err
	}
	return &wasiVmCLIEntry{objectKey: key, vm: vm, rootfs: edges["rootfs"]}, nil
}

// vmBlock is a VM block that copies itself.
type vmBlock[T any] interface {
	block.Ptr[T]

	// CloneVT returns a deep copy of the block.
	CloneVT() *T
}

// readVMBlock reads a copy of the block of the VM object at key.
func readVMBlock[T any, PT vmBlock[T]](ctx context.Context, tx world.WorldState, key string) (PT, error) {
	vm, err := world.LookupObjectBody[T, PT](ctx, tx, key)
	if errors.Is(err, world.ErrObjectNotFound) {
		return nil, errors.Errorf("VM %q not found", key)
	}
	if err != nil {
		return nil, errors.Wrap(err, "read VM block")
	}
	return vm.CloneVT(), nil
}

// readVMEdges reads the target of each named edge of the VM at key, or ""
// for an edge the VM does not have.
func readVMEdges(ctx context.Context, tx world.WorldState, key string, preds map[string]quad.IRI) (map[string]string, error) {
	out := make(map[string]string, len(preds))
	for name, pred := range preds {
		quads, err := tx.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, string(pred), "", ""), 1)
		if err != nil {
			return nil, errors.Wrapf(err, "lookup %s edge", name)
		}
		if len(quads) == 0 {
			out[name] = ""
			continue
		}
		target, err := world.GraphValueToKey(quads[0].GetObj())
		if err != nil {
			return nil, errors.Wrapf(err, "parse %s edge", name)
		}
		out[name] = target
	}
	return out, nil
}

// writeVMList writes the VMs as JSON or YAML, or as a table.
func writeVMList(vms []vmCLIEntry, outputFormat string) error {
	// Write the VMs as one array field, or as a table.
	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var f bool
		ms.WriteMoreIf(&f)
		ms.WriteObjectField("vms")
		ms.WriteArrayStart()
		var vf bool
		for _, vm := range vms {
			ms.WriteMoreIf(&vf)
			vm.writeJSON(ms)
		}
		ms.WriteArrayEnd()
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}
	rows := [][]string{vmListHeader}
	for _, vm := range vms {
		rows = append(rows, vm.listRow())
	}
	writeTable(os.Stdout, "", rows)
	return nil
}

// writeVMInfo writes one VM as JSON or YAML, or as labeled fields.
func writeVMInfo(vm vmCLIEntry, outputFormat string) error {
	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		vm.writeJSON(ms)
		return formatOutput(buf.Bytes(), outputFormat)
	}
	writeFields(os.Stdout, vm.infoFields())
	return nil
}
