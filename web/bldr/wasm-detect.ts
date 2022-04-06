// detectWasmSupported checks if we can use wasm.
export function detectWasmSupported(): boolean {
  // https://stackoverflow.com/a/47880734/431369
  try {
    if (
      typeof WebAssembly === 'object' &&
      typeof WebAssembly.instantiate === 'function'
    ) {
      const module = new WebAssembly.Module(
        Uint8Array.of(0x0, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00)
      )
      if (module instanceof WebAssembly.Module)
        return new WebAssembly.Instance(module) instanceof WebAssembly.Instance
    }
  } catch (e) {
    // noop
  }
  return false
}
