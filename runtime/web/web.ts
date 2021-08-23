/* eslint-disable */
import { util, configure, Writer, Reader } from 'protobufjs/minimal'
import * as Long from 'long'

export const protobufPackage = 'web'

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

/** RuntimeToWeb are messages sent to the Web runtime from the Go runtime. */
export interface RuntimeToWeb {
  messageType: RuntimeToWebType
  /** CreateView is the body of the CREATE_VIEW message. */
  createView: CreateView | undefined
  /** QueryWebStatus is the body of the QUERY_VIEW_STATUS message. */
  queryViewStatus: QueryWebStatus | undefined
}

/** WebViewToRuntime are messages sent to the Runtime from the WebView. */
export interface WebViewToRuntime {
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

const baseRuntimeToWeb: object = { messageType: 0 }

export const RuntimeToWeb = {
  encode(message: RuntimeToWeb, writer: Writer = Writer.create()): Writer {
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

  decode(input: Reader | Uint8Array, length?: number): RuntimeToWeb {
    const reader = input instanceof Reader ? input : new Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = { ...baseRuntimeToWeb } as RuntimeToWeb
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
    const message = { ...baseRuntimeToWeb } as RuntimeToWeb
    if (object.messageType !== undefined && object.messageType !== null) {
      message.messageType = runtimeToWebTypeFromJSON(object.messageType)
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
      message.queryViewStatus = QueryWebStatus.fromJSON(object.queryViewStatus)
    } else {
      message.queryViewStatus = undefined
    }
    return message
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

  fromPartial(object: DeepPartial<RuntimeToWeb>): RuntimeToWeb {
    const message = { ...baseRuntimeToWeb } as RuntimeToWeb
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
      message.queryViewStatus = QueryWebStatus.fromPartial(
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
    if (message.webStatus !== undefined) {
      WebStatus.encode(message.webStatus, writer.uint32(18).fork()).ldelim()
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
          message.webStatus = WebStatus.decode(reader, reader.uint32())
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
      message.messageType = webToRuntimeTypeFromJSON(object.messageType)
    } else {
      message.messageType = 0
    }
    if (object.webStatus !== undefined && object.webStatus !== null) {
      message.webStatus = WebStatus.fromJSON(object.webStatus)
    } else {
      message.webStatus = undefined
    }
    return message
  },

  toJSON(message: WebViewToRuntime): unknown {
    const obj: any = {}
    message.messageType !== undefined &&
      (obj.messageType = webToRuntimeTypeToJSON(message.messageType))
    message.webStatus !== undefined &&
      (obj.webStatus = message.webStatus
        ? WebStatus.toJSON(message.webStatus)
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
    if (object.webStatus !== undefined && object.webStatus !== null) {
      message.webStatus = WebStatus.fromPartial(object.webStatus)
    } else {
      message.webStatus = undefined
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

const baseQueryWebStatus: object = {}

export const QueryWebStatus = {
  encode(_: QueryWebStatus, writer: Writer = Writer.create()): Writer {
    return writer
  },

  decode(input: Reader | Uint8Array, length?: number): QueryWebStatus {
    const reader = input instanceof Reader ? input : new Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = { ...baseQueryWebStatus } as QueryWebStatus
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
    const message = { ...baseQueryWebStatus } as QueryWebStatus
    return message
  },

  toJSON(_: QueryWebStatus): unknown {
    const obj: any = {}
    return obj
  },

  fromPartial(_: DeepPartial<QueryWebStatus>): QueryWebStatus {
    const message = { ...baseQueryWebStatus } as QueryWebStatus
    return message
  },
}

const baseWebStatus: object = {}

export const WebStatus = {
  encode(message: WebStatus, writer: Writer = Writer.create()): Writer {
    for (const v of message.webViews) {
      WebViewStatus.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: Reader | Uint8Array, length?: number): WebStatus {
    const reader = input instanceof Reader ? input : new Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = { ...baseWebStatus } as WebStatus
    message.webViews = []
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
    const message = { ...baseWebStatus } as WebStatus
    message.webViews = []
    if (object.webViews !== undefined && object.webViews !== null) {
      for (const e of object.webViews) {
        message.webViews.push(WebViewStatus.fromJSON(e))
      }
    }
    return message
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

  fromPartial(object: DeepPartial<WebStatus>): WebStatus {
    const message = { ...baseWebStatus } as WebStatus
    message.webViews = []
    if (object.webViews !== undefined && object.webViews !== null) {
      for (const e of object.webViews) {
        message.webViews.push(WebViewStatus.fromPartial(e))
      }
    }
    return message
  },
}

const baseWebViewStatus: object = { id: '', permanent: false }

export const WebViewStatus = {
  encode(message: WebViewStatus, writer: Writer = Writer.create()): Writer {
    if (message.id !== '') {
      writer.uint32(10).string(message.id)
    }
    if (message.permanent === true) {
      writer.uint32(16).bool(message.permanent)
    }
    return writer
  },

  decode(input: Reader | Uint8Array, length?: number): WebViewStatus {
    const reader = input instanceof Reader ? input : new Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = { ...baseWebViewStatus } as WebViewStatus
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
    const message = { ...baseWebViewStatus } as WebViewStatus
    if (object.id !== undefined && object.id !== null) {
      message.id = String(object.id)
    } else {
      message.id = ''
    }
    if (object.permanent !== undefined && object.permanent !== null) {
      message.permanent = Boolean(object.permanent)
    } else {
      message.permanent = false
    }
    return message
  },

  toJSON(message: WebViewStatus): unknown {
    const obj: any = {}
    message.id !== undefined && (obj.id = message.id)
    message.permanent !== undefined && (obj.permanent = message.permanent)
    return obj
  },

  fromPartial(object: DeepPartial<WebViewStatus>): WebViewStatus {
    const message = { ...baseWebViewStatus } as WebViewStatus
    if (object.id !== undefined && object.id !== null) {
      message.id = object.id
    } else {
      message.id = ''
    }
    if (object.permanent !== undefined && object.permanent !== null) {
      message.permanent = object.permanent
    } else {
      message.permanent = false
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
