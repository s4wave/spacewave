/* eslint-disable */
import { RpcStreamPacket } from '@go/github.com/aperturerobotics/starpc/rpcstream/rpcstream.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'web.document'

/** WatchWebDocumentStatusRequest is the body of the WatchWebDocumentStatus request. */
export interface WatchWebDocumentStatusRequest {}

/** WebDocumentStatus contains a snapshot of status for a Document instance. */
export interface WebDocumentStatus {
  /** Snapshot indicates this is a full snapshot of the lists. */
  snapshot: boolean
  /** WebViews contains the list of web view statuses. */
  webViews: WebViewStatus[]
  /** WebWorkers contains the list of web worker statuses. */
  webWorkers: WebWorkerStatus[]
}

/** WebViewStatus contains status for a web view. */
export interface WebViewStatus {
  /** Id is the unique identifier for the webview. */
  id: string
  /**
   * Deleted indicates the web view was just removed.
   * If set, all below fields are ignored.
   */
  deleted: boolean
  /**
   * ParentId is the unique identifier for the parent web view.
   * May be empty.
   */
  parentId: string
  /** Permanent indicates that this is a "root" webview and cannot be closed. */
  permanent: boolean
}

/** WebWorkerStatus contains status for a web worker. */
export interface WebWorkerStatus {
  /** Id is the unique identifier for the worker. */
  id: string
  /**
   * Deleted indicates the web worker was just removed.
   * If set, all below fields are ignored.
   */
  deleted: boolean
  /** Shared indicates that the worker is a SharedWorker. */
  shared: boolean
}

/** CreateWebViewRequest is a request to create a new web view. */
export interface CreateWebViewRequest {
  /** id is the identifier for the new WebView. */
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

/** CreateWebWorkerRequest is a request to create a new web worker. */
export interface CreateWebWorkerRequest {
  /** Id is the identifier for the new WebWorker. */
  id: string
  /** Url is the url to the source to load into the worker. */
  url: string
  /** Shared indicates this should be a worker shared between all WebDocument (if possible) */
  shared: boolean
}

/** CreateWebWorkerResponse is the response to the CreateWebWorker request. */
export interface CreateWebWorkerResponse {
  /**
   * Created indicates the WebWorker was created.
   * If this is not set, assumes we cannot create WebWorkers.
   */
  created: boolean
  /**
   * Shared indicates that SharedWorker was supported & used
   * Should only be set if shared was also set in the request
   */
  shared: boolean
}

/** RemoveWebWorkerRequest is a request to terminate a web worker. */
export interface RemoveWebWorkerRequest {
  /** Id is the identifier for the removed WebWorker. */
  id: string
}

/** RemoveWebWorkerResponse is the response to the RemoveWebWorker request. */
export interface RemoveWebWorkerResponse {
  /**
   * Removed indicates the WebWorker was removed.
   * If false, the worker was not found or not running.
   */
  removed: boolean
}

function createBaseWatchWebDocumentStatusRequest(): WatchWebDocumentStatusRequest {
  return {}
}

export const WatchWebDocumentStatusRequest = {
  encode(
    _: WatchWebDocumentStatusRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): WatchWebDocumentStatusRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWatchWebDocumentStatusRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<WatchWebDocumentStatusRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<
          WatchWebDocumentStatusRequest | WatchWebDocumentStatusRequest[]
        >
      | Iterable<
          WatchWebDocumentStatusRequest | WatchWebDocumentStatusRequest[]
        >,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WatchWebDocumentStatusRequest.encode(p).finish()]
        }
      } else {
        yield* [WatchWebDocumentStatusRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WatchWebDocumentStatusRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WatchWebDocumentStatusRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WatchWebDocumentStatusRequest.decode(p)]
        }
      } else {
        yield* [WatchWebDocumentStatusRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(_: any): WatchWebDocumentStatusRequest {
    return {}
  },

  toJSON(_: WatchWebDocumentStatusRequest): unknown {
    const obj: any = {}
    return obj
  },

  create<I extends Exact<DeepPartial<WatchWebDocumentStatusRequest>, I>>(
    base?: I,
  ): WatchWebDocumentStatusRequest {
    return WatchWebDocumentStatusRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<WatchWebDocumentStatusRequest>, I>>(
    _: I,
  ): WatchWebDocumentStatusRequest {
    const message = createBaseWatchWebDocumentStatusRequest()
    return message
  },
}

function createBaseWebDocumentStatus(): WebDocumentStatus {
  return { snapshot: false, webViews: [], webWorkers: [] }
}

export const WebDocumentStatus = {
  encode(
    message: WebDocumentStatus,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.snapshot === true) {
      writer.uint32(8).bool(message.snapshot)
    }
    for (const v of message.webViews) {
      WebViewStatus.encode(v!, writer.uint32(18).fork()).ldelim()
    }
    for (const v of message.webWorkers) {
      WebWorkerStatus.encode(v!, writer.uint32(26).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WebDocumentStatus {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWebDocumentStatus()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.snapshot = reader.bool()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.webViews.push(WebViewStatus.decode(reader, reader.uint32()))
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.webWorkers.push(
            WebWorkerStatus.decode(reader, reader.uint32()),
          )
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
  // Transform<WebDocumentStatus, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WebDocumentStatus | WebDocumentStatus[]>
      | Iterable<WebDocumentStatus | WebDocumentStatus[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WebDocumentStatus.encode(p).finish()]
        }
      } else {
        yield* [WebDocumentStatus.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WebDocumentStatus>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WebDocumentStatus> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WebDocumentStatus.decode(p)]
        }
      } else {
        yield* [WebDocumentStatus.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): WebDocumentStatus {
    return {
      snapshot: isSet(object.snapshot)
        ? globalThis.Boolean(object.snapshot)
        : false,
      webViews: globalThis.Array.isArray(object?.webViews)
        ? object.webViews.map((e: any) => WebViewStatus.fromJSON(e))
        : [],
      webWorkers: globalThis.Array.isArray(object?.webWorkers)
        ? object.webWorkers.map((e: any) => WebWorkerStatus.fromJSON(e))
        : [],
    }
  },

  toJSON(message: WebDocumentStatus): unknown {
    const obj: any = {}
    if (message.snapshot === true) {
      obj.snapshot = message.snapshot
    }
    if (message.webViews?.length) {
      obj.webViews = message.webViews.map((e) => WebViewStatus.toJSON(e))
    }
    if (message.webWorkers?.length) {
      obj.webWorkers = message.webWorkers.map((e) => WebWorkerStatus.toJSON(e))
    }
    return obj
  },

  create<I extends Exact<DeepPartial<WebDocumentStatus>, I>>(
    base?: I,
  ): WebDocumentStatus {
    return WebDocumentStatus.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<WebDocumentStatus>, I>>(
    object: I,
  ): WebDocumentStatus {
    const message = createBaseWebDocumentStatus()
    message.snapshot = object.snapshot ?? false
    message.webViews =
      object.webViews?.map((e) => WebViewStatus.fromPartial(e)) || []
    message.webWorkers =
      object.webWorkers?.map((e) => WebWorkerStatus.fromPartial(e)) || []
    return message
  },
}

function createBaseWebViewStatus(): WebViewStatus {
  return { id: '', deleted: false, parentId: '', permanent: false }
}

export const WebViewStatus = {
  encode(
    message: WebViewStatus,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id !== '') {
      writer.uint32(10).string(message.id)
    }
    if (message.deleted === true) {
      writer.uint32(16).bool(message.deleted)
    }
    if (message.parentId !== '') {
      writer.uint32(26).string(message.parentId)
    }
    if (message.permanent === true) {
      writer.uint32(32).bool(message.permanent)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WebViewStatus {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWebViewStatus()
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
          if (tag !== 16) {
            break
          }

          message.deleted = reader.bool()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.parentId = reader.string()
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
  // Transform<WebViewStatus, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WebViewStatus | WebViewStatus[]>
      | Iterable<WebViewStatus | WebViewStatus[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WebViewStatus.encode(p).finish()]
        }
      } else {
        yield* [WebViewStatus.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WebViewStatus>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WebViewStatus> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WebViewStatus.decode(p)]
        }
      } else {
        yield* [WebViewStatus.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): WebViewStatus {
    return {
      id: isSet(object.id) ? globalThis.String(object.id) : '',
      deleted: isSet(object.deleted)
        ? globalThis.Boolean(object.deleted)
        : false,
      parentId: isSet(object.parentId)
        ? globalThis.String(object.parentId)
        : '',
      permanent: isSet(object.permanent)
        ? globalThis.Boolean(object.permanent)
        : false,
    }
  },

  toJSON(message: WebViewStatus): unknown {
    const obj: any = {}
    if (message.id !== '') {
      obj.id = message.id
    }
    if (message.deleted === true) {
      obj.deleted = message.deleted
    }
    if (message.parentId !== '') {
      obj.parentId = message.parentId
    }
    if (message.permanent === true) {
      obj.permanent = message.permanent
    }
    return obj
  },

  create<I extends Exact<DeepPartial<WebViewStatus>, I>>(
    base?: I,
  ): WebViewStatus {
    return WebViewStatus.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<WebViewStatus>, I>>(
    object: I,
  ): WebViewStatus {
    const message = createBaseWebViewStatus()
    message.id = object.id ?? ''
    message.deleted = object.deleted ?? false
    message.parentId = object.parentId ?? ''
    message.permanent = object.permanent ?? false
    return message
  },
}

function createBaseWebWorkerStatus(): WebWorkerStatus {
  return { id: '', deleted: false, shared: false }
}

export const WebWorkerStatus = {
  encode(
    message: WebWorkerStatus,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id !== '') {
      writer.uint32(10).string(message.id)
    }
    if (message.deleted === true) {
      writer.uint32(16).bool(message.deleted)
    }
    if (message.shared === true) {
      writer.uint32(24).bool(message.shared)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WebWorkerStatus {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWebWorkerStatus()
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
          if (tag !== 16) {
            break
          }

          message.deleted = reader.bool()
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.shared = reader.bool()
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
  // Transform<WebWorkerStatus, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WebWorkerStatus | WebWorkerStatus[]>
      | Iterable<WebWorkerStatus | WebWorkerStatus[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WebWorkerStatus.encode(p).finish()]
        }
      } else {
        yield* [WebWorkerStatus.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WebWorkerStatus>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WebWorkerStatus> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [WebWorkerStatus.decode(p)]
        }
      } else {
        yield* [WebWorkerStatus.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): WebWorkerStatus {
    return {
      id: isSet(object.id) ? globalThis.String(object.id) : '',
      deleted: isSet(object.deleted)
        ? globalThis.Boolean(object.deleted)
        : false,
      shared: isSet(object.shared) ? globalThis.Boolean(object.shared) : false,
    }
  },

  toJSON(message: WebWorkerStatus): unknown {
    const obj: any = {}
    if (message.id !== '') {
      obj.id = message.id
    }
    if (message.deleted === true) {
      obj.deleted = message.deleted
    }
    if (message.shared === true) {
      obj.shared = message.shared
    }
    return obj
  },

  create<I extends Exact<DeepPartial<WebWorkerStatus>, I>>(
    base?: I,
  ): WebWorkerStatus {
    return WebWorkerStatus.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<WebWorkerStatus>, I>>(
    object: I,
  ): WebWorkerStatus {
    const message = createBaseWebWorkerStatus()
    message.id = object.id ?? ''
    message.deleted = object.deleted ?? false
    message.shared = object.shared ?? false
    return message
  },
}

function createBaseCreateWebViewRequest(): CreateWebViewRequest {
  return { id: '' }
}

export const CreateWebViewRequest = {
  encode(
    message: CreateWebViewRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id !== '') {
      writer.uint32(10).string(message.id)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): CreateWebViewRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseCreateWebViewRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.id = reader.string()
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
  // Transform<CreateWebViewRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<CreateWebViewRequest | CreateWebViewRequest[]>
      | Iterable<CreateWebViewRequest | CreateWebViewRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [CreateWebViewRequest.encode(p).finish()]
        }
      } else {
        yield* [CreateWebViewRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, CreateWebViewRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<CreateWebViewRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [CreateWebViewRequest.decode(p)]
        }
      } else {
        yield* [CreateWebViewRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): CreateWebViewRequest {
    return { id: isSet(object.id) ? globalThis.String(object.id) : '' }
  },

  toJSON(message: CreateWebViewRequest): unknown {
    const obj: any = {}
    if (message.id !== '') {
      obj.id = message.id
    }
    return obj
  },

  create<I extends Exact<DeepPartial<CreateWebViewRequest>, I>>(
    base?: I,
  ): CreateWebViewRequest {
    return CreateWebViewRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<CreateWebViewRequest>, I>>(
    object: I,
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
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.created === true) {
      writer.uint32(8).bool(message.created)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): CreateWebViewResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseCreateWebViewResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.created = reader.bool()
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
  // Transform<CreateWebViewResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<CreateWebViewResponse | CreateWebViewResponse[]>
      | Iterable<CreateWebViewResponse | CreateWebViewResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [CreateWebViewResponse.encode(p).finish()]
        }
      } else {
        yield* [CreateWebViewResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, CreateWebViewResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<CreateWebViewResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [CreateWebViewResponse.decode(p)]
        }
      } else {
        yield* [CreateWebViewResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): CreateWebViewResponse {
    return {
      created: isSet(object.created)
        ? globalThis.Boolean(object.created)
        : false,
    }
  },

  toJSON(message: CreateWebViewResponse): unknown {
    const obj: any = {}
    if (message.created === true) {
      obj.created = message.created
    }
    return obj
  },

  create<I extends Exact<DeepPartial<CreateWebViewResponse>, I>>(
    base?: I,
  ): CreateWebViewResponse {
    return CreateWebViewResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<CreateWebViewResponse>, I>>(
    object: I,
  ): CreateWebViewResponse {
    const message = createBaseCreateWebViewResponse()
    message.created = object.created ?? false
    return message
  },
}

function createBaseCreateWebWorkerRequest(): CreateWebWorkerRequest {
  return { id: '', url: '', shared: false }
}

export const CreateWebWorkerRequest = {
  encode(
    message: CreateWebWorkerRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id !== '') {
      writer.uint32(10).string(message.id)
    }
    if (message.url !== '') {
      writer.uint32(18).string(message.url)
    }
    if (message.shared === true) {
      writer.uint32(24).bool(message.shared)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): CreateWebWorkerRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseCreateWebWorkerRequest()
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

          message.url = reader.string()
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.shared = reader.bool()
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
  // Transform<CreateWebWorkerRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<CreateWebWorkerRequest | CreateWebWorkerRequest[]>
      | Iterable<CreateWebWorkerRequest | CreateWebWorkerRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [CreateWebWorkerRequest.encode(p).finish()]
        }
      } else {
        yield* [CreateWebWorkerRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, CreateWebWorkerRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<CreateWebWorkerRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [CreateWebWorkerRequest.decode(p)]
        }
      } else {
        yield* [CreateWebWorkerRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): CreateWebWorkerRequest {
    return {
      id: isSet(object.id) ? globalThis.String(object.id) : '',
      url: isSet(object.url) ? globalThis.String(object.url) : '',
      shared: isSet(object.shared) ? globalThis.Boolean(object.shared) : false,
    }
  },

  toJSON(message: CreateWebWorkerRequest): unknown {
    const obj: any = {}
    if (message.id !== '') {
      obj.id = message.id
    }
    if (message.url !== '') {
      obj.url = message.url
    }
    if (message.shared === true) {
      obj.shared = message.shared
    }
    return obj
  },

  create<I extends Exact<DeepPartial<CreateWebWorkerRequest>, I>>(
    base?: I,
  ): CreateWebWorkerRequest {
    return CreateWebWorkerRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<CreateWebWorkerRequest>, I>>(
    object: I,
  ): CreateWebWorkerRequest {
    const message = createBaseCreateWebWorkerRequest()
    message.id = object.id ?? ''
    message.url = object.url ?? ''
    message.shared = object.shared ?? false
    return message
  },
}

function createBaseCreateWebWorkerResponse(): CreateWebWorkerResponse {
  return { created: false, shared: false }
}

export const CreateWebWorkerResponse = {
  encode(
    message: CreateWebWorkerResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.created === true) {
      writer.uint32(8).bool(message.created)
    }
    if (message.shared === true) {
      writer.uint32(16).bool(message.shared)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): CreateWebWorkerResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseCreateWebWorkerResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.created = reader.bool()
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.shared = reader.bool()
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
  // Transform<CreateWebWorkerResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<CreateWebWorkerResponse | CreateWebWorkerResponse[]>
      | Iterable<CreateWebWorkerResponse | CreateWebWorkerResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [CreateWebWorkerResponse.encode(p).finish()]
        }
      } else {
        yield* [CreateWebWorkerResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, CreateWebWorkerResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<CreateWebWorkerResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [CreateWebWorkerResponse.decode(p)]
        }
      } else {
        yield* [CreateWebWorkerResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): CreateWebWorkerResponse {
    return {
      created: isSet(object.created)
        ? globalThis.Boolean(object.created)
        : false,
      shared: isSet(object.shared) ? globalThis.Boolean(object.shared) : false,
    }
  },

  toJSON(message: CreateWebWorkerResponse): unknown {
    const obj: any = {}
    if (message.created === true) {
      obj.created = message.created
    }
    if (message.shared === true) {
      obj.shared = message.shared
    }
    return obj
  },

  create<I extends Exact<DeepPartial<CreateWebWorkerResponse>, I>>(
    base?: I,
  ): CreateWebWorkerResponse {
    return CreateWebWorkerResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<CreateWebWorkerResponse>, I>>(
    object: I,
  ): CreateWebWorkerResponse {
    const message = createBaseCreateWebWorkerResponse()
    message.created = object.created ?? false
    message.shared = object.shared ?? false
    return message
  },
}

function createBaseRemoveWebWorkerRequest(): RemoveWebWorkerRequest {
  return { id: '' }
}

export const RemoveWebWorkerRequest = {
  encode(
    message: RemoveWebWorkerRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.id !== '') {
      writer.uint32(10).string(message.id)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): RemoveWebWorkerRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRemoveWebWorkerRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.id = reader.string()
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
  // Transform<RemoveWebWorkerRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RemoveWebWorkerRequest | RemoveWebWorkerRequest[]>
      | Iterable<RemoveWebWorkerRequest | RemoveWebWorkerRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [RemoveWebWorkerRequest.encode(p).finish()]
        }
      } else {
        yield* [RemoveWebWorkerRequest.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RemoveWebWorkerRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RemoveWebWorkerRequest> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [RemoveWebWorkerRequest.decode(p)]
        }
      } else {
        yield* [RemoveWebWorkerRequest.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): RemoveWebWorkerRequest {
    return { id: isSet(object.id) ? globalThis.String(object.id) : '' }
  },

  toJSON(message: RemoveWebWorkerRequest): unknown {
    const obj: any = {}
    if (message.id !== '') {
      obj.id = message.id
    }
    return obj
  },

  create<I extends Exact<DeepPartial<RemoveWebWorkerRequest>, I>>(
    base?: I,
  ): RemoveWebWorkerRequest {
    return RemoveWebWorkerRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<RemoveWebWorkerRequest>, I>>(
    object: I,
  ): RemoveWebWorkerRequest {
    const message = createBaseRemoveWebWorkerRequest()
    message.id = object.id ?? ''
    return message
  },
}

function createBaseRemoveWebWorkerResponse(): RemoveWebWorkerResponse {
  return { removed: false }
}

export const RemoveWebWorkerResponse = {
  encode(
    message: RemoveWebWorkerResponse,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.removed === true) {
      writer.uint32(8).bool(message.removed)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): RemoveWebWorkerResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRemoveWebWorkerResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.removed = reader.bool()
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
  // Transform<RemoveWebWorkerResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RemoveWebWorkerResponse | RemoveWebWorkerResponse[]>
      | Iterable<RemoveWebWorkerResponse | RemoveWebWorkerResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [RemoveWebWorkerResponse.encode(p).finish()]
        }
      } else {
        yield* [RemoveWebWorkerResponse.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RemoveWebWorkerResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RemoveWebWorkerResponse> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [RemoveWebWorkerResponse.decode(p)]
        }
      } else {
        yield* [RemoveWebWorkerResponse.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): RemoveWebWorkerResponse {
    return {
      removed: isSet(object.removed)
        ? globalThis.Boolean(object.removed)
        : false,
    }
  },

  toJSON(message: RemoveWebWorkerResponse): unknown {
    const obj: any = {}
    if (message.removed === true) {
      obj.removed = message.removed
    }
    return obj
  },

  create<I extends Exact<DeepPartial<RemoveWebWorkerResponse>, I>>(
    base?: I,
  ): RemoveWebWorkerResponse {
    return RemoveWebWorkerResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<RemoveWebWorkerResponse>, I>>(
    object: I,
  ): RemoveWebWorkerResponse {
    const message = createBaseRemoveWebWorkerResponse()
    message.removed = object.removed ?? false
    return message
  },
}

/**
 * WebDocumentHost is the API exposed by the Go runtime for WebDocument.
 *
 * Usually accessed by the TypeScript WebDocument controller.
 */
export interface WebDocumentHost {
  /**
   * WebViewRpc opens a stream for a RPC call from a WebView.
   * Exposes the WebViewHost service.
   * Id is the webViewId.
   */
  WebViewRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket>
}

export const WebDocumentHostServiceName = 'web.document.WebDocumentHost'
export class WebDocumentHostClientImpl implements WebDocumentHost {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || WebDocumentHostServiceName
    this.rpc = rpc
    this.WebViewRpc = this.WebViewRpc.bind(this)
  }
  WebViewRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket> {
    const data = RpcStreamPacket.encodeTransform(request)
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      'WebViewRpc',
      data,
      abortSignal || undefined,
    )
    return RpcStreamPacket.decodeTransform(result)
  }
}

/**
 * WebDocumentHost is the API exposed by the Go runtime for WebDocument.
 *
 * Usually accessed by the TypeScript WebDocument controller.
 */
export type WebDocumentHostDefinition = typeof WebDocumentHostDefinition
export const WebDocumentHostDefinition = {
  name: 'WebDocumentHost',
  fullName: 'web.document.WebDocumentHost',
  methods: {
    /**
     * WebViewRpc opens a stream for a RPC call from a WebView.
     * Exposes the WebViewHost service.
     * Id is the webViewId.
     */
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

/**
 * WebDocument is the API exposed by the TypeScript WebDocument managing WebViews.
 * Usually maps to a single Window or Tab.
 */
export interface WebDocument {
  /** WatchWebDocumentStatus returns an initial snapshot of WebViews followed by updates. */
  WatchWebDocumentStatus(
    request: WatchWebDocumentStatusRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WebDocumentStatus>
  /**
   * CreateWebView requests to create a new WebView at the root level.
   * Returns created: false if unable to create WebViews.
   */
  CreateWebView(
    request: CreateWebViewRequest,
    abortSignal?: AbortSignal,
  ): Promise<CreateWebViewResponse>
  /**
   * WebViewRpc opens a stream for a RPC call to a WebView.
   * ID is the webViewId.
   */
  WebViewRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket>
  /**
   * CreateWebWorker requests to spawn a WebWorker with an instance identifier.
   * If a worker already exists with that ID, it will be terminated before starting the new.
   * Returns created: false if unable to create WebWorkers.
   * This usually creates a new SharedWorker.
   * The worker is expected to close itself if it becomes disconnected or broken.
   * The worker is passed a MessagePort that can be used to open WebRuntimeClient.
   */
  CreateWebWorker(
    request: CreateWebWorkerRequest,
    abortSignal?: AbortSignal,
  ): Promise<CreateWebWorkerResponse>
  /** RemoveWebWorker requests to terminate a WebWorker with an instance identifier. */
  RemoveWebWorker(
    request: RemoveWebWorkerRequest,
    abortSignal?: AbortSignal,
  ): Promise<RemoveWebWorkerResponse>
}

export const WebDocumentServiceName = 'web.document.WebDocument'
export class WebDocumentClientImpl implements WebDocument {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || WebDocumentServiceName
    this.rpc = rpc
    this.WatchWebDocumentStatus = this.WatchWebDocumentStatus.bind(this)
    this.CreateWebView = this.CreateWebView.bind(this)
    this.WebViewRpc = this.WebViewRpc.bind(this)
    this.CreateWebWorker = this.CreateWebWorker.bind(this)
    this.RemoveWebWorker = this.RemoveWebWorker.bind(this)
  }
  WatchWebDocumentStatus(
    request: WatchWebDocumentStatusRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WebDocumentStatus> {
    const data = WatchWebDocumentStatusRequest.encode(request).finish()
    const result = this.rpc.serverStreamingRequest(
      this.service,
      'WatchWebDocumentStatus',
      data,
      abortSignal || undefined,
    )
    return WebDocumentStatus.decodeTransform(result)
  }

  CreateWebView(
    request: CreateWebViewRequest,
    abortSignal?: AbortSignal,
  ): Promise<CreateWebViewResponse> {
    const data = CreateWebViewRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'CreateWebView',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      CreateWebViewResponse.decode(_m0.Reader.create(data)),
    )
  }

  WebViewRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket> {
    const data = RpcStreamPacket.encodeTransform(request)
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      'WebViewRpc',
      data,
      abortSignal || undefined,
    )
    return RpcStreamPacket.decodeTransform(result)
  }

  CreateWebWorker(
    request: CreateWebWorkerRequest,
    abortSignal?: AbortSignal,
  ): Promise<CreateWebWorkerResponse> {
    const data = CreateWebWorkerRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'CreateWebWorker',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      CreateWebWorkerResponse.decode(_m0.Reader.create(data)),
    )
  }

  RemoveWebWorker(
    request: RemoveWebWorkerRequest,
    abortSignal?: AbortSignal,
  ): Promise<RemoveWebWorkerResponse> {
    const data = RemoveWebWorkerRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'RemoveWebWorker',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      RemoveWebWorkerResponse.decode(_m0.Reader.create(data)),
    )
  }
}

/**
 * WebDocument is the API exposed by the TypeScript WebDocument managing WebViews.
 * Usually maps to a single Window or Tab.
 */
export type WebDocumentDefinition = typeof WebDocumentDefinition
export const WebDocumentDefinition = {
  name: 'WebDocument',
  fullName: 'web.document.WebDocument',
  methods: {
    /** WatchWebDocumentStatus returns an initial snapshot of WebViews followed by updates. */
    watchWebDocumentStatus: {
      name: 'WatchWebDocumentStatus',
      requestType: WatchWebDocumentStatusRequest,
      requestStream: false,
      responseType: WebDocumentStatus,
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
    /**
     * WebViewRpc opens a stream for a RPC call to a WebView.
     * ID is the webViewId.
     */
    webViewRpc: {
      name: 'WebViewRpc',
      requestType: RpcStreamPacket,
      requestStream: true,
      responseType: RpcStreamPacket,
      responseStream: true,
      options: {},
    },
    /**
     * CreateWebWorker requests to spawn a WebWorker with an instance identifier.
     * If a worker already exists with that ID, it will be terminated before starting the new.
     * Returns created: false if unable to create WebWorkers.
     * This usually creates a new SharedWorker.
     * The worker is expected to close itself if it becomes disconnected or broken.
     * The worker is passed a MessagePort that can be used to open WebRuntimeClient.
     */
    createWebWorker: {
      name: 'CreateWebWorker',
      requestType: CreateWebWorkerRequest,
      requestStream: false,
      responseType: CreateWebWorkerResponse,
      responseStream: false,
      options: {},
    },
    /** RemoveWebWorker requests to terminate a WebWorker with an instance identifier. */
    removeWebWorker: {
      name: 'RemoveWebWorker',
      requestType: RemoveWebWorkerRequest,
      requestStream: false,
      responseType: RemoveWebWorkerResponse,
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
    : T extends globalThis.Array<infer U>
      ? globalThis.Array<DeepPartial<U>>
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
