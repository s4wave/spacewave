/* eslint-disable */
import * as Long from 'long'
import * as _m0 from 'protobufjs/minimal'

export const protobufPackage = 'web.runtime'

/** RuntimeToWebType is the set of sync message types */
export enum RuntimeToWebType {
  RuntimeToWebType_UNKNOWN = 0,
  /** RuntimeToWebType_QUERY_STATUS - RuntimeToWebType_QUERY_STATUS queries the web runtime status. */
  RuntimeToWebType_QUERY_STATUS = 1,
  /** RuntimeToWebType_CREATE_VIEW - RuntimeToWebType_CREATE_VIEW requests to create a new web view. */
  RuntimeToWebType_CREATE_VIEW = 2,
  UNRECOGNIZED = -1,
}

export function runtimeToWebTypeFromJSON(object: any): RuntimeToWebType {
  switch (object) {
    case 0:
    case 'RuntimeToWebType_UNKNOWN':
      return RuntimeToWebType.RuntimeToWebType_UNKNOWN
    case 1:
    case 'RuntimeToWebType_QUERY_STATUS':
      return RuntimeToWebType.RuntimeToWebType_QUERY_STATUS
    case 2:
    case 'RuntimeToWebType_CREATE_VIEW':
      return RuntimeToWebType.RuntimeToWebType_CREATE_VIEW
    case -1:
    case 'UNRECOGNIZED':
    default:
      return RuntimeToWebType.UNRECOGNIZED
  }
}

export function runtimeToWebTypeToJSON(object: RuntimeToWebType): string {
  switch (object) {
    case RuntimeToWebType.RuntimeToWebType_UNKNOWN:
      return 'RuntimeToWebType_UNKNOWN'
    case RuntimeToWebType.RuntimeToWebType_QUERY_STATUS:
      return 'RuntimeToWebType_QUERY_STATUS'
    case RuntimeToWebType.RuntimeToWebType_CREATE_VIEW:
      return 'RuntimeToWebType_CREATE_VIEW'
    default:
      return 'UNKNOWN'
  }
}

/** WebToRuntimeType is the set of messages to the runtime from the web Runtime. */
export enum WebToRuntimeType {
  WebToRuntimeType_UNKNOWN = 0,
  /** WebToRuntimeType_STATUS - WebToRuntimeType_STATUS is a full status report. */
  WebToRuntimeType_STATUS = 1,
  UNRECOGNIZED = -1,
}

export function webToRuntimeTypeFromJSON(object: any): WebToRuntimeType {
  switch (object) {
    case 0:
    case 'WebToRuntimeType_UNKNOWN':
      return WebToRuntimeType.WebToRuntimeType_UNKNOWN
    case 1:
    case 'WebToRuntimeType_STATUS':
      return WebToRuntimeType.WebToRuntimeType_STATUS
    case -1:
    case 'UNRECOGNIZED':
    default:
      return WebToRuntimeType.UNRECOGNIZED
  }
}

export function webToRuntimeTypeToJSON(object: WebToRuntimeType): string {
  switch (object) {
    case WebToRuntimeType.WebToRuntimeType_UNKNOWN:
      return 'WebToRuntimeType_UNKNOWN'
    case WebToRuntimeType.WebToRuntimeType_STATUS:
      return 'WebToRuntimeType_STATUS'
    default:
      return 'UNKNOWN'
  }
}

/**
 * WebInitRuntime is a message to init the Runtime from the Web runtime.
 *
 * Sent to the WebWorker to initialize it.
 */
export interface WebInitRuntime {
  /**
   * RuntimeId the ID to use for the runtime instance.
   *
   * must be set
   * used to determine the broadcast channel ids
   */
  runtimeId: string
}

/** RuntimeToWeb are messages sent to the Web runtime from the Go runtime. */
export interface RuntimeToWeb {
  messageType: RuntimeToWebType
  /** CreateView is the body of the CREATE_VIEW message. */
  createView: CreateView | undefined
  /** QueryWebStatus is the body of the QUERY_VIEW_STATUS message. */
  queryViewStatus: QueryWebStatus | undefined
}

/** WebToRuntime are messages sent to the Runtime from the WebView. */
export interface WebToRuntime {
  messageType: WebToRuntimeType
  /** WebStatus is the body of the VIEW_STATUS message. */
  webStatus: WebStatus | undefined
}

/** CreateView is a message to create a new WebView. */
export interface CreateView {
  /** Id is the unique identifier for the new WebView. */
  id: string
}

/** QueryWebStatus is the body for QUERY_STATUS. */
export interface QueryWebStatus {}

/**
 * WebStatus is a web-view status report to the runtime.
 *
 * sent when the WebView starts up and/or is prompted
 */
export interface WebStatus {
  /** WebViews contains the list of web views. */
  webViews: WebViewStatus[]
}

/** WebViewStatus contains status for a web view. */
export interface WebViewStatus {
  /**
   * Id is the unique identifier for the webview.
   * if !is_root, id is specified by the runtime when creating the WebView.
   */
  id: string
  /** Permanent indicates that this is a "root" webview and cannot be closed. */
  permanent: boolean
}

function createBaseWebInitRuntime(): WebInitRuntime {
  return { runtimeId: '' }
}

export const WebInitRuntime = {
  encode(
    message: WebInitRuntime,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.runtimeId !== '') {
      writer.uint32(10).string(message.runtimeId)
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
    }
  },

  toJSON(message: WebInitRuntime): unknown {
    const obj: any = {}
    message.runtimeId !== undefined && (obj.runtimeId = message.runtimeId)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<WebInitRuntime>, I>>(
    object: I
  ): WebInitRuntime {
    const message = createBaseWebInitRuntime()
    message.runtimeId = object.runtimeId ?? ''
    return message
  },
}

function createBaseRuntimeToWeb(): RuntimeToWeb {
  return { messageType: 0, createView: undefined, queryViewStatus: undefined }
}

export const RuntimeToWeb = {
  encode(
    message: RuntimeToWeb,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.messageType !== 0) {
      writer.uint32(8).int32(message.messageType)
    }
    if (message.createView !== undefined) {
      CreateView.encode(message.createView, writer.uint32(18).fork()).ldelim()
    }
    if (message.queryViewStatus !== undefined) {
      QueryWebStatus.encode(
        message.queryViewStatus,
        writer.uint32(26).fork()
      ).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RuntimeToWeb {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRuntimeToWeb()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.messageType = reader.int32() as any
          break
        case 2:
          message.createView = CreateView.decode(reader, reader.uint32())
          break
        case 3:
          message.queryViewStatus = QueryWebStatus.decode(
            reader,
            reader.uint32()
          )
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): RuntimeToWeb {
    return {
      messageType: isSet(object.messageType)
        ? runtimeToWebTypeFromJSON(object.messageType)
        : 0,
      createView: isSet(object.createView)
        ? CreateView.fromJSON(object.createView)
        : undefined,
      queryViewStatus: isSet(object.queryViewStatus)
        ? QueryWebStatus.fromJSON(object.queryViewStatus)
        : undefined,
    }
  },

  toJSON(message: RuntimeToWeb): unknown {
    const obj: any = {}
    message.messageType !== undefined &&
      (obj.messageType = runtimeToWebTypeToJSON(message.messageType))
    message.createView !== undefined &&
      (obj.createView = message.createView
        ? CreateView.toJSON(message.createView)
        : undefined)
    message.queryViewStatus !== undefined &&
      (obj.queryViewStatus = message.queryViewStatus
        ? QueryWebStatus.toJSON(message.queryViewStatus)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<RuntimeToWeb>, I>>(
    object: I
  ): RuntimeToWeb {
    const message = createBaseRuntimeToWeb()
    message.messageType = object.messageType ?? 0
    message.createView =
      object.createView !== undefined && object.createView !== null
        ? CreateView.fromPartial(object.createView)
        : undefined
    message.queryViewStatus =
      object.queryViewStatus !== undefined && object.queryViewStatus !== null
        ? QueryWebStatus.fromPartial(object.queryViewStatus)
        : undefined
    return message
  },
}

function createBaseWebToRuntime(): WebToRuntime {
  return { messageType: 0, webStatus: undefined }
}

export const WebToRuntime = {
  encode(
    message: WebToRuntime,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.messageType !== 0) {
      writer.uint32(8).int32(message.messageType)
    }
    if (message.webStatus !== undefined) {
      WebStatus.encode(message.webStatus, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WebToRuntime {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWebToRuntime()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.messageType = reader.int32() as any
          break
        case 2:
          message.webStatus = WebStatus.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): WebToRuntime {
    return {
      messageType: isSet(object.messageType)
        ? webToRuntimeTypeFromJSON(object.messageType)
        : 0,
      webStatus: isSet(object.webStatus)
        ? WebStatus.fromJSON(object.webStatus)
        : undefined,
    }
  },

  toJSON(message: WebToRuntime): unknown {
    const obj: any = {}
    message.messageType !== undefined &&
      (obj.messageType = webToRuntimeTypeToJSON(message.messageType))
    message.webStatus !== undefined &&
      (obj.webStatus = message.webStatus
        ? WebStatus.toJSON(message.webStatus)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<WebToRuntime>, I>>(
    object: I
  ): WebToRuntime {
    const message = createBaseWebToRuntime()
    message.messageType = object.messageType ?? 0
    message.webStatus =
      object.webStatus !== undefined && object.webStatus !== null
        ? WebStatus.fromPartial(object.webStatus)
        : undefined
    return message
  },
}

function createBaseCreateView(): CreateView {
  return { id: '' }
}

export const CreateView = {
  encode(
    message: CreateView,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.id !== '') {
      writer.uint32(10).string(message.id)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): CreateView {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseCreateView()
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

  fromJSON(object: any): CreateView {
    return {
      id: isSet(object.id) ? String(object.id) : '',
    }
  },

  toJSON(message: CreateView): unknown {
    const obj: any = {}
    message.id !== undefined && (obj.id = message.id)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<CreateView>, I>>(
    object: I
  ): CreateView {
    const message = createBaseCreateView()
    message.id = object.id ?? ''
    return message
  },
}

function createBaseQueryWebStatus(): QueryWebStatus {
  return {}
}

export const QueryWebStatus = {
  encode(
    _: QueryWebStatus,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): QueryWebStatus {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseQueryWebStatus()
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

  fromJSON(_: any): QueryWebStatus {
    return {}
  },

  toJSON(_: QueryWebStatus): unknown {
    const obj: any = {}
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<QueryWebStatus>, I>>(
    _: I
  ): QueryWebStatus {
    const message = createBaseQueryWebStatus()
    return message
  },
}

function createBaseWebStatus(): WebStatus {
  return { webViews: [] }
}

export const WebStatus = {
  encode(
    message: WebStatus,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    for (const v of message.webViews) {
      WebViewStatus.encode(v!, writer.uint32(10).fork()).ldelim()
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
      webViews: Array.isArray(object?.webViews)
        ? object.webViews.map((e: any) => WebViewStatus.fromJSON(e))
        : [],
    }
  },

  toJSON(message: WebStatus): unknown {
    const obj: any = {}
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
    message.webViews =
      object.webViews?.map((e) => WebViewStatus.fromPartial(e)) || []
    return message
  },
}

function createBaseWebViewStatus(): WebViewStatus {
  return { id: '', permanent: false }
}

export const WebViewStatus = {
  encode(
    message: WebViewStatus,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.id !== '') {
      writer.uint32(10).string(message.id)
    }
    if (message.permanent === true) {
      writer.uint32(16).bool(message.permanent)
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
      permanent: isSet(object.permanent) ? Boolean(object.permanent) : false,
    }
  },

  toJSON(message: WebViewStatus): unknown {
    const obj: any = {}
    message.id !== undefined && (obj.id = message.id)
    message.permanent !== undefined && (obj.permanent = message.permanent)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<WebViewStatus>, I>>(
    object: I
  ): WebViewStatus {
    const message = createBaseWebViewStatus()
    message.id = object.id ?? ''
    message.permanent = object.permanent ?? false
    return message
  },
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
  : T extends Array<infer U>
  ? Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U>
  ? ReadonlyArray<DeepPartial<U>>
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

// If you get a compile-error about 'Constructor<Long> and ... have no overlap',
// add '--ts_proto_opt=esModuleInterop=true' as a flag when calling 'protoc'.
if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any
  _m0.configure()
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
