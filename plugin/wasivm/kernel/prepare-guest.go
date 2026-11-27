package kernel

import "strconv"

// guestStackPointer is the global holding a guest's C stack pointer. setjmp
// saves it and longjmp restores it.
const guestStackPointer = "__stack_pointer"

// guestGlobalPrefix starts the export name the kernel gives a mutable global
// the guest does not export, followed by the global's index.
const guestGlobalPrefix = "__wasivm_global_"

// guestSignalTrampoline is the function the kernel adds to each guest to run
// a signal handler. It takes the handler's table index and the signal number
// and calls the handler with the signal number.
const guestSignalTrampoline = "__wasivm_deliver_signal"

// prepareGuest returns the guest binary wasm with the exports the kernel
// relies on: the C stack pointer, every mutable global, and the signal
// trampoline when the guest has a function table to hold handlers. It also
// returns the export names of the mutable globals, which fork copies.
func prepareGuest(wasm []byte) ([]byte, []string, error) {
	// Read the module and its exports.
	m, err := parseGuestModule(wasm)
	if err != nil {
		return nil, nil, err
	}
	exports, err := m.exports()
	if err != nil {
		return nil, nil, err
	}
	exportedGlobals := make(map[uint32]string)
	for name, exp := range exports {
		if exp.kind == wasmExportGlobal {
			exportedGlobals[exp.index] = name
		}
	}

	// Export the stack pointer the name section identifies.
	if _, ok := exports[guestStackPointer]; !ok {
		if index, ok := m.namedGlobal(guestStackPointer); ok {
			if err := m.addExport(guestStackPointer, wasmExportGlobal, index); err != nil {
				return nil, nil, err
			}
			exportedGlobals[index] = guestStackPointer
		}
	}

	// Export the other mutable globals.
	indices, err := m.mutableGlobals()
	if err != nil {
		return nil, nil, err
	}
	globals := make([]string, 0, len(indices))
	for _, index := range indices {
		name, ok := exportedGlobals[index]
		if !ok {
			name = guestGlobalPrefix + strconv.FormatUint(uint64(index), 10)
			if err := m.addExport(name, wasmExportGlobal, index); err != nil {
				return nil, nil, err
			}
		}
		globals = append(globals, name)
	}

	// Add the signal trampoline.
	if _, ok := exports[guestSignalTrampoline]; !ok {
		if err := addSignalTrampoline(m); err != nil {
			return nil, nil, err
		}
	}
	return m.encode(), globals, nil
}

// addSignalTrampoline defines and exports the signal trampoline in m. A
// guest without a table holds no function pointers, so it gets no trampoline.
func addSignalTrampoline(m *guestModule) error {
	// Find the types of a handler, (i32) -> (), and of the trampoline,
	// (i32 i32) -> ().
	if ok, err := m.hasTable(); !ok || err != nil {
		return err
	}
	handlerType, err := m.funcType([]byte{0x7f}, nil)
	if err != nil {
		return err
	}
	trampolineType, err := m.funcType([]byte{0x7f, 0x7f}, nil)
	if err != nil {
		return err
	}

	// Push the signal number, then call the handler through table 0:
	// no locals, local.get 1, local.get 0, call_indirect, end.
	body := []byte{0x00, 0x20, 0x01, 0x20, 0x00, 0x11}
	body = appendWasmU32(body, handlerType)
	body = append(body, 0x00, 0x0b)
	index, err := m.addFunction(trampolineType, body)
	if err != nil {
		return err
	}
	return m.addExport(guestSignalTrampoline, wasmExportFunc, index)
}
