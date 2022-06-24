/* eslint-disable */
import { Observable } from 'rxjs'
import Long from 'long'
import { RpcStreamPacket } from '../../vendor/github.com/aperturerobotics/starpc/rpcstream/rpcstream.pb'
import { map } from 'rxjs/operators'
import * as _m0 from 'protobufjs/minimal'

export const protobufPackage = 'web.runtime'

/**
 * WebInitRuntime is a message to init the Runtime from the Web runtime.
 *
 * Sent to the WebWorker to initialize it.
 */
export interface WebInitRuntime {
  /**
   * RuntimeId is the shared identifier for the Go Runtime instance.
   *
   * must be set
   */
  runtimeId: string
  /** WebRuntimeUuid is the identifier of the starting Web runtime. */
  webRuntimeUuid: string
}

/** WatchWebStatusRequest is the body of the WatchWebStatus request. */
export interface WatchWebStatusRequest {}

/** WebStatus contains a snapshot of status for a Runtime instance. */
export interface WebStatus {
  /** Snapshot indicates this is a full snapshot of the lists. */
  snapshot: boolean
  /** WebViews contains the list of web views. */
  webViews: WebViewStatus[]
}

/**
 * WebViewStatus contains status for a web view.
 *
 * WebToRuntimeType_WEB_VIEW_STATUS
 */
export interface WebViewStatus {
  /**
   * Id is the unique identifier for the webview.
   * if !is_root, id is specified by the runtime when creating the WebView.
   */
  id: string
  /**
   * Deleted indicates the web view was just removed.
   * If set, all below fields are ignored.
   */
  deleted: boolean
  /** Permanent indicates that this is a "root" webview and cannot be closed. */
  permanent: boolean
}

/** CreateWebViewRequest is a request to create a new web view. */
export interface CreateWebViewRequest {
  /** id is the identifier for the new web view. */
  id: string
}

/** CreateWebViewResponse is the response to the CreateWebView request. */
export interface CreateWebViewResponse {
  /**
   * Created indicates the web view was created.
   * If this is not set, assumes we cannot create WebViews.
   */
  created: boolean
}

function createBaseWebInitRuntime(): WebInitRuntime {
  return { runtimeId: '', webRuntimeUuid: '' }
}

export const WebInitRuntime = {
  encode(
    message: WebInitRuntime,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.runtimeId !== '') {
      writer.uint32(10).string(message.runtimeId)
    }
    if (message.webRuntimeUuid !== '') {
      writer.uint32(18).string(message.webRuntimeUuid)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WebInitRuntime {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWebInitRuntime()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.runtimeId = reader.string()
          break
        case 2:
          message.webRuntimeUuid = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): WebInitRuntime {
    return {
      runtimeId: isSet(object.runtimeId) ? String(object.runtimeId) : '',
      webRuntimeUuid: isSet(object.webRuntimeUuid)
        ? String(object.webRuntimeUuid)
        : '',
    }
  },

  toJSON(message: WebInitRuntime): unknown {
    const obj: any = {}
    message.runtimeId !== undefined && (obj.runtimeId = message.runtimeId)
    message.webRuntimeUuid !== undefined &&
      (obj.webRuntimeUuid = message.webRuntimeUuid)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<WebInitRuntime>, I>>(
    object: I
  ): WebInitRuntime {
    const message = createBaseWebInitRuntime()
    message.runtimeId = object.runtimeId ?? ''
    message.webRuntimeUuid = object.webRuntimeUuid ?? ''
    return message
  },
}

function createBaseWatchWebStatusRequest(): WatchWebStatusRequest {
  return {}
}

export const WatchWebStatusRequest = {
  encode(
    _: WatchWebStatusRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): WatchWebStatusRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWatchWebStatusRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(_: any): WatchWebStatusRequest {
    return {}
  },

  toJSON(_: WatchWebStatusRequest): unknown {
    const obj: any = {}
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<WatchWebStatusRequest>, I>>(
    _: I
  ): WatchWebStatusRequest {
    const message = createBaseWatchWebStatusRequest()
    return message
  },
}

function createBaseWebStatus(): WebStatus {
  return { snapshot: false, webViews: [] }
}

export const WebStatus = {
  encode(
    message: WebStatus,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.snapshot === true) {
      writer.uint32(8).bool(message.snapshot)
    }
    for (const v of message.webViews) {
      WebViewStatus.encode(v!, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WebStatus {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWebStatus()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.snapshot = reader.bool()
          break
        case 2:
          message.webViews.push(WebViewStatus.decode(reader, reader.uint32()))
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): WebStatus {
    return {
      snapshot: isSet(object.snapshot) ? Boolean(object.snapshot) : false,
      webViews: Array.isArray(object?.webViews)
        ? object.webViews.map((e: any) => WebViewStatus.fromJSON(e))
        : [],
    }
  },

  toJSON(message: WebStatus): unknown {
    const obj: any = {}
    message.snapshot !== undefined && (obj.snapshot = message.snapshot)
    if (message.webViews) {
      obj.webViews = message.webViews.map((e) =>
        e ? WebViewStatus.toJSON(e) : undefined
      )
    } else {
      obj.webViews = []
    }
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<WebStatus>, I>>(
    object: I
  ): WebStatus {
    const message = createBaseWebStatus()
    message.snapshot = object.snapshot ?? false
    message.webViews =
      object.webViews?.map((e) => WebViewStatus.fromPartial(e)) || []
    return message
  },
}

function createBaseWebViewStatus(): WebViewStatus {
  return { id: '', deleted: false, permanent: false }
}

export const WebViewStatus = {
  encode(
    message: WebViewStatus,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.id !== '') {
      writer.uint32(10).string(message.id)
    }
    if (message.deleted === true) {
      writer.uint32(16).bool(message.deleted)
    }
    if (message.permanent === true) {
      writer.uint32(24).bool(message.permanent)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WebViewStatus {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWebViewStatus()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.id = reader.string()
          break
        case 2:
          message.deleted = reader.bool()
          break
        case 3:
          message.permanent = reader.bool()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): WebViewStatus {
    return {
      id: isSet(object.id) ? String(object.id) : '',
      deleted: isSet(object.deleted) ? Boolean(object.deleted) : false,
      permanent: isSet(object.permanent) ? Boolean(object.permanent) : false,
    }
  },

  toJSON(message: WebViewStatus): unknown {
    const obj: any = {}
    message.id !== undefined && (obj.id = message.id)
    message.deleted !== undefined && (obj.deleted = message.deleted)
    message.permanent !== undefined && (obj.permanent = message.permanent)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<WebViewStatus>, I>>(
    object: I
  ): WebViewStatus {
    const message = createBaseWebViewStatus()
    message.id = object.id ?? ''
    message.deleted = object.deleted ?? false
    message.permanent = object.permanent ?? false
    return message
  },
}

function createBaseCreateWebViewRequest(): CreateWebViewRequest {
  return { id: '' }
}

export const CreateWebViewRequest = {
  encode(
    message: CreateWebViewRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.id !== '') {
      writer.uint32(10).string(message.id)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): CreateWebViewRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseCreateWebViewRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.id = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): CreateWebViewRequest {
    return {
      id: isSet(object.id) ? String(object.id) : '',
    }
  },

  toJSON(message: CreateWebViewRequest): unknown {
    const obj: any = {}
    message.id !== undefined && (obj.id = message.id)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<CreateWebViewRequest>, I>>(
    object: I
  ): CreateWebViewRequest {
    const message = createBaseCreateWebViewRequest()
    message.id = object.id ?? ''
    return message
  },
}

function createBaseCreateWebViewResponse(): CreateWebViewResponse {
  return { created: false }
}

export const CreateWebViewResponse = {
  encode(
    message: CreateWebViewResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.created === true) {
      writer.uint32(8).bool(message.created)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): CreateWebViewResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseCreateWebViewResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.created = reader.bool()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): CreateWebViewResponse {
    return {
      created: isSet(object.created) ? Boolean(object.created) : false,
    }
  },

  toJSON(message: CreateWebViewResponse): unknown {
    const obj: any = {}
    message.created !== undefined && (obj.created = message.created)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<CreateWebViewResponse>, I>>(
    object: I
  ): CreateWebViewResponse {
    const message = createBaseCreateWebViewResponse()
    message.created = object.created ?? false
    return message
  },
}

/** WebRuntime is the API exposed by the TypeScript Runtime. */
export interface WebRuntime {
  /** WatchWebStatus returns an initial snapshot of web views followed by updates. */
  WatchWebStatus(request: WatchWebStatusRequest): Observable<WebStatus>
  /**
   * CreateWebView requests to create a new WebView at the root level.
   * Returns created: false if unable to create WebViews.
   */
  CreateWebView(request: CreateWebViewRequest): Promise<CreateWebViewResponse>
  /** WebViewRpc opens a stream for a RPC call to a WebView. */
  WebViewRpc(request: Observable<RpcStreamPacket>): Observable<RpcStreamPacket>
}

export class WebRuntimeClientImpl implements WebRuntime {
  private readonly rpc: Rpc
  constructor(rpc: Rpc) {
    this.rpc = rpc
    this.WatchWebStatus = this.WatchWebStatus.bind(this)
    this.CreateWebView = this.CreateWebView.bind(this)
    this.WebViewRpc = this.WebViewRpc.bind(this)
  }
  WatchWebStatus(request: WatchWebStatusRequest): Observable<WebStatus> {
    const data = WatchWebStatusRequest.encode(request).finish()
    const result = this.rpc.serverStreamingRequest(
      'web.runtime.WebRuntime',
      'WatchWebStatus',
      data
    )
    return result.pipe(map((data) => WebStatus.decode(new _m0.Reader(data))))
  }

  CreateWebView(request: CreateWebViewRequest): Promise<CreateWebViewResponse> {
    const data = CreateWebViewRequest.encode(request).finish()
    const promise = this.rpc.request(
      'web.runtime.WebRuntime',
      'CreateWebView',
      data
    )
    return promise.then((data) =>
      CreateWebViewResponse.decode(new _m0.Reader(data))
    )
  }

  WebViewRpc(
    request: Observable<RpcStreamPacket>
  ): Observable<RpcStreamPacket> {
    const data = request.pipe(
      map((request) => RpcStreamPacket.encode(request).finish())
    )
    const result = this.rpc.bidirectionalStreamingRequest(
      'web.runtime.WebRuntime',
      'WebViewRpc',
      data
    )
    return result.pipe(
      map((data) => RpcStreamPacket.decode(new _m0.Reader(data)))
    )
  }
}

/** WebRuntime is the API exposed by the TypeScript Runtime. */
export type WebRuntimeDefinition = typeof WebRuntimeDefinition
export const WebRuntimeDefinition = {
  name: 'WebRuntime',
  fullName: 'web.runtime.WebRuntime',
  methods: {
    /** WatchWebStatus returns an initial snapshot of web views followed by updates. */
    watchWebStatus: {
      name: 'WatchWebStatus',
      requestType: WatchWebStatusRequest,
      requestStream: false,
      responseType: WebStatus,
      responseStream: true,
      options: {},
    },
    /**
     * CreateWebView requests to create a new WebView at the root level.
     * Returns created: false if unable to create WebViews.
     */
    createWebView: {
      name: 'CreateWebView',
      requestType: CreateWebViewRequest,
      requestStream: false,
      responseType: CreateWebViewResponse,
      responseStream: false,
      options: {},
    },
    /** WebViewRpc opens a stream for a RPC call to a WebView. */
    webViewRpc: {
      name: 'WebViewRpc',
      requestType: RpcStreamPacket,
      requestStream: true,
      responseType: RpcStreamPacket,
      responseStream: true,
      options: {},
    },
  },
} as const

/** HostRuntime is the API exposed by the Go Runtime. */
export interface HostRuntime {
  /** ServiceWorkerRpc opens a stream for a RPC call from the ServiceWorker. */
  ServiceWorkerRpc(
    request: Observable<RpcStreamPacket>
  ): Observable<RpcStreamPacket>
  /** WebViewRpc opens a stream for a RPC call from a WebView. */
  WebViewRpc(request: Observable<RpcStreamPacket>): Observable<RpcStreamPacket>
}

export class HostRuntimeClientImpl implements HostRuntime {
  private readonly rpc: Rpc
  constructor(rpc: Rpc) {
    this.rpc = rpc
    this.ServiceWorkerRpc = this.ServiceWorkerRpc.bind(this)
    this.WebViewRpc = this.WebViewRpc.bind(this)
  }
  ServiceWorkerRpc(
    request: Observable<RpcStreamPacket>
  ): Observable<RpcStreamPacket> {
    const data = request.pipe(
      map((request) => RpcStreamPacket.encode(request).finish())
    )
    const result = this.rpc.bidirectionalStreamingRequest(
      'web.runtime.HostRuntime',
      'ServiceWorkerRpc',
      data
    )
    return result.pipe(
      map((data) => RpcStreamPacket.decode(new _m0.Reader(data)))
    )
  }

  WebViewRpc(
    request: Observable<RpcStreamPacket>
  ): Observable<RpcStreamPacket> {
    const data = request.pipe(
      map((request) => RpcStreamPacket.encode(request).finish())
    )
    const result = this.rpc.bidirectionalStreamingRequest(
      'web.runtime.HostRuntime',
      'WebViewRpc',
      data
    )
    return result.pipe(
      map((data) => RpcStreamPacket.decode(new _m0.Reader(data)))
    )
  }
}

/** HostRuntime is the API exposed by the Go Runtime. */
export type HostRuntimeDefinition = typeof HostRuntimeDefinition
export const HostRuntimeDefinition = {
  name: 'HostRuntime',
  fullName: 'web.runtime.HostRuntime',
  methods: {
    /** ServiceWorkerRpc opens a stream for a RPC call from the ServiceWorker. */
    serviceWorkerRpc: {
      name: 'ServiceWorkerRpc',
      requestType: RpcStreamPacket,
      requestStream: true,
      responseType: RpcStreamPacket,
      responseStream: true,
      options: {},
    },
    /** WebViewRpc opens a stream for a RPC call from a WebView. */
    webViewRpc: {
      name: 'WebViewRpc',
      requestType: RpcStreamPacket,
      requestStream: true,
      responseType: RpcStreamPacket,
      responseStream: true,
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
  clientStreamingRequest(
    service: string,
    method: string,
    data: Observable<Uint8Array>
  ): Promise<Uint8Array>
  serverStreamingRequest(
    service: string,
    method: string,
    data: Uint8Array
  ): Observable<Uint8Array>
  bidirectionalStreamingRequest(
    service: string,
    method: string,
    data: Observable<Uint8Array>
  ): Observable<Uint8Array>
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
