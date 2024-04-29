import { pushable, Pushable } from 'it-pushable'
import { PacketStream, castToError } from 'starpc'
import { Source } from 'it-stream-types'

// See wasm_exec.js from the Go standard library.
declare class Go {
  importObject: WebAssembly.Imports
  env: Record<string, string>
  argv: string[]
  run(inst: WebAssembly.Module): Promise<void>
}

interface Global extends WindowOrWorkerGlobalScope {
  // similar to OpenStreamFunc
  openStream: (
    onMessage: (message: Uint8Array) => void,
    onClose: (errMsg?: string) => void,
  ) => Promise<Pushable<Uint8Array>>
}
//eslint-disable-next-line
const goGlobal: Global = globalThis as any

function buildPacketStream(): PacketStream {
  const source = pushable<Uint8Array>({ objectMode: true })

  // const sinkPushable = pushable<Uint8Array>({ objectMode: true })
  // const sink = buildPushableSink(sinkPushable)

  const sink = async (source: Source<Uint8Array>) => {
    for await (const pkt of source) {
      console.log('got packet in js', pkt)
    }
    console.log('source ended in js')
  }

  source.push(new Uint8Array([0]))
  source.push(new Uint8Array([1]))
  source.push(new Uint8Array([2]))
  source.end()

  return { source, sink }
}

goGlobal.openStream = async (
  onMessage,
  onClose,
): Promise<Pushable<Uint8Array>> => {
  const packetStream = buildPacketStream()
  const packetSource = packetStream.source
  queueMicrotask(async () => {
    try {
      for await (const msg of packetSource) {
        onMessage(msg)
      }
      onClose()
    } catch (err) {
      const e = castToError(err)
      onClose(e.toString())
    }
  })

  const push = pushable<Uint8Array>({ objectMode: true })
  queueMicrotask(() => packetStream.sink(push))
  return push
}

const go = new Go()
WebAssembly.instantiateStreaming(fetch('main.wasm'), go.importObject).then(
  (result) => {
    go.run(result.instance)
  },
)
