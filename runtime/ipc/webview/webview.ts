/* eslint-disable */
import { util, configure, Writer, Reader } from 'protobufjs/minimal'
import * as Long from 'long'

export const protobufPackage = 'webview'

/** RuntimeToWebViewType is the set of sync message types */
export enum RuntimeToWebViewType {
  RuntimeToWebViewType_UNKNOWN = 0,
  /** RuntimeToWebViewType_CREATE_VIEW - RuntimeToWebViewType_CREATE_VIEW creates a new web view. */
  RuntimeToWebViewType_CREATE_VIEW = 1,
  /** RuntimeToWebViewType_QUERY_STATUS - RuntimeToWebViewType_QUERY_STATUS queries the web view status. */
  RuntimeToWebViewType_QUERY_STATUS = 2,
  UNRECOGNIZED = -1,
}

export function runtimeToWebViewTypeFromJSON(
  object: any
): RuntimeToWebViewType {
  switch (object) {
    case 0:
    case 'RuntimeToWebViewType_UNKNOWN':
      return RuntimeToWebViewType.RuntimeToWebViewType_UNKNOWN
    case 1:
    case 'RuntimeToWebViewType_CREATE_VIEW':
      return RuntimeToWebViewType.RuntimeToWebViewType_CREATE_VIEW
    case 2:
    case 'RuntimeToWebViewType_QUERY_STATUS':
      return RuntimeToWebViewType.RuntimeToWebViewType_QUERY_STATUS
    case -1:
    case 'UNRECOGNIZED':
    default:
      return RuntimeToWebViewType.UNRECOGNIZED
  }
}

export function runtimeToWebViewTypeToJSON(
  object: RuntimeToWebViewType
): string {
  switch (object) {
    case RuntimeToWebViewType.RuntimeToWebViewType_UNKNOWN:
      return 'RuntimeToWebViewType_UNKNOWN'
    case RuntimeToWebViewType.RuntimeToWebViewType_CREATE_VIEW:
      return 'RuntimeToWebViewType_CREATE_VIEW'
    case RuntimeToWebViewType.RuntimeToWebViewType_QUERY_STATUS:
      return 'RuntimeToWebViewType_QUERY_STATUS'
    default:
      return 'UNKNOWN'
  }
}

/** WebViewToRuntimeType is the set of messages to the runtime. */
export enum WebViewToRuntimeType {
  WebViewToRuntimeType_UNKNOWN = 0,
  /** WebViewToRuntimeType_VIEW_STATUS - WebViewToRuntimeType_VIEW_STATUS is the view status response. */
  WebViewToRuntimeType_VIEW_STATUS = 1,
  UNRECOGNIZED = -1,
}

export function webViewToRuntimeTypeFromJSON(
  object: any
): WebViewToRuntimeType {
  switch (object) {
    case 0:
    case 'WebViewToRuntimeType_UNKNOWN':
      return WebViewToRuntimeType.WebViewToRuntimeType_UNKNOWN
    case 1:
    case 'WebViewToRuntimeType_VIEW_STATUS':
      return WebViewToRuntimeType.WebViewToRuntimeType_VIEW_STATUS
    case -1:
    case 'UNRECOGNIZED':
    default:
      return WebViewToRuntimeType.UNRECOGNIZED
  }
}

export function webViewToRuntimeTypeToJSON(
  object: WebViewToRuntimeType
): string {
  switch (object) {
    case WebViewToRuntimeType.WebViewToRuntimeType_UNKNOWN:
      return 'WebViewToRuntimeType_UNKNOWN'
    case WebViewToRuntimeType.WebViewToRuntimeType_VIEW_STATUS:
      return 'WebViewToRuntimeType_VIEW_STATUS'
    default:
      return 'UNKNOWN'
  }
}

/** RuntimeToWebView are messages sent to the WebView. */
export interface RuntimeToWebView {
  messageType: RuntimeToWebViewType
  /** CreateView is the body of the CREATE_VIEW message. */
  createView: CreateView | undefined
  /** QueryViewStatus is the body of the QUERY_VIEW_STATUS message. */
  queryViewStatus: QueryViewStatus | undefined
}

/** WebViewToRuntime are messages sent to the Runtime from the WebView. */
export interface WebViewToRuntime {
  messageType: WebViewToRuntimeType
  /** ViewStatus is the body of the VIEW_STATUS message. */
  viewStatus: ViewStatus | undefined
}

/** Createiew is a message to create a new WebView. */
export interface CreateView {
  /** Id is the unique identifier for the new WebView. */
  id: string
}

/** QueryViewStatus is a request for QUERY_STATUS. */
export interface QueryViewStatus {}

/**
 * ViewStatus is a web-view status report to the runtime.
 *
 * sent when the WebView starts up and/or is prompted
 */
export interface ViewStatus {
  /**
   * Id is the unique identifier for the webview.
   * if !is_root, id is specified by the runtime when creating the WebView.
   */
  id: string
  /** IsRoot indicates that this is a "root" webview and cannot be closed. */
  isRoot: boolean
}

const baseRuntimeToWebView: object = { messageType: 0 }

export const RuntimeToWebView = {
  encode(message: RuntimeToWebView, writer: Writer = Writer.create()): Writer {
    if (message.messageType !== 0) {
      writer.uint32(8).int32(message.messageType)
    }
    if (message.createView !== undefined) {
      CreateView.encode(message.createView, writer.uint32(18).fork()).ldelim()
    }
    if (message.queryViewStatus !== undefined) {
      QueryViewStatus.encode(
        message.queryViewStatus,
        writer.uint32(26).fork()
      ).ldelim()
    }
    return writer
  },

  decode(input: Reader | Uint8Array, length?: number): RuntimeToWebView {
    const reader = input instanceof Reader ? input : new Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = { ...baseRuntimeToWebView } as RuntimeToWebView
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
          message.queryViewStatus = QueryViewStatus.decode(
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

  fromJSON(object: any): RuntimeToWebView {
    const message = { ...baseRuntimeToWebView } as RuntimeToWebView
    if (object.messageType !== undefined && object.messageType !== null) {
      message.messageType = runtimeToWebViewTypeFromJSON(object.messageType)
    } else {
      message.messageType = 0
    }
    if (object.createView !== undefined && object.createView !== null) {
      message.createView = CreateView.fromJSON(object.createView)
    } else {
      message.createView = undefined
    }
    if (
      object.queryViewStatus !== undefined &&
      object.queryViewStatus !== null
    ) {
      message.queryViewStatus = QueryViewStatus.fromJSON(object.queryViewStatus)
    } else {
      message.queryViewStatus = undefined
    }
    return message
  },

  toJSON(message: RuntimeToWebView): unknown {
    const obj: any = {}
    message.messageType !== undefined &&
      (obj.messageType = runtimeToWebViewTypeToJSON(message.messageType))
    message.createView !== undefined &&
      (obj.createView = message.createView
        ? CreateView.toJSON(message.createView)
        : undefined)
    message.queryViewStatus !== undefined &&
      (obj.queryViewStatus = message.queryViewStatus
        ? QueryViewStatus.toJSON(message.queryViewStatus)
        : undefined)
    return obj
  },

  fromPartial(object: DeepPartial<RuntimeToWebView>): RuntimeToWebView {
    const message = { ...baseRuntimeToWebView } as RuntimeToWebView
    if (object.messageType !== undefined && object.messageType !== null) {
      message.messageType = object.messageType
    } else {
      message.messageType = 0
    }
    if (object.createView !== undefined && object.createView !== null) {
      message.createView = CreateView.fromPartial(object.createView)
    } else {
      message.createView = undefined
    }
    if (
      object.queryViewStatus !== undefined &&
      object.queryViewStatus !== null
    ) {
      message.queryViewStatus = QueryViewStatus.fromPartial(
        object.queryViewStatus
      )
    } else {
      message.queryViewStatus = undefined
    }
    return message
  },
}

const baseWebViewToRuntime: object = { messageType: 0 }

export const WebViewToRuntime = {
  encode(message: WebViewToRuntime, writer: Writer = Writer.create()): Writer {
    if (message.messageType !== 0) {
      writer.uint32(8).int32(message.messageType)
    }
    if (message.viewStatus !== undefined) {
      ViewStatus.encode(message.viewStatus, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: Reader | Uint8Array, length?: number): WebViewToRuntime {
    const reader = input instanceof Reader ? input : new Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = { ...baseWebViewToRuntime } as WebViewToRuntime
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.messageType = reader.int32() as any
          break
        case 2:
          message.viewStatus = ViewStatus.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): WebViewToRuntime {
    const message = { ...baseWebViewToRuntime } as WebViewToRuntime
    if (object.messageType !== undefined && object.messageType !== null) {
      message.messageType = webViewToRuntimeTypeFromJSON(object.messageType)
    } else {
      message.messageType = 0
    }
    if (object.viewStatus !== undefined && object.viewStatus !== null) {
      message.viewStatus = ViewStatus.fromJSON(object.viewStatus)
    } else {
      message.viewStatus = undefined
    }
    return message
  },

  toJSON(message: WebViewToRuntime): unknown {
    const obj: any = {}
    message.messageType !== undefined &&
      (obj.messageType = webViewToRuntimeTypeToJSON(message.messageType))
    message.viewStatus !== undefined &&
      (obj.viewStatus = message.viewStatus
        ? ViewStatus.toJSON(message.viewStatus)
        : undefined)
    return obj
  },

  fromPartial(object: DeepPartial<WebViewToRuntime>): WebViewToRuntime {
    const message = { ...baseWebViewToRuntime } as WebViewToRuntime
    if (object.messageType !== undefined && object.messageType !== null) {
      message.messageType = object.messageType
    } else {
      message.messageType = 0
    }
    if (object.viewStatus !== undefined && object.viewStatus !== null) {
      message.viewStatus = ViewStatus.fromPartial(object.viewStatus)
    } else {
      message.viewStatus = undefined
    }
    return message
  },
}

const baseCreateView: object = { id: '' }

export const CreateView = {
  encode(message: CreateView, writer: Writer = Writer.create()): Writer {
    if (message.id !== '') {
      writer.uint32(10).string(message.id)
    }
    return writer
  },

  decode(input: Reader | Uint8Array, length?: number): CreateView {
    const reader = input instanceof Reader ? input : new Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = { ...baseCreateView } as CreateView
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
    const message = { ...baseCreateView } as CreateView
    if (object.id !== undefined && object.id !== null) {
      message.id = String(object.id)
    } else {
      message.id = ''
    }
    return message
  },

  toJSON(message: CreateView): unknown {
    const obj: any = {}
    message.id !== undefined && (obj.id = message.id)
    return obj
  },

  fromPartial(object: DeepPartial<CreateView>): CreateView {
    const message = { ...baseCreateView } as CreateView
    if (object.id !== undefined && object.id !== null) {
      message.id = object.id
    } else {
      message.id = ''
    }
    return message
  },
}

const baseQueryViewStatus: object = {}

export const QueryViewStatus = {
  encode(_: QueryViewStatus, writer: Writer = Writer.create()): Writer {
    return writer
  },

  decode(input: Reader | Uint8Array, length?: number): QueryViewStatus {
    const reader = input instanceof Reader ? input : new Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = { ...baseQueryViewStatus } as QueryViewStatus
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

  fromJSON(_: any): QueryViewStatus {
    const message = { ...baseQueryViewStatus } as QueryViewStatus
    return message
  },

  toJSON(_: QueryViewStatus): unknown {
    const obj: any = {}
    return obj
  },

  fromPartial(_: DeepPartial<QueryViewStatus>): QueryViewStatus {
    const message = { ...baseQueryViewStatus } as QueryViewStatus
    return message
  },
}

const baseViewStatus: object = { id: '', isRoot: false }

export const ViewStatus = {
  encode(message: ViewStatus, writer: Writer = Writer.create()): Writer {
    if (message.id !== '') {
      writer.uint32(10).string(message.id)
    }
    if (message.isRoot === true) {
      writer.uint32(16).bool(message.isRoot)
    }
    return writer
  },

  decode(input: Reader | Uint8Array, length?: number): ViewStatus {
    const reader = input instanceof Reader ? input : new Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = { ...baseViewStatus } as ViewStatus
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.id = reader.string()
          break
        case 2:
          message.isRoot = reader.bool()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): ViewStatus {
    const message = { ...baseViewStatus } as ViewStatus
    if (object.id !== undefined && object.id !== null) {
      message.id = String(object.id)
    } else {
      message.id = ''
    }
    if (object.isRoot !== undefined && object.isRoot !== null) {
      message.isRoot = Boolean(object.isRoot)
    } else {
      message.isRoot = false
    }
    return message
  },

  toJSON(message: ViewStatus): unknown {
    const obj: any = {}
    message.id !== undefined && (obj.id = message.id)
    message.isRoot !== undefined && (obj.isRoot = message.isRoot)
    return obj
  },

  fromPartial(object: DeepPartial<ViewStatus>): ViewStatus {
    const message = { ...baseViewStatus } as ViewStatus
    if (object.id !== undefined && object.id !== null) {
      message.id = object.id
    } else {
      message.id = ''
    }
    if (object.isRoot !== undefined && object.isRoot !== null) {
      message.isRoot = object.isRoot
    } else {
      message.isRoot = false
    }
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

// If you get a compile-error about 'Constructor<Long> and ... have no overlap',
// add '--ts_proto_opt=esModuleInterop=true' as a flag when calling 'protoc'.
if (util.Long !== Long) {
  util.Long = Long as any
  configure()
}
