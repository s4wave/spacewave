export interface GoScriptBlake3Sidecar {
  hash(data: Uint8Array, outLen: number): Uint8Array
  keyedHash(key: Uint8Array, data: Uint8Array, outLen: number): Uint8Array
  deriveKey(context: string, material: Uint8Array, outLen: number): Uint8Array
}

declare global {
  var BLDR_BLAKE3: GoScriptBlake3Sidecar | undefined
}

type Blake3WasmExports = {
  memory: WebAssembly.Memory
  blake3_workspace(len: number): number
  blake3_hash(
    dataPtr: number,
    dataLen: number,
    outPtr: number,
    outLen: number,
  ): void
  blake3_keyed_hash(
    keyPtr: number,
    dataPtr: number,
    dataLen: number,
    outPtr: number,
    outLen: number,
  ): void
  blake3_derive_key(
    contextPtr: number,
    contextLen: number,
    materialPtr: number,
    materialLen: number,
    outPtr: number,
    outLen: number,
  ): void
}

const blake3SidecarPath = './sidecars/blake3.wasm'
const textEncoder = new TextEncoder()

export async function installGoScriptBlake3Sidecar(): Promise<void> {
  if (globalThis.BLDR_BLAKE3) {
    return
  }
  const wasmURL = new URL(blake3SidecarPath, import.meta.url)
  const response = await fetch(wasmURL)
  if (!response.ok) {
    throw new Error(`failed to load BLAKE3 sidecar: ${response.status}`)
  }
  const { instance } = await WebAssembly.instantiate(
    await response.arrayBuffer(),
    {},
  )
  globalThis.BLDR_BLAKE3 = new WasmBlake3Sidecar(
    instance.exports as Blake3WasmExports,
  )
}

class WasmBlake3Sidecar implements GoScriptBlake3Sidecar {
  public constructor(private readonly exports: Blake3WasmExports) {}

  public hash(data: Uint8Array, outLen: number): Uint8Array {
    const frame = this.allocFrame(data.byteLength, outLen)
    frame.memory.set(data, frame.ptr)
    this.exports.blake3_hash(frame.ptr, data.byteLength, frame.outPtr, outLen)
    return frame.memory.slice(frame.outPtr, frame.outPtr + outLen)
  }

  public keyedHash(
    key: Uint8Array,
    data: Uint8Array,
    outLen: number,
  ): Uint8Array {
    if (key.byteLength !== 32) {
      throw new Error(`BLAKE3 key length ${key.byteLength} is not 32 bytes`)
    }
    const dataPtr = key.byteLength
    const frame = this.allocFrame(key.byteLength + data.byteLength, outLen)
    frame.memory.set(key, frame.ptr)
    frame.memory.set(data, frame.ptr + dataPtr)
    this.exports.blake3_keyed_hash(
      frame.ptr,
      frame.ptr + dataPtr,
      data.byteLength,
      frame.outPtr,
      outLen,
    )
    return frame.memory.slice(frame.outPtr, frame.outPtr + outLen)
  }

  public deriveKey(
    context: string,
    material: Uint8Array,
    outLen: number,
  ): Uint8Array {
    const contextBytes = textEncoder.encode(context)
    const materialPtr = contextBytes.byteLength
    const frame = this.allocFrame(
      contextBytes.byteLength + material.byteLength,
      outLen,
    )
    frame.memory.set(contextBytes, frame.ptr)
    frame.memory.set(material, frame.ptr + materialPtr)
    this.exports.blake3_derive_key(
      frame.ptr,
      contextBytes.byteLength,
      frame.ptr + materialPtr,
      material.byteLength,
      frame.outPtr,
      outLen,
    )
    return frame.memory.slice(frame.outPtr, frame.outPtr + outLen)
  }

  private allocFrame(inputLen: number, outLen: number): Blake3CallFrame {
    const ptr = this.exports.blake3_workspace(inputLen + outLen)
    if (ptr === 0) {
      throw new Error('BLAKE3 sidecar workspace allocation failed')
    }
    return {
      ptr,
      outPtr: ptr + inputLen,
      memory: new Uint8Array(this.exports.memory.buffer),
    }
  }
}

type Blake3CallFrame = {
  ptr: number
  outPtr: number
  memory: Uint8Array
}
