import type { V86fsAdapter } from './v86fs-bridge.js'
import { v86SerialChannelName, type SerialFrame } from './serial-channel.js'

type V86Emulator = {
  add_listener(
    event: 'serial0-output-byte',
    callback: (data: number) => void,
  ): void
  serial0_send(data: string): void
  stop(): void
  destroy(): void
}

type V86Constructor = new (options: Record<string, unknown>) => V86Emulator

type V86Module = {
  V86: V86Constructor
}

export type V86ModuleLoader = () => Promise<V86Module>

export type V86BootAssets = {
  wasm: Uint8Array
  bios: Uint8Array
  vgaBios: Uint8Array
  kernel: Uint8Array
}

export type V86RuntimeEnvironment = {
  executionOwner: 'main-document' | 'worker' | 'unknown'
  hasDocument: boolean
  hasWindow: boolean
  hasSelf: boolean
  hasBroadcastChannel: boolean
  hasOffscreenCanvas: boolean
}

export type V86RunnerContract = {
  backendExecution: V86RuntimeEnvironment['executionOwner']
  displayOwner: 'viewer'
  filesystemBridge: 'v86fs-adapter'
  screenBridge: 'headless'
  terminalBridge: 'BroadcastChannel'
}

export type BrowserV86Runtime = {
  serialChannelName: string
  ready: Promise<void>
  stop(): void
}

export type StartBrowserV86RuntimeOptions = {
  assets: V86BootAssets
  instanceKey: string
  v86fsAdapter: V86fsAdapter
}

const wasmPageBytes = 64 * 1024
const v86GuestMemoryBytes = 256 * 1024 * 1024
const v86VgaMemoryBytes = 2 * 1024 * 1024
const v86WasmMemoryHeadroomBytes = 16 * 1024 * 1024
// v86minBootReadyMarker identifies the first guest output that proves the
// kernel and init path reached an interactive boot state.
export const v86minBootReadyMarker = /(?:login:|# |\$ |Welcome)/

const v86GuestFailureMarker = /(?:kernel panic|panic:|v86 runtime error)/i

function defaultV86ModuleLoader(): Promise<V86Module> {
  return import('@aptre/v86')
}

export function detectV86RuntimeEnvironment(
  globals: typeof globalThis = globalThis,
): V86RuntimeEnvironment {
  const values = globals as Record<string, unknown>
  const hasDocument = typeof values.document !== 'undefined'
  const hasWindow = typeof values.window !== 'undefined'
  const hasSelf = typeof values.self !== 'undefined'
  const executionOwner =
    hasDocument && hasWindow ? 'main-document' : hasSelf ? 'worker' : 'unknown'

  return {
    executionOwner,
    hasDocument,
    hasWindow,
    hasSelf,
    hasBroadcastChannel: typeof values.BroadcastChannel !== 'undefined',
    hasOffscreenCanvas: typeof values.OffscreenCanvas !== 'undefined',
  }
}

export function getV86RunnerContract(
  env: V86RuntimeEnvironment = detectV86RuntimeEnvironment(),
): V86RunnerContract {
  return {
    backendExecution: env.executionOwner,
    displayOwner: 'viewer',
    filesystemBridge: 'v86fs-adapter',
    screenBridge: 'headless',
    terminalBridge: 'BroadcastChannel',
  }
}

function assertV86RuntimeEnvironment(env: V86RuntimeEnvironment): void {
  if (!env.hasBroadcastChannel) {
    throw new Error(
      'spacewave-v86 browser runner requires BroadcastChannel for the viewer serial bridge',
    )
  }
}

async function loadV86Module(
  contract: V86RunnerContract,
  loadV86Module: V86ModuleLoader,
): Promise<V86Module> {
  try {
    return await loadV86Module()
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    throw new Error(
      'failed to import @aptre/v86 for ' +
        contract.backendExecution +
        ' backend; provide a worker-safe V86 module or move DOM-dependent boot into a main-document bridge: ' +
        message,
      { cause: err },
    )
  }
}

function viewBuffer(data: Uint8Array): ArrayBuffer {
  if (data.byteOffset === 0 && data.byteLength === data.buffer.byteLength) {
    return data.buffer
  }
  return data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength)
}

function preGrowV86WasmMemory(
  imports: WebAssembly.Imports,
  minimumBytes: number,
): void {
  const env = (imports as { env?: { memory?: WebAssembly.Memory } }).env
  const memory = env?.memory
  if (!(memory instanceof WebAssembly.Memory)) return

  const currentBytes = memory.buffer.byteLength
  if (currentBytes >= minimumBytes) return

  memory.grow(Math.ceil((minimumBytes - currentBytes) / wasmPageBytes))
}

async function instantiateV86Wasm(
  wasm: Uint8Array,
  imports: WebAssembly.Imports,
  minimumMemoryBytes: number,
): Promise<WebAssembly.Exports> {
  preGrowV86WasmMemory(imports, minimumMemoryBytes)
  const { instance } = await WebAssembly.instantiate(viewBuffer(wasm), imports)
  return instance.exports
}

export async function startBrowserV86Runtime(
  options: StartBrowserV86RuntimeOptions,
  loadModule: V86ModuleLoader = defaultV86ModuleLoader,
): Promise<BrowserV86Runtime> {
  if (!options.instanceKey) {
    throw new Error('spacewave-v86 browser runner requires an instance key')
  }

  const env = detectV86RuntimeEnvironment()
  assertV86RuntimeEnvironment(env)
  const contract = getV86RunnerContract(env)
  const { V86 } = await loadV86Module(contract, loadModule)

  const serialChannelName = v86SerialChannelName(options.instanceKey)
  const serialChannel = new BroadcastChannel(serialChannelName)
  let stopped = false
  let emulator: V86Emulator | undefined
  let serialText = ''
  let readySettled = false
  const readyResolvers = Promise.withResolvers<void>()
  const ready = readyResolvers.promise
  const resolveReady = readyResolvers.resolve
  const rejectReady = readyResolvers.reject
  const settleReady = (error?: Error) => {
    if (readySettled) return
    readySettled = true
    if (error) {
      rejectReady(error)
    } else {
      resolveReady()
    }
  }

  try {
    const minimumWasmMemoryBytes =
      v86GuestMemoryBytes + v86VgaMemoryBytes + v86WasmMemoryHeadroomBytes

    emulator = new V86({
      wasm_fn: ({ env }: { env: WebAssembly.ModuleImports }) =>
        instantiateV86Wasm(
          options.assets.wasm,
          { env },
          minimumWasmMemoryBytes,
        ),
      memory_size: v86GuestMemoryBytes,
      vga_memory_size: v86VgaMemoryBytes,
      bios: { buffer: viewBuffer(options.assets.bios) },
      vga_bios: { buffer: viewBuffer(options.assets.vgaBios) },
      bzimage: { buffer: viewBuffer(options.assets.kernel) },
      // Boot through /sbin/init so busybox gives ttyS0 job control; bash PID 1 breaks Ctrl-C/SIGINT.
      cmdline: 'rw init=/sbin/init root=v86fs rootfstype=v86fs console=ttyS0',
      virtio_v86fs: true,
      virtio_v86fs_adapter: options.v86fsAdapter,
      disable_keyboard: true,
      disable_mouse: true,
      disable_speaker: true,
      autostart: true,
    })

    emulator.add_listener('serial0-output-byte', (byte: number) => {
      serialChannel.postMessage({ dir: 'out', byte })
      serialText = (serialText + String.fromCharCode(byte)).slice(-8192)
      if (v86GuestFailureMarker.test(serialText)) {
        settleReady(new Error('v86min guest boot failed'))
      } else if (v86minBootReadyMarker.test(serialText)) {
        settleReady()
      }
    })
    serialChannel.onmessage = (ev: MessageEvent<SerialFrame>) => {
      const frame = ev.data
      if (!frame || frame.dir !== 'in') return
      if (typeof frame.text === 'string' && frame.text.length > 0) {
        emulator.serial0_send(frame.text)
      }
    }

    return {
      serialChannelName,
      ready,
      stop() {
        if (stopped) return
        stopped = true
        settleReady()
        serialChannel.close()
        emulator.stop()
        emulator.destroy()
      },
    }
  } catch (err) {
    settleReady(err instanceof Error ? err : new Error(String(err)))
    serialChannel.close()
    emulator?.stop()
    emulator?.destroy()
    throw err
  }
}
