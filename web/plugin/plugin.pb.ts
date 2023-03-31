/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'bldr.web.plugin'

/** HandleWebViewViaPluginRequest is a request to handle web views via a plugin RPC. */
export interface HandleWebViewViaPluginRequest {
  /** HandlePluginId is the plugin the web plugin should send WebViews to. */
  handlePluginId: string
  /**
   * WebViewidRegex is the regex of web view IDs to handle with handlePluginId.
   * If empty, will forward any.
   */
  webViewIdRegex: string
}

/** HandleWebViewViaPluginResponse is the response to HandleWebViewViaPlugin. */
export interface HandleWebViewViaPluginResponse {
  body?: { $case: 'ready'; ready: boolean }
}

function createBaseHandleWebViewViaPluginRequest(): HandleWebViewViaPluginRequest {
  return { handlePluginId: '', webViewIdRegex: '' }
}

export const HandleWebViewViaPluginRequest = {
  encode(
    message: HandleWebViewViaPluginRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.handlePluginId !== '') {
      writer.uint32(10).string(message.handlePluginId)
    }
    if (message.webViewIdRegex !== '') {
      writer.uint32(18).string(message.webViewIdRegex)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): HandleWebViewViaPluginRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseHandleWebViewViaPluginRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break
          }

          message.handlePluginId = reader.string()
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.webViewIdRegex = reader.string()
          continue
      }
      if ((tag & 7) == 4 || tag == 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<HandleWebViewViaPluginRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<
          HandleWebViewViaPluginRequest | HandleWebViewViaPluginRequest[]
        >
      | Iterable<
          HandleWebViewViaPluginRequest | HandleWebViewViaPluginRequest[]
        >
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HandleWebViewViaPluginRequest.encode(p).finish()]
        }
      } else {
        yield* [HandleWebViewViaPluginRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, HandleWebViewViaPluginRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<HandleWebViewViaPluginRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HandleWebViewViaPluginRequest.decode(p)]
        }
      } else {
        yield* [HandleWebViewViaPluginRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): HandleWebViewViaPluginRequest {
    return {
      handlePluginId: isSet(object.handlePluginId)
        ? String(object.handlePluginId)
        : '',
      webViewIdRegex: isSet(object.webViewIdRegex)
        ? String(object.webViewIdRegex)
        : '',
    }
  },

  toJSON(message: HandleWebViewViaPluginRequest): unknown {
    const obj: any = {}
    message.handlePluginId !== undefined &&
      (obj.handlePluginId = message.handlePluginId)
    message.webViewIdRegex !== undefined &&
      (obj.webViewIdRegex = message.webViewIdRegex)
    return obj
  },

  create<I extends Exact<DeepPartial<HandleWebViewViaPluginRequest>, I>>(
    base?: I
  ): HandleWebViewViaPluginRequest {
    return HandleWebViewViaPluginRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<HandleWebViewViaPluginRequest>, I>>(
    object: I
  ): HandleWebViewViaPluginRequest {
    const message = createBaseHandleWebViewViaPluginRequest()
    message.handlePluginId = object.handlePluginId ?? ''
    message.webViewIdRegex = object.webViewIdRegex ?? ''
    return message
  },
}

function createBaseHandleWebViewViaPluginResponse(): HandleWebViewViaPluginResponse {
  return { body: undefined }
}

export const HandleWebViewViaPluginResponse = {
  encode(
    message: HandleWebViewViaPluginResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    switch (message.body?.$case) {
      case 'ready':
        writer.uint32(8).bool(message.body.ready)
        break
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): HandleWebViewViaPluginResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseHandleWebViewViaPluginResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag != 8) {
            break
          }

          message.body = { $case: 'ready', ready: reader.bool() }
          continue
      }
      if ((tag & 7) == 4 || tag == 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<HandleWebViewViaPluginResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<
          HandleWebViewViaPluginResponse | HandleWebViewViaPluginResponse[]
        >
      | Iterable<
          HandleWebViewViaPluginResponse | HandleWebViewViaPluginResponse[]
        >
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HandleWebViewViaPluginResponse.encode(p).finish()]
        }
      } else {
        yield* [HandleWebViewViaPluginResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, HandleWebViewViaPluginResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<HandleWebViewViaPluginResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HandleWebViewViaPluginResponse.decode(p)]
        }
      } else {
        yield* [HandleWebViewViaPluginResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): HandleWebViewViaPluginResponse {
    return {
      body: isSet(object.ready)
        ? { $case: 'ready', ready: Boolean(object.ready) }
        : undefined,
    }
  },

  toJSON(message: HandleWebViewViaPluginResponse): unknown {
    const obj: any = {}
    message.body?.$case === 'ready' && (obj.ready = message.body?.ready)
    return obj
  },

  create<I extends Exact<DeepPartial<HandleWebViewViaPluginResponse>, I>>(
    base?: I
  ): HandleWebViewViaPluginResponse {
    return HandleWebViewViaPluginResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<HandleWebViewViaPluginResponse>, I>>(
    object: I
  ): HandleWebViewViaPluginResponse {
    const message = createBaseHandleWebViewViaPluginResponse()
    if (
      object.body?.$case === 'ready' &&
      object.body?.ready !== undefined &&
      object.body?.ready !== null
    ) {
      message.body = { $case: 'ready', ready: object.body.ready }
    }
    return message
  },
}

/** WebPlugin implements the bldr web plugin service. */
export interface WebPlugin {
  /** HandleWebViewViaPlugin configures handling web views via a plugin RPC. */
  HandleWebViewViaPlugin(
    request: HandleWebViewViaPluginRequest,
    abortSignal?: AbortSignal
  ): AsyncIterable<HandleWebViewViaPluginResponse>
}

export class WebPluginClientImpl implements WebPlugin {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || 'bldr.web.plugin.WebPlugin'
    this.rpc = rpc
    this.HandleWebViewViaPlugin = this.HandleWebViewViaPlugin.bind(this)
  }
  HandleWebViewViaPlugin(
    request: HandleWebViewViaPluginRequest,
    abortSignal?: AbortSignal
  ): AsyncIterable<HandleWebViewViaPluginResponse> {
    const data = HandleWebViewViaPluginRequest.encode(request).finish()
    const result = this.rpc.serverStreamingRequest(
      this.service,
      'HandleWebViewViaPlugin',
      data,
      abortSignal || undefined
    )
    return HandleWebViewViaPluginResponse.decodeTransform(result)
  }
}

/** WebPlugin implements the bldr web plugin service. */
export type WebPluginDefinition = typeof WebPluginDefinition
export const WebPluginDefinition = {
  name: 'WebPlugin',
  fullName: 'bldr.web.plugin.WebPlugin',
  methods: {
    /** HandleWebViewViaPlugin configures handling web views via a plugin RPC. */
    handleWebViewViaPlugin: {
      name: 'HandleWebViewViaPlugin',
      requestType: HandleWebViewViaPluginRequest,
      requestStream: false,
      responseType: HandleWebViewViaPluginResponse,
      responseStream: true,
      options: { _unknownFields: {} },
    },
  },
} as const

interface Rpc {
  request(
    service: string,
    method: string,
    data: Uint8Array,
    abortSignal?: AbortSignal
  ): Promise<Uint8Array>
  clientStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>,
    abortSignal?: AbortSignal
  ): Promise<Uint8Array>
  serverStreamingRequest(
    service: string,
    method: string,
    data: Uint8Array,
    abortSignal?: AbortSignal
  ): AsyncIterable<Uint8Array>
  bidirectionalStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>,
    abortSignal?: AbortSignal
  ): AsyncIterable<Uint8Array>
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
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & {
      [K in Exclude<keyof I, KeysOfUnion<P>>]: never
    }

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any
  _m0.configure()
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
