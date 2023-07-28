/* eslint-disable */
import { Backoff } from '@go/github.com/aperturerobotics/util/backoff/backoff.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'bldr.web.plugin'

/** HandleWebViewViaPluginRequest is a request to handle web views via a plugin RPC. */
export interface HandleWebViewViaPluginRequest {
  /** HandlePluginId is the plugin the web plugin should send WebViews to. */
  handlePluginId: string
  /**
   * WebViewidRe is the regex of web view IDs to handle with handlePluginId.
   * If empty, will forward any.
   */
  webViewIdRe: string
}

/** HandleWebViewViaPluginResponse is the response to HandleWebViewViaPlugin. */
export interface HandleWebViewViaPluginResponse {
  body?: { $case: 'ready'; ready: boolean } | undefined
}

/** HandleRpcViaPluginRequest is a request to handle web views via a plugin RPC. */
export interface HandleRpcViaPluginRequest {
  /** HandlePluginId is the plugin the web plugin should send Rpcs to. */
  handlePluginId: string
  /**
   * ServiceIdRe is the regex of service IDs to forward.
   * If empty, will forward any.
   */
  serviceIdRe: string
  /**
   * ServerIdRe is the regex of server IDs to forward for.
   * If empty, will forward any.
   */
  serverIdRe: string
  /**
   * Backoff is the backoff config for calling the RPC service.
   * If unset, defaults to reasonable defaults.
   */
  backoff: Backoff | undefined
}

/** HandleRpcViaPluginResponse is the response to HandleRpcViaPlugin. */
export interface HandleRpcViaPluginResponse {
  body?: { $case: 'ready'; ready: boolean } | undefined
}

function createBaseHandleWebViewViaPluginRequest(): HandleWebViewViaPluginRequest {
  return { handlePluginId: '', webViewIdRe: '' }
}

export const HandleWebViewViaPluginRequest = {
  encode(
    message: HandleWebViewViaPluginRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.handlePluginId !== '') {
      writer.uint32(10).string(message.handlePluginId)
    }
    if (message.webViewIdRe !== '') {
      writer.uint32(18).string(message.webViewIdRe)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): HandleWebViewViaPluginRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseHandleWebViewViaPluginRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.handlePluginId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.webViewIdRe = reader.string()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
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
        >,
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
      | Iterable<Uint8Array | Uint8Array[]>,
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
      webViewIdRe: isSet(object.webViewIdRe) ? String(object.webViewIdRe) : '',
    }
  },

  toJSON(message: HandleWebViewViaPluginRequest): unknown {
    const obj: any = {}
    if (message.handlePluginId !== '') {
      obj.handlePluginId = message.handlePluginId
    }
    if (message.webViewIdRe !== '') {
      obj.webViewIdRe = message.webViewIdRe
    }
    return obj
  },

  create<I extends Exact<DeepPartial<HandleWebViewViaPluginRequest>, I>>(
    base?: I,
  ): HandleWebViewViaPluginRequest {
    return HandleWebViewViaPluginRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<HandleWebViewViaPluginRequest>, I>>(
    object: I,
  ): HandleWebViewViaPluginRequest {
    const message = createBaseHandleWebViewViaPluginRequest()
    message.handlePluginId = object.handlePluginId ?? ''
    message.webViewIdRe = object.webViewIdRe ?? ''
    return message
  },
}

function createBaseHandleWebViewViaPluginResponse(): HandleWebViewViaPluginResponse {
  return { body: undefined }
}

export const HandleWebViewViaPluginResponse = {
  encode(
    message: HandleWebViewViaPluginResponse,
    writer: _m0.Writer = _m0.Writer.create(),
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
    length?: number,
  ): HandleWebViewViaPluginResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseHandleWebViewViaPluginResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.body = { $case: 'ready', ready: reader.bool() }
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
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
        >,
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
      | Iterable<Uint8Array | Uint8Array[]>,
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
    if (message.body?.$case === 'ready') {
      obj.ready = message.body.ready
    }
    return obj
  },

  create<I extends Exact<DeepPartial<HandleWebViewViaPluginResponse>, I>>(
    base?: I,
  ): HandleWebViewViaPluginResponse {
    return HandleWebViewViaPluginResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<HandleWebViewViaPluginResponse>, I>>(
    object: I,
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

function createBaseHandleRpcViaPluginRequest(): HandleRpcViaPluginRequest {
  return {
    handlePluginId: '',
    serviceIdRe: '',
    serverIdRe: '',
    backoff: undefined,
  }
}

export const HandleRpcViaPluginRequest = {
  encode(
    message: HandleRpcViaPluginRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.handlePluginId !== '') {
      writer.uint32(10).string(message.handlePluginId)
    }
    if (message.serviceIdRe !== '') {
      writer.uint32(18).string(message.serviceIdRe)
    }
    if (message.serverIdRe !== '') {
      writer.uint32(26).string(message.serverIdRe)
    }
    if (message.backoff !== undefined) {
      Backoff.encode(message.backoff, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): HandleRpcViaPluginRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseHandleRpcViaPluginRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.handlePluginId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.serviceIdRe = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.serverIdRe = reader.string()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.backoff = Backoff.decode(reader, reader.uint32())
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<HandleRpcViaPluginRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<HandleRpcViaPluginRequest | HandleRpcViaPluginRequest[]>
      | Iterable<HandleRpcViaPluginRequest | HandleRpcViaPluginRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HandleRpcViaPluginRequest.encode(p).finish()]
        }
      } else {
        yield* [HandleRpcViaPluginRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, HandleRpcViaPluginRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<HandleRpcViaPluginRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HandleRpcViaPluginRequest.decode(p)]
        }
      } else {
        yield* [HandleRpcViaPluginRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): HandleRpcViaPluginRequest {
    return {
      handlePluginId: isSet(object.handlePluginId)
        ? String(object.handlePluginId)
        : '',
      serviceIdRe: isSet(object.serviceIdRe) ? String(object.serviceIdRe) : '',
      serverIdRe: isSet(object.serverIdRe) ? String(object.serverIdRe) : '',
      backoff: isSet(object.backoff)
        ? Backoff.fromJSON(object.backoff)
        : undefined,
    }
  },

  toJSON(message: HandleRpcViaPluginRequest): unknown {
    const obj: any = {}
    if (message.handlePluginId !== '') {
      obj.handlePluginId = message.handlePluginId
    }
    if (message.serviceIdRe !== '') {
      obj.serviceIdRe = message.serviceIdRe
    }
    if (message.serverIdRe !== '') {
      obj.serverIdRe = message.serverIdRe
    }
    if (message.backoff !== undefined) {
      obj.backoff = Backoff.toJSON(message.backoff)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<HandleRpcViaPluginRequest>, I>>(
    base?: I,
  ): HandleRpcViaPluginRequest {
    return HandleRpcViaPluginRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<HandleRpcViaPluginRequest>, I>>(
    object: I,
  ): HandleRpcViaPluginRequest {
    const message = createBaseHandleRpcViaPluginRequest()
    message.handlePluginId = object.handlePluginId ?? ''
    message.serviceIdRe = object.serviceIdRe ?? ''
    message.serverIdRe = object.serverIdRe ?? ''
    message.backoff =
      object.backoff !== undefined && object.backoff !== null
        ? Backoff.fromPartial(object.backoff)
        : undefined
    return message
  },
}

function createBaseHandleRpcViaPluginResponse(): HandleRpcViaPluginResponse {
  return { body: undefined }
}

export const HandleRpcViaPluginResponse = {
  encode(
    message: HandleRpcViaPluginResponse,
    writer: _m0.Writer = _m0.Writer.create(),
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
    length?: number,
  ): HandleRpcViaPluginResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseHandleRpcViaPluginResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.body = { $case: 'ready', ready: reader.bool() }
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<HandleRpcViaPluginResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<HandleRpcViaPluginResponse | HandleRpcViaPluginResponse[]>
      | Iterable<HandleRpcViaPluginResponse | HandleRpcViaPluginResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HandleRpcViaPluginResponse.encode(p).finish()]
        }
      } else {
        yield* [HandleRpcViaPluginResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, HandleRpcViaPluginResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<HandleRpcViaPluginResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HandleRpcViaPluginResponse.decode(p)]
        }
      } else {
        yield* [HandleRpcViaPluginResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): HandleRpcViaPluginResponse {
    return {
      body: isSet(object.ready)
        ? { $case: 'ready', ready: Boolean(object.ready) }
        : undefined,
    }
  },

  toJSON(message: HandleRpcViaPluginResponse): unknown {
    const obj: any = {}
    if (message.body?.$case === 'ready') {
      obj.ready = message.body.ready
    }
    return obj
  },

  create<I extends Exact<DeepPartial<HandleRpcViaPluginResponse>, I>>(
    base?: I,
  ): HandleRpcViaPluginResponse {
    return HandleRpcViaPluginResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<HandleRpcViaPluginResponse>, I>>(
    object: I,
  ): HandleRpcViaPluginResponse {
    const message = createBaseHandleRpcViaPluginResponse()
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
  /** HandleWebViewViaPlugin configures handling web views via a plugin. */
  HandleWebViewViaPlugin(
    request: HandleWebViewViaPluginRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<HandleWebViewViaPluginResponse>
  /** HandleRpcViaPlugin configures handling rpcs via a plugin. */
  HandleRpcViaPlugin(
    request: HandleRpcViaPluginRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<HandleRpcViaPluginResponse>
}

export const WebPluginServiceName = 'bldr.web.plugin.WebPlugin'
export class WebPluginClientImpl implements WebPlugin {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || WebPluginServiceName
    this.rpc = rpc
    this.HandleWebViewViaPlugin = this.HandleWebViewViaPlugin.bind(this)
    this.HandleRpcViaPlugin = this.HandleRpcViaPlugin.bind(this)
  }
  HandleWebViewViaPlugin(
    request: HandleWebViewViaPluginRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<HandleWebViewViaPluginResponse> {
    const data = HandleWebViewViaPluginRequest.encode(request).finish()
    const result = this.rpc.serverStreamingRequest(
      this.service,
      'HandleWebViewViaPlugin',
      data,
      abortSignal || undefined,
    )
    return HandleWebViewViaPluginResponse.decodeTransform(result)
  }

  HandleRpcViaPlugin(
    request: HandleRpcViaPluginRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<HandleRpcViaPluginResponse> {
    const data = HandleRpcViaPluginRequest.encode(request).finish()
    const result = this.rpc.serverStreamingRequest(
      this.service,
      'HandleRpcViaPlugin',
      data,
      abortSignal || undefined,
    )
    return HandleRpcViaPluginResponse.decodeTransform(result)
  }
}

/** WebPlugin implements the bldr web plugin service. */
export type WebPluginDefinition = typeof WebPluginDefinition
export const WebPluginDefinition = {
  name: 'WebPlugin',
  fullName: 'bldr.web.plugin.WebPlugin',
  methods: {
    /** HandleWebViewViaPlugin configures handling web views via a plugin. */
    handleWebViewViaPlugin: {
      name: 'HandleWebViewViaPlugin',
      requestType: HandleWebViewViaPluginRequest,
      requestStream: false,
      responseType: HandleWebViewViaPluginResponse,
      responseStream: true,
      options: {},
    },
    /** HandleRpcViaPlugin configures handling rpcs via a plugin. */
    handleRpcViaPlugin: {
      name: 'HandleRpcViaPlugin',
      requestType: HandleRpcViaPluginRequest,
      requestStream: false,
      responseType: HandleRpcViaPluginResponse,
      responseStream: true,
      options: {},
    },
  },
} as const

interface Rpc {
  request(
    service: string,
    method: string,
    data: Uint8Array,
    abortSignal?: AbortSignal,
  ): Promise<Uint8Array>
  clientStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>,
    abortSignal?: AbortSignal,
  ): Promise<Uint8Array>
  serverStreamingRequest(
    service: string,
    method: string,
    data: Uint8Array,
    abortSignal?: AbortSignal,
  ): AsyncIterable<Uint8Array>
  bidirectionalStreamingRequest(
    service: string,
    method: string,
    data: AsyncIterable<Uint8Array>,
    abortSignal?: AbortSignal,
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
