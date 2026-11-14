import type { V86Config } from '@s4wave/sdk/vm/v86.pb.js'
import type { V86fsAdapter, V86fsNotifier } from './v86fs-bridge.js'
import {
  ScreenRelay,
  v86ScreenChannelName,
  type ScreenEmulator,
} from './screen-channel.js'
import { v86SerialChannelName, type SerialFrame } from './serial-channel.js'

type V86Emulator = ScreenEmulator & {
  add_listener(
    event: 'serial0-output-byte',
    callback: (data: number) => void,
  ): void
  serial0_send(data: string): void
  stop(): void
  destroy(): void
  v86: { cpu: { devices: { virtio_v86fs: V86fsNotifier } } }
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

// V86RuntimeEnvironment describes the context the runner executes in. Only a
// worker takes keyboard input, because in a document v86 would read every
// key pressed on the page.
export type V86RuntimeEnvironment = {
  executionOwner: 'main-document' | 'worker' | 'unknown'
  hasBroadcastChannel: boolean
}

export type BrowserV86Runtime = {
  serialChannelName: string
  ready: Promise<void>
  stop(): void
}

export type StartBrowserV86RuntimeOptions = {
  assets: V86BootAssets
  config?: V86Config
  // objectKey is the VmV86 object key; it names the serial channel.
  objectKey: string
  v86fsAdapter: V86fsAdapter
  // attachV86fsNotifier receives the guest filesystem device once the guest
  // reaches userspace. The guest mounts with usermode helpers, so mount
  // notifications delivered earlier fail.
  attachV86fsNotifier?: (notifier: V86fsNotifier) => void
}

const mib = 1024 * 1024
const wasmPageBytes = 64 * 1024
const defaultGuestMemoryMb = 256
const defaultVgaMemoryMb = 8
const v86WasmMemoryHeadroomBytes = 16 * mib
// v86BaseCmdline boots through /sbin/init so systemd runs the ttyS0 shell with
// job control; bash as PID 1 breaks Ctrl-C and SIGINT. The empty rootflags
// clears the kernel's built-in 9p flags, which v86fs rejects. quiet keeps
// kernel and systemd progress off the console, so the console opens on the
// shell; boot args such as loglevel=7 come after it and restore the log.
const v86BaseCmdline =
  'rw init=/sbin/init root=v86fs rootfstype=v86fs rootflags= console=ttyS0 quiet'
// v86minBootReadyMarker identifies the first guest output that proves the
// kernel and init path reached an interactive boot state.
export const v86minBootReadyMarker = /(?:login:|# |\$ |Welcome)/

const v86GuestFailureMarker = /(?:kernel panic|panic:|v86 runtime error)/i

function defaultV86ModuleLoader(): Promise<V86Module> {
  return import('@aptre/v86')
}

// detectV86RuntimeEnvironment inspects globals for the runner's context.
export function detectV86RuntimeEnvironment(
  globals: typeof globalThis = globalThis,
): V86RuntimeEnvironment {
  const values = globals as Record<string, unknown>
  const hasDocument =
    typeof values.document !== 'undefined' &&
    typeof values.window !== 'undefined'
  const hasSelf = typeof values.self !== 'undefined'
  return {
    executionOwner: hasDocument
      ? 'main-document'
      : hasSelf
        ? 'worker'
        : 'unknown',
    hasBroadcastChannel: typeof values.BroadcastChannel !== 'undefined',
  }
}

function assertV86RuntimeEnvironment(env: V86RuntimeEnvironment): void {
  if (!env.hasBroadcastChannel) {
    throw new Error(
      'spacewave-v86 browser runner requires BroadcastChannel for the viewer bridge',
    )
  }
}

async function loadV86Module(
  env: V86RuntimeEnvironment,
  loadV86Module: V86ModuleLoader,
): Promise<V86Module> {
  try {
    return await loadV86Module()
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    throw new Error(
      'failed to import @aptre/v86 for ' +
        env.executionOwner +
        ' backend; provide a worker-safe V86 module or move DOM-dependent boot into a main-document bridge: ' +
        message,
      { cause: err },
    )
  }
}

// viewBuffer returns an ArrayBuffer holding exactly data's bytes, reusing the
// backing buffer when data spans all of it. A SharedArrayBuffer is copied,
// because v86 and WebAssembly.instantiate take an ArrayBuffer.
function viewBuffer(data: Uint8Array): ArrayBuffer {
  const { buffer } = data
  if (
    buffer instanceof ArrayBuffer &&
    data.byteOffset === 0 &&
    data.byteLength === buffer.byteLength
  ) {
    return buffer
  }
  return data.slice().buffer
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
  if (!options.objectKey) {
    throw new Error('spacewave-v86 browser runner requires a VM object key')
  }

  const env = detectV86RuntimeEnvironment()
  assertV86RuntimeEnvironment(env)
  const { V86 } = await loadV86Module(env, loadModule)

  const guestMemoryBytes =
    (options.config?.memoryMb || defaultGuestMemoryMb) * mib
  const vgaMemoryBytes =
    (options.config?.vgaMemoryMb || defaultVgaMemoryMb) * mib
  const bootArgs = options.config?.bootArgs?.trim()
  const cmdline = bootArgs ? v86BaseCmdline + ' ' + bootArgs : v86BaseCmdline

  const serialChannelName = v86SerialChannelName(options.objectKey)
  const serialChannel = new BroadcastChannel(serialChannelName)
  const screenChannel = new BroadcastChannel(
    v86ScreenChannelName(options.objectKey),
  )
  const screen = new ScreenRelay(screenChannel)
  let stopped = false
  let emulator: V86Emulator | undefined

  let readySettled = false
  const readyResolvers = Promise.withResolvers<void>()
  const ready = readyResolvers.promise
  const settleReady = (error?: Error) => {
    if (readySettled) return
    readySettled = true
    if (error) {
      readyResolvers.reject(error)
    } else {
      readyResolvers.resolve()
    }
  }

  // serialBatch collects the bytes the guest emits in one emulator slice so
  // the viewer receives one frame per slice instead of one per byte. Until
  // the guest is ready, the decoded tail is scanned for the boot markers.
  let serialBatch: number[] = []
  let serialText = ''
  const serialDecoder = new TextDecoder()
  const flushSerialBatch = () => {
    const data = Uint8Array.from(serialBatch)
    serialBatch = []
    serialChannel.postMessage({ dir: 'out', data } satisfies SerialFrame)
    if (readySettled) return

    serialText = (
      serialText + serialDecoder.decode(data, { stream: true })
    ).slice(-8192)
    const failure = v86GuestFailureMarker.exec(serialText)
    if (failure) {
      const line = serialText.slice(failure.index).split('\n', 1)[0].trim()
      settleReady(new Error('v86min guest boot failed: ' + line))
    } else if (v86minBootReadyMarker.test(serialText)) {
      settleReady()
    }
  }

  try {
    const minimumWasmMemoryBytes =
      guestMemoryBytes + vgaMemoryBytes + v86WasmMemoryHeadroomBytes

    const vm = new V86({
      wasm_fn: ({ env }: { env: WebAssembly.ModuleImports }) =>
        instantiateV86Wasm(
          options.assets.wasm,
          { env },
          minimumWasmMemoryBytes,
        ),
      memory_size: guestMemoryBytes,
      vga_memory_size: vgaMemoryBytes,
      bios: { buffer: viewBuffer(options.assets.bios) },
      vga_bios: { buffer: viewBuffer(options.assets.vgaBios) },
      bzimage: { buffer: viewBuffer(options.assets.kernel) },
      cmdline,
      virtio_v86fs: true,
      virtio_v86fs_adapter: options.v86fsAdapter,
      screen: { adapter: screen },
      disable_keyboard: env.executionOwner !== 'worker',
      disable_mouse: true,
      disable_speaker: true,
      autostart: true,
    })
    emulator = vm

    screen.attach(vm)
    vm.add_listener('serial0-output-byte', (byte: number) => {
      if (serialBatch.length === 0) queueMicrotask(flushSerialBatch)
      serialBatch.push(byte)
    })
    void ready.then(
      () => {
        if (stopped) return
        options.attachV86fsNotifier?.(vm.v86.cpu.devices.virtio_v86fs)
      },
      () => {},
    )
    serialChannel.onmessage = (ev: MessageEvent<SerialFrame>) => {
      const frame = ev.data
      if (!frame || frame.dir !== 'in') return
      if (frame.text) vm.serial0_send(frame.text)
    }

    return {
      serialChannelName,
      ready,
      stop() {
        if (stopped) return
        stopped = true
        settleReady()
        serialChannel.close()
        screenChannel.close()
        vm.stop()
        vm.destroy()
      },
    }
  } catch (err) {
    settleReady(err instanceof Error ? err : new Error(String(err)))
    serialChannel.close()
    screenChannel.close()
    emulator?.stop()
    emulator?.destroy()
    throw err
  }
}
