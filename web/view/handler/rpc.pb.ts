/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'web.view.handler'

/** HandleWebViewRequest is a request to handle a web view. */
export interface HandleWebViewRequest {
  /** Id is the unique identifier for the webview. */
  id: string
  /**
   * ParentId is the identifier of the parent WebView.
   * May be empty.
   */
  parentId: string
  /**
   * DocumentId is the identifier of the parent WebDocument.
   * May be empty.
   */
  documentId: string
  /** Permanent indicates that this is a "root" webview and cannot be closed. */
  permanent: boolean
}

/** HandleWebViewResponse is a response to handle a web view. */
export interface HandleWebViewResponse {
  /**
   * Error contains any error handling the web view.
   * If empty, returns and does not retry.
   */
  error: string
}

function createBaseHandleWebViewRequest(): HandleWebViewRequest {
  return { id: '', parentId: '', documentId: '', permanent: false }
}

export const HandleWebViewRequest = {
  encode(
    message: HandleWebViewRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.id !== '') {
      writer.uint32(10).string(message.id)
    }
    if (message.parentId !== '') {
      writer.uint32(18).string(message.parentId)
    }
    if (message.documentId !== '') {
      writer.uint32(26).string(message.documentId)
    }
    if (message.permanent === true) {
      writer.uint32(32).bool(message.permanent)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): HandleWebViewRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseHandleWebViewRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.id = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.parentId = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.documentId = reader.string()
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.permanent = reader.bool()
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
  // Transform<HandleWebViewRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<HandleWebViewRequest | HandleWebViewRequest[]>
      | Iterable<HandleWebViewRequest | HandleWebViewRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HandleWebViewRequest.encode(p).finish()]
        }
      } else {
        yield* [HandleWebViewRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, HandleWebViewRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<HandleWebViewRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HandleWebViewRequest.decode(p)]
        }
      } else {
        yield* [HandleWebViewRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): HandleWebViewRequest {
    return {
      id: isSet(object.id) ? String(object.id) : '',
      parentId: isSet(object.parentId) ? String(object.parentId) : '',
      documentId: isSet(object.documentId) ? String(object.documentId) : '',
      permanent: isSet(object.permanent) ? Boolean(object.permanent) : false,
    }
  },

  toJSON(message: HandleWebViewRequest): unknown {
    const obj: any = {}
    message.id !== undefined && (obj.id = message.id)
    message.parentId !== undefined && (obj.parentId = message.parentId)
    message.documentId !== undefined && (obj.documentId = message.documentId)
    message.permanent !== undefined && (obj.permanent = message.permanent)
    return obj
  },

  create<I extends Exact<DeepPartial<HandleWebViewRequest>, I>>(
    base?: I
  ): HandleWebViewRequest {
    return HandleWebViewRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<HandleWebViewRequest>, I>>(
    object: I
  ): HandleWebViewRequest {
    const message = createBaseHandleWebViewRequest()
    message.id = object.id ?? ''
    message.parentId = object.parentId ?? ''
    message.documentId = object.documentId ?? ''
    message.permanent = object.permanent ?? false
    return message
  },
}

function createBaseHandleWebViewResponse(): HandleWebViewResponse {
  return { error: '' }
}

export const HandleWebViewResponse = {
  encode(
    message: HandleWebViewResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.error !== '') {
      writer.uint32(10).string(message.error)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): HandleWebViewResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseHandleWebViewResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.error = reader.string()
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
  // Transform<HandleWebViewResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<HandleWebViewResponse | HandleWebViewResponse[]>
      | Iterable<HandleWebViewResponse | HandleWebViewResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HandleWebViewResponse.encode(p).finish()]
        }
      } else {
        yield* [HandleWebViewResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, HandleWebViewResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<HandleWebViewResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HandleWebViewResponse.decode(p)]
        }
      } else {
        yield* [HandleWebViewResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): HandleWebViewResponse {
    return { error: isSet(object.error) ? String(object.error) : '' }
  },

  toJSON(message: HandleWebViewResponse): unknown {
    const obj: any = {}
    message.error !== undefined && (obj.error = message.error)
    return obj
  },

  create<I extends Exact<DeepPartial<HandleWebViewResponse>, I>>(
    base?: I
  ): HandleWebViewResponse {
    return HandleWebViewResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<HandleWebViewResponse>, I>>(
    object: I
  ): HandleWebViewResponse {
    const message = createBaseHandleWebViewResponse()
    message.error = object.error ?? ''
    return message
  },
}

/** HandleWebViewService implements the HandleWebView directive. */
export interface HandleWebViewService {
  /**
   * HandleWebView handles a web view via rpc.
   * The RPC will be held open while the handler runs.
   * The RPC is canceled if the WebView is removed.
   * The handler can access the WebView service via AccessWebViews.
   */
  HandleWebView(
    request: HandleWebViewRequest,
    abortSignal?: AbortSignal
  ): Promise<HandleWebViewResponse>
}

export const HandleWebViewServiceServiceName =
  'web.view.handler.HandleWebViewService'
export class HandleWebViewServiceClientImpl implements HandleWebViewService {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || HandleWebViewServiceServiceName
    this.rpc = rpc
    this.HandleWebView = this.HandleWebView.bind(this)
  }
  HandleWebView(
    request: HandleWebViewRequest,
    abortSignal?: AbortSignal
  ): Promise<HandleWebViewResponse> {
    const data = HandleWebViewRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'HandleWebView',
      data,
      abortSignal || undefined
    )
    return promise.then((data) =>
      HandleWebViewResponse.decode(_m0.Reader.create(data))
    )
  }
}

/** HandleWebViewService implements the HandleWebView directive. */
export type HandleWebViewServiceDefinition =
  typeof HandleWebViewServiceDefinition
export const HandleWebViewServiceDefinition = {
  name: 'HandleWebViewService',
  fullName: 'web.view.handler.HandleWebViewService',
  methods: {
    /**
     * HandleWebView handles a web view via rpc.
     * The RPC will be held open while the handler runs.
     * The RPC is canceled if the WebView is removed.
     * The handler can access the WebView service via AccessWebViews.
     */
    handleWebView: {
      name: 'HandleWebView',
      requestType: HandleWebViewRequest,
      requestStream: false,
      responseType: HandleWebViewResponse,
      responseStream: false,
      options: {},
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
