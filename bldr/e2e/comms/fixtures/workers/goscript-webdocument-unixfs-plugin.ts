import type { BackendAPI } from '@aptre/bldr-sdk'

import runGoScriptPlugin from '../../../../web/runtime/goscript/plugin-goscript.js'


type GoPushableSink = {
  push: (message: Uint8Array) => void
  end: () => void
}

declare global {
  var BLDR_PLUGIN_START_INFO: string | undefined
  var BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME:
    | ((
        onMessage: (message: Uint8Array) => void,
        onClose: (errMsg?: string) => void,
        onResolve: (sink: GoPushableSink) => void,
        onReject: (errMsg: string) => void,
      ) => void)
    | undefined
  var BLDR_PLUGIN_SET_ACCEPT_STREAM:
    | ((acceptStream?: (localPort: MessagePort) => void) => void)
    | undefined
  var BLDR_PLUGIN_REPORT_RUNTIME_FAILURE: ((err: unknown) => void) | undefined
}

export default async function main(api: BackendAPI) {
  await runGoScriptPlugin(api, pluginMain)
}

async function pluginMain(): Promise<void> {
  installAcceptStream()

  const response = await openStreamToWebRuntime()
  if (response[0] !== 12) {
    throw new Error(
      `unexpected WebRuntime response packet: ${Array.from(response).join(',')}`,
    )
  }
  console.info('__BLDR_JS_PLUGIN_READY__')

  const { promise: keepAlive } = Promise.withResolvers<void>()
  await keepAlive
}

function installAcceptStream(): void {
  const setAcceptStream = globalThis.BLDR_PLUGIN_SET_ACCEPT_STREAM
  if (!setAcceptStream) {
    throw new Error('missing BLDR_PLUGIN_SET_ACCEPT_STREAM')
  }

  setAcceptStream((port: MessagePort) => {
    let releasePacket: ((message: Uint8Array) => void) | undefined
    port.onmessage = (ev: MessageEvent<Uint8Array>) => {
      const message = ev.data
      if (releasePacket) {
        const release = releasePacket
        releasePacket = undefined
        release(message)
        return
      }

      if (message?.[0] === 21) {
        port.postMessage(new Uint8Array([22]))
        return
      }
      if (message?.[0] === 31) {
        void handleInFlightReloadTrigger(port, (resolve) => {
          releasePacket = resolve
        })
        return
      }
      if (message?.[0] === 41) {
        void handleTerminalOrphanFailFast(port)
        return
      }
      port.postMessage(new Uint8Array([99]))
    }
    port.start()
  })
}

async function handleInFlightReloadTrigger(
  port: MessagePort,
  setReleasePacket: (resolve: (message: Uint8Array) => void) => void,
): Promise<void> {
  try {
    port.postMessage(new Uint8Array([32]))
    const { promise: waitRelease, resolve } =
      Promise.withResolvers<Uint8Array>()
    setReleasePacket(resolve)
    const release = await waitRelease
    if (release[0] !== 33) {
      throw new Error(
        `unexpected in-flight reload trigger packet: ${Array.from(release).join(',')}`,
      )
    }

    port.postMessage(new Uint8Array([34]))
    const { promise: waitCloseAck, resolve: resolveCloseAck } =
      Promise.withResolvers<Uint8Array>()
    setReleasePacket(resolveCloseAck)
    const closeAck = await waitCloseAck
    if (closeAck[0] !== 35) {
      throw new Error(
        `unexpected in-flight reload close ack: ${Array.from(closeAck).join(',')}`,
      )
    }
    const response = await openStreamToWebRuntime()
    if (response[0] !== 12) {
      throw new Error(
        `unexpected in-flight WebRuntime response packet: ${Array.from(response).join(',')}`,
      )
    }
  } catch (err) {
    globalThis.BLDR_PLUGIN_REPORT_RUNTIME_FAILURE?.(err)
    throw err
  }
}

type HeldStreamOutcome = { ok: boolean; err?: string }

// handleTerminalOrphanFailFast holds an active plugin-to-runtime stream open,
// signals armed, then reports how the stream settles once the fixture discards
// the last WebDocument with a terminal close. A deliberate discard must fail the
// orphaned stream fast rather than keep it waiting for a replacement route.
async function handleTerminalOrphanFailFast(port: MessagePort): Promise<void> {
  const active = Promise.withResolvers<void>()
  const outcome = openHeldStreamToWebRuntime(active.resolve)
  await active.promise
  port.postMessage(new Uint8Array([42]))
  const result = await outcome
  if (result.ok) {
    port.postMessage(new Uint8Array([45]))
    return
  }
  const errBytes = new TextEncoder().encode(result.err ?? '')
  const packet = new Uint8Array(errBytes.length + 1)
  packet[0] = 44
  packet.set(errBytes, 1)
  port.postMessage(packet)
}

// openHeldStreamToWebRuntime opens a runtime stream, pushes a hold marker the
// host never answers, and resolves once the stream settles: a client close
// yields the terminal error, an unexpected host response yields ok.
function openHeldStreamToWebRuntime(
  onActive: () => void,
): Promise<HeldStreamOutcome> {
  const openStream = globalThis.BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME
  if (!openStream) {
    throw new Error('missing BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME')
  }

  const { promise, resolve } = Promise.withResolvers<HeldStreamOutcome>()
  let settled = false
  const settle = (out: HeldStreamOutcome) => {
    if (settled) {
      return
    }
    settled = true
    resolve(out)
  }

  openStream(
    () => settle({ ok: true }),
    (errMsg) => {
      if (errMsg) {
        settle({ ok: false, err: errMsg })
      }
    },
    (sink) => {
      sink.push(new Uint8Array([51]))
      onActive()
    },
    (errMsg) => settle({ ok: false, err: errMsg }),
  )

  return promise
}

function openStreamToWebRuntime(): Promise<Uint8Array> {
  const openStream = globalThis.BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME
  if (!openStream) {
    throw new Error('missing BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME')
  }

  const { promise, resolve, reject } = Promise.withResolvers<Uint8Array>()
  let streamSink: GoPushableSink | undefined
  let settled = false
  const settle = (fn: () => void) => {
    if (settled) {
      return
    }
    settled = true
    fn()
  }

  openStream(
    (message) => {
      streamSink?.end()
      settle(() => resolve(message))
    },
    (errMsg) => {
      if (!errMsg) {
        return
      }
      settle(() => reject(new Error(errMsg)))
    },
    (sink) => {
      streamSink = sink
      sink.push(buildStartInfoRequest())
    },
    (errMsg) => {
      settle(() => reject(new Error(errMsg)))
    },
  )

  return promise
}

function buildStartInfoRequest(): Uint8Array {
  const encoded = globalThis.BLDR_PLUGIN_START_INFO
  if (!encoded) {
    throw new Error('missing BLDR_PLUGIN_START_INFO')
  }
  const startInfo = new TextEncoder().encode(atob(encoded))
  const request = new Uint8Array(startInfo.length + 1)
  request[0] = 11
  request.set(startInfo, 1)
  return request
}
