/* eslint-disable */
import Long from 'long'
import * as _m0 from 'protobufjs/minimal'

export const protobufPackage = 'web.runtime.view'

/** EchoMsg is the message body for Echo. */
export interface EchoMsg {
  body: string
}

function createBaseEchoMsg(): EchoMsg {
  return { body: '' }
}

export const EchoMsg = {
  encode(
    message: EchoMsg,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.body !== '') {
      writer.uint32(10).string(message.body)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): EchoMsg {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseEchoMsg()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.body = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<EchoMsg, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<EchoMsg | EchoMsg[]> | Iterable<EchoMsg | EchoMsg[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [EchoMsg.encode(p).finish()]
        }
      } else {
        yield* [EchoMsg.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, EchoMsg>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<EchoMsg> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [EchoMsg.decode(p)]
        }
      } else {
        yield* [EchoMsg.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): EchoMsg {
    return {
      body: isSet(object.body) ? String(object.body) : '',
    }
  },

  toJSON(message: EchoMsg): unknown {
    const obj: any = {}
    message.body !== undefined && (obj.body = message.body)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<EchoMsg>, I>>(object: I): EchoMsg {
    const message = createBaseEchoMsg()
    message.body = object.body ?? ''
    return message
  },
}

/** WebViewHost is exposed by the Go Runtime for the WebView to call. */
export interface WebViewHost {}

export class WebViewHostClientImpl implements WebViewHost {
  private readonly rpc: Rpc
  constructor(rpc: Rpc) {
    this.rpc = rpc
  }
}

/** WebViewHost is exposed by the Go Runtime for the WebView to call. */
export type WebViewHostDefinition = typeof WebViewHostDefinition
export const WebViewHostDefinition = {
  name: 'WebViewHost',
  fullName: 'web.runtime.view.WebViewHost',
  methods: {},
} as const

/** WebViewRenderer is exposed by the WebView renderer. */
export interface WebViewRenderer {
  /**
   * Echo returns the given message.
   * DEMO
   */
  Echo(request: EchoMsg): Promise<EchoMsg>
}

export class WebViewRendererClientImpl implements WebViewRenderer {
  private readonly rpc: Rpc
  constructor(rpc: Rpc) {
    this.rpc = rpc
    this.Echo = this.Echo.bind(this)
  }
  Echo(request: EchoMsg): Promise<EchoMsg> {
    const data = EchoMsg.encode(request).finish()
    const promise = this.rpc.request(
      'web.runtime.view.WebViewRenderer',
      'Echo',
      data
    )
    return promise.then((data) => EchoMsg.decode(new _m0.Reader(data)))
  }
}

/** WebViewRenderer is exposed by the WebView renderer. */
export type WebViewRendererDefinition = typeof WebViewRendererDefinition
export const WebViewRendererDefinition = {
  name: 'WebViewRenderer',
  fullName: 'web.runtime.view.WebViewRenderer',
  methods: {
    /**
     * Echo returns the given message.
     * DEMO
     */
    echo: {
      name: 'Echo',
      requestType: EchoMsg,
      requestStream: false,
      responseType: EchoMsg,
      responseStream: false,
      options: {},
    },
  },
} as const

interface Rpc {
  request(
    service: string,
    method: string,
    data: Uint8Array
  ): Promise<Uint8Array>
}

type Builtin =
  | Date
  | Function
  | Uint8Array
  | string
  | number
  | boolean
  | undefined

export type DeepPartial<T> = T extends Builtin
  ? T
  : T extends Long
  ? string | number | Long
  : T extends Array<infer U>
  ? Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U>
  ? ReadonlyArray<DeepPartial<U>>
  : T extends { $case: string }
  ? { [K in keyof Omit<T, '$case'>]?: DeepPartial<T[K]> } & {
      $case: T['$case']
    }
  : T extends {}
  ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>

type KeysOfUnion<T> = T extends T ? keyof T : never
export type Exact<P, I extends P> = P extends Builtin
  ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & Record<
        Exclude<keyof I, KeysOfUnion<P>>,
        never
      >

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any
  _m0.configure()
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
