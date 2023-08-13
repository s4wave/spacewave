/* eslint-disable */
import { RpcStreamPacket } from '@go/github.com/aperturerobotics/starpc/rpcstream/rpcstream.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'web.runtime'

/** WebRuntimeClientType is the set of client types for a WebRuntime. */
export enum WebRuntimeClientType {
  /** WebRuntimeClientType_UNKNOWN - WebRuntimeClientType_UNKNOWN is the unknown type. */
  WebRuntimeClientType_UNKNOWN = 0,
  /** WebRuntimeClientType_WEB_DOCUMENT - WebRuntimeClientType_WEB_DOCUMENT is the WebDocument type. */
  WebRuntimeClientType_WEB_DOCUMENT = 1,
  /** WebRuntimeClientType_SERVICE_WORKER - WebRuntimeClientType_SERVICE_WORKER is the ServiceWorker type. */
  WebRuntimeClientType_SERVICE_WORKER = 2,
  UNRECOGNIZED = -1,
}

export function webRuntimeClientTypeFromJSON(
  object: any,
): WebRuntimeClientType {
  switch (object) {
    case 0:
    case 'WebRuntimeClientType_UNKNOWN':
      return WebRuntimeClientType.WebRuntimeClientType_UNKNOWN
    case 1:
    case 'WebRuntimeClientType_WEB_DOCUMENT':
      return WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT
    case 2:
    case 'WebRuntimeClientType_SERVICE_WORKER':
      return WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER
    case -1:
    case 'UNRECOGNIZED':
    default:
      return WebRuntimeClientType.UNRECOGNIZED
  }
}

export function webRuntimeClientTypeToJSON(
  object: WebRuntimeClientType,
): string {
  switch (object) {
    case WebRuntimeClientType.WebRuntimeClientType_UNKNOWN:
      return 'WebRuntimeClientType_UNKNOWN'
    case WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT:
      return 'WebRuntimeClientType_WEB_DOCUMENT'
    case WebRuntimeClientType.WebRuntimeClientType_SERVICE_WORKER:
      return 'WebRuntimeClientType_SERVICE_WORKER'
    case WebRuntimeClientType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/**
 * WebRuntimeHostInit initializes the WebRuntimeHost.
 *
 * only used in cases where the host runtime is initialized by the web runtime.
 */
export interface WebRuntimeHostInit {
  /**
   * WebRuntimeId is the identifier for the WebRuntime instance.
   *
   * must be set
   */
  webRuntimeId: string
}

/** WatchWebRuntimeStatusRequest is the body of the WatchWebRuntimeStatus request. */
export interface WatchWebRuntimeStatusRequest {}

/** WebRuntimeStatus contains a snapshot of status for a Runtime instance. */
export interface WebRuntimeStatus {
  /** Snapshot indicates this is a full snapshot of the lists. */
  snapshot: boolean
  /** WebDocuments contains the list of web documents. */
  webDocuments: WebDocumentStatus[]
}

/** WebDocumentStatus contains status for a WebDocument. */
export interface WebDocumentStatus {
  /** Id is the unique identifier for the WebDocument. */
  id: string
  /**
   * Deleted indicates the document was just removed.
   * If set, all below fields are ignored.
   */
  deleted: boolean
  /** Permanent indicates that this document cannot be closed. */
  permanent: boolean
}

/** CreateWebDocumentRequest is a request to create a new web view. */
export interface CreateWebDocumentRequest {
  /** id is the identifier for the new WebDocument. */
  id: string
}

/** CreateWebDocumentResponse is the response to the CreateWebDocument request. */
export interface CreateWebDocumentResponse {
  /**
   * Removed indicates the WebDocument was created.
   * If this is not set, assumes we cannot create WebDocuments.
   */
  created: boolean
}

/** RemoveWebDocumentRequest is a request to remove a WebDocument. */
export interface RemoveWebDocumentRequest {
  /** id is the identifier for the WebDocument. */
  id: string
}

/** RemoveWebDocumentResponse is the response to the RemoveWebDocument request. */
export interface RemoveWebDocumentResponse {
  /**
   * Removed indicates the WebDocument was removed.
   * If this is not set, the document did not exist.
   */
  removed: boolean
}

/** WebRuntimeClientInit is a message sent by a client of a WebRuntime. */
export interface WebRuntimeClientInit {
  /**
   * RuntimeId is the shared identifier for the Go Runtime instance.
   *
   * must be set
   */
  webRuntimeId: string
  /**
   * ClientUuid is the identifier of the client.
   *
   * must be set
   */
  clientUuid: string
  /** ClientType is the type of the client. */
  clientType: WebRuntimeClientType
}

function createBaseWebRuntimeHostInit(): WebRuntimeHostInit {
  return { webRuntimeId: '' }
}

export const WebRuntimeHostInit = {
  encode(
    message: WebRuntimeHostInit,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.webRuntimeId !== '') {
      writer.uint32(10).string(message.webRuntimeId)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WebRuntimeHostInit {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWebRuntimeHostInit()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.webRuntimeId = reader.string()
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
  // Transform<WebRuntimeHostInit, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WebRuntimeHostInit | WebRuntimeHostInit[]>
      | Iterable<WebRuntimeHostInit | WebRuntimeHostInit[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WebRuntimeHostInit.encode(p).finish()]
        }
      } else {
        yield* [WebRuntimeHostInit.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WebRuntimeHostInit>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WebRuntimeHostInit> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WebRuntimeHostInit.decode(p)]
        }
      } else {
        yield* [WebRuntimeHostInit.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): WebRuntimeHostInit {
    return {
      webRuntimeId: isSet(object.webRuntimeId)
        ? String(object.webRuntimeId)
        : '',
    }
  },

  toJSON(message: WebRuntimeHostInit): unknown {
    const obj: any = {}
    if (message.webRuntimeId !== '') {
      obj.webRuntimeId = message.webRuntimeId
    }
    return obj
  },

  create<I extends Exact<DeepPartial<WebRuntimeHostInit>, I>>(
    base?: I,
  ): WebRuntimeHostInit {
    return WebRuntimeHostInit.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<WebRuntimeHostInit>, I>>(
    object: I,
  ): WebRuntimeHostInit {
    const message = createBaseWebRuntimeHostInit()
    message.webRuntimeId = object.webRuntimeId ?? ''
    return message
  },
}

function createBaseWatchWebRuntimeStatusRequest(): WatchWebRuntimeStatusRequest {
  return {}
}

export const WatchWebRuntimeStatusRequest = {
  encode(
    _: WatchWebRuntimeStatusRequest,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): WatchWebRuntimeStatusRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWatchWebRuntimeStatusRequest()
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
  // Transform<WatchWebRuntimeStatusRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<
          WatchWebRuntimeStatusRequest | WatchWebRuntimeStatusRequest[]
        >
      | Iterable<WatchWebRuntimeStatusRequest | WatchWebRuntimeStatusRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WatchWebRuntimeStatusRequest.encode(p).finish()]
        }
      } else {
        yield* [WatchWebRuntimeStatusRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WatchWebRuntimeStatusRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WatchWebRuntimeStatusRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WatchWebRuntimeStatusRequest.decode(p)]
        }
      } else {
        yield* [WatchWebRuntimeStatusRequest.decode(pkt)]
      }
    }
  },

  fromJSON(_: any): WatchWebRuntimeStatusRequest {
    return {}
  },

  toJSON(_: WatchWebRuntimeStatusRequest): unknown {
    const obj: any = {}
    return obj
  },

  create<I extends Exact<DeepPartial<WatchWebRuntimeStatusRequest>, I>>(
    base?: I,
  ): WatchWebRuntimeStatusRequest {
    return WatchWebRuntimeStatusRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<WatchWebRuntimeStatusRequest>, I>>(
    _: I,
  ): WatchWebRuntimeStatusRequest {
    const message = createBaseWatchWebRuntimeStatusRequest()
    return message
  },
}

function createBaseWebRuntimeStatus(): WebRuntimeStatus {
  return { snapshot: false, webDocuments: [] }
}

export const WebRuntimeStatus = {
  encode(
    message: WebRuntimeStatus,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.snapshot === true) {
      writer.uint32(8).bool(message.snapshot)
    }
    for (const v of message.webDocuments) {
      WebDocumentStatus.encode(v!, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WebRuntimeStatus {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWebRuntimeStatus()
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

          message.webDocuments.push(
            WebDocumentStatus.decode(reader, reader.uint32()),
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
  // Transform<WebRuntimeStatus, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WebRuntimeStatus | WebRuntimeStatus[]>
      | Iterable<WebRuntimeStatus | WebRuntimeStatus[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WebRuntimeStatus.encode(p).finish()]
        }
      } else {
        yield* [WebRuntimeStatus.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WebRuntimeStatus>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WebRuntimeStatus> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WebRuntimeStatus.decode(p)]
        }
      } else {
        yield* [WebRuntimeStatus.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): WebRuntimeStatus {
    return {
      snapshot: isSet(object.snapshot) ? Boolean(object.snapshot) : false,
      webDocuments: Array.isArray(object?.webDocuments)
        ? object.webDocuments.map((e: any) => WebDocumentStatus.fromJSON(e))
        : [],
    }
  },

  toJSON(message: WebRuntimeStatus): unknown {
    const obj: any = {}
    if (message.snapshot === true) {
      obj.snapshot = message.snapshot
    }
    if (message.webDocuments?.length) {
      obj.webDocuments = message.webDocuments.map((e) =>
        WebDocumentStatus.toJSON(e),
      )
    }
    return obj
  },

  create<I extends Exact<DeepPartial<WebRuntimeStatus>, I>>(
    base?: I,
  ): WebRuntimeStatus {
    return WebRuntimeStatus.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<WebRuntimeStatus>, I>>(
    object: I,
  ): WebRuntimeStatus {
    const message = createBaseWebRuntimeStatus()
    message.snapshot = object.snapshot ?? false
    message.webDocuments =
      object.webDocuments?.map((e) => WebDocumentStatus.fromPartial(e)) || []
    return message
  },
}

function createBaseWebDocumentStatus(): WebDocumentStatus {
  return { id: '', deleted: false, permanent: false }
}

export const WebDocumentStatus = {
  encode(
    message: WebDocumentStatus,
    writer: _m0.Writer = _m0.Writer.create(),
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

  decode(input: _m0.Reader | Uint8Array, length?: number): WebDocumentStatus {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWebDocumentStatus()
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
  // Transform<WebDocumentStatus, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WebDocumentStatus | WebDocumentStatus[]>
      | Iterable<WebDocumentStatus | WebDocumentStatus[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WebDocumentStatus.encode(p).finish()]
        }
      } else {
        yield* [WebDocumentStatus.encode(pkt).finish()]
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
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WebDocumentStatus.decode(p)]
        }
      } else {
        yield* [WebDocumentStatus.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): WebDocumentStatus {
    return {
      id: isSet(object.id) ? String(object.id) : '',
      deleted: isSet(object.deleted) ? Boolean(object.deleted) : false,
      permanent: isSet(object.permanent) ? Boolean(object.permanent) : false,
    }
  },

  toJSON(message: WebDocumentStatus): unknown {
    const obj: any = {}
    if (message.id !== '') {
      obj.id = message.id
    }
    if (message.deleted === true) {
      obj.deleted = message.deleted
    }
    if (message.permanent === true) {
      obj.permanent = message.permanent
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
    message.id = object.id ?? ''
    message.deleted = object.deleted ?? false
    message.permanent = object.permanent ?? false
    return message
  },
}

function createBaseCreateWebDocumentRequest(): CreateWebDocumentRequest {
  return { id: '' }
}

export const CreateWebDocumentRequest = {
  encode(
    message: CreateWebDocumentRequest,
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
  ): CreateWebDocumentRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseCreateWebDocumentRequest()
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
  // Transform<CreateWebDocumentRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<CreateWebDocumentRequest | CreateWebDocumentRequest[]>
      | Iterable<CreateWebDocumentRequest | CreateWebDocumentRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [CreateWebDocumentRequest.encode(p).finish()]
        }
      } else {
        yield* [CreateWebDocumentRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, CreateWebDocumentRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<CreateWebDocumentRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [CreateWebDocumentRequest.decode(p)]
        }
      } else {
        yield* [CreateWebDocumentRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): CreateWebDocumentRequest {
    return { id: isSet(object.id) ? String(object.id) : '' }
  },

  toJSON(message: CreateWebDocumentRequest): unknown {
    const obj: any = {}
    if (message.id !== '') {
      obj.id = message.id
    }
    return obj
  },

  create<I extends Exact<DeepPartial<CreateWebDocumentRequest>, I>>(
    base?: I,
  ): CreateWebDocumentRequest {
    return CreateWebDocumentRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<CreateWebDocumentRequest>, I>>(
    object: I,
  ): CreateWebDocumentRequest {
    const message = createBaseCreateWebDocumentRequest()
    message.id = object.id ?? ''
    return message
  },
}

function createBaseCreateWebDocumentResponse(): CreateWebDocumentResponse {
  return { created: false }
}

export const CreateWebDocumentResponse = {
  encode(
    message: CreateWebDocumentResponse,
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
  ): CreateWebDocumentResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseCreateWebDocumentResponse()
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
  // Transform<CreateWebDocumentResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<CreateWebDocumentResponse | CreateWebDocumentResponse[]>
      | Iterable<CreateWebDocumentResponse | CreateWebDocumentResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [CreateWebDocumentResponse.encode(p).finish()]
        }
      } else {
        yield* [CreateWebDocumentResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, CreateWebDocumentResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<CreateWebDocumentResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [CreateWebDocumentResponse.decode(p)]
        }
      } else {
        yield* [CreateWebDocumentResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): CreateWebDocumentResponse {
    return { created: isSet(object.created) ? Boolean(object.created) : false }
  },

  toJSON(message: CreateWebDocumentResponse): unknown {
    const obj: any = {}
    if (message.created === true) {
      obj.created = message.created
    }
    return obj
  },

  create<I extends Exact<DeepPartial<CreateWebDocumentResponse>, I>>(
    base?: I,
  ): CreateWebDocumentResponse {
    return CreateWebDocumentResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<CreateWebDocumentResponse>, I>>(
    object: I,
  ): CreateWebDocumentResponse {
    const message = createBaseCreateWebDocumentResponse()
    message.created = object.created ?? false
    return message
  },
}

function createBaseRemoveWebDocumentRequest(): RemoveWebDocumentRequest {
  return { id: '' }
}

export const RemoveWebDocumentRequest = {
  encode(
    message: RemoveWebDocumentRequest,
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
  ): RemoveWebDocumentRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRemoveWebDocumentRequest()
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
  // Transform<RemoveWebDocumentRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RemoveWebDocumentRequest | RemoveWebDocumentRequest[]>
      | Iterable<RemoveWebDocumentRequest | RemoveWebDocumentRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoveWebDocumentRequest.encode(p).finish()]
        }
      } else {
        yield* [RemoveWebDocumentRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RemoveWebDocumentRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RemoveWebDocumentRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoveWebDocumentRequest.decode(p)]
        }
      } else {
        yield* [RemoveWebDocumentRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): RemoveWebDocumentRequest {
    return { id: isSet(object.id) ? String(object.id) : '' }
  },

  toJSON(message: RemoveWebDocumentRequest): unknown {
    const obj: any = {}
    if (message.id !== '') {
      obj.id = message.id
    }
    return obj
  },

  create<I extends Exact<DeepPartial<RemoveWebDocumentRequest>, I>>(
    base?: I,
  ): RemoveWebDocumentRequest {
    return RemoveWebDocumentRequest.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<RemoveWebDocumentRequest>, I>>(
    object: I,
  ): RemoveWebDocumentRequest {
    const message = createBaseRemoveWebDocumentRequest()
    message.id = object.id ?? ''
    return message
  },
}

function createBaseRemoveWebDocumentResponse(): RemoveWebDocumentResponse {
  return { removed: false }
}

export const RemoveWebDocumentResponse = {
  encode(
    message: RemoveWebDocumentResponse,
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
  ): RemoveWebDocumentResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRemoveWebDocumentResponse()
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
  // Transform<RemoveWebDocumentResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RemoveWebDocumentResponse | RemoveWebDocumentResponse[]>
      | Iterable<RemoveWebDocumentResponse | RemoveWebDocumentResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoveWebDocumentResponse.encode(p).finish()]
        }
      } else {
        yield* [RemoveWebDocumentResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RemoveWebDocumentResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RemoveWebDocumentResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoveWebDocumentResponse.decode(p)]
        }
      } else {
        yield* [RemoveWebDocumentResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): RemoveWebDocumentResponse {
    return { removed: isSet(object.removed) ? Boolean(object.removed) : false }
  },

  toJSON(message: RemoveWebDocumentResponse): unknown {
    const obj: any = {}
    if (message.removed === true) {
      obj.removed = message.removed
    }
    return obj
  },

  create<I extends Exact<DeepPartial<RemoveWebDocumentResponse>, I>>(
    base?: I,
  ): RemoveWebDocumentResponse {
    return RemoveWebDocumentResponse.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<RemoveWebDocumentResponse>, I>>(
    object: I,
  ): RemoveWebDocumentResponse {
    const message = createBaseRemoveWebDocumentResponse()
    message.removed = object.removed ?? false
    return message
  },
}

function createBaseWebRuntimeClientInit(): WebRuntimeClientInit {
  return { webRuntimeId: '', clientUuid: '', clientType: 0 }
}

export const WebRuntimeClientInit = {
  encode(
    message: WebRuntimeClientInit,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.webRuntimeId !== '') {
      writer.uint32(10).string(message.webRuntimeId)
    }
    if (message.clientUuid !== '') {
      writer.uint32(18).string(message.clientUuid)
    }
    if (message.clientType !== 0) {
      writer.uint32(24).int32(message.clientType)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): WebRuntimeClientInit {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWebRuntimeClientInit()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.webRuntimeId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.clientUuid = reader.string()
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.clientType = reader.int32() as any
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
  // Transform<WebRuntimeClientInit, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WebRuntimeClientInit | WebRuntimeClientInit[]>
      | Iterable<WebRuntimeClientInit | WebRuntimeClientInit[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WebRuntimeClientInit.encode(p).finish()]
        }
      } else {
        yield* [WebRuntimeClientInit.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WebRuntimeClientInit>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WebRuntimeClientInit> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WebRuntimeClientInit.decode(p)]
        }
      } else {
        yield* [WebRuntimeClientInit.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): WebRuntimeClientInit {
    return {
      webRuntimeId: isSet(object.webRuntimeId)
        ? String(object.webRuntimeId)
        : '',
      clientUuid: isSet(object.clientUuid) ? String(object.clientUuid) : '',
      clientType: isSet(object.clientType)
        ? webRuntimeClientTypeFromJSON(object.clientType)
        : 0,
    }
  },

  toJSON(message: WebRuntimeClientInit): unknown {
    const obj: any = {}
    if (message.webRuntimeId !== '') {
      obj.webRuntimeId = message.webRuntimeId
    }
    if (message.clientUuid !== '') {
      obj.clientUuid = message.clientUuid
    }
    if (message.clientType !== 0) {
      obj.clientType = webRuntimeClientTypeToJSON(message.clientType)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<WebRuntimeClientInit>, I>>(
    base?: I,
  ): WebRuntimeClientInit {
    return WebRuntimeClientInit.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<WebRuntimeClientInit>, I>>(
    object: I,
  ): WebRuntimeClientInit {
    const message = createBaseWebRuntimeClientInit()
    message.webRuntimeId = object.webRuntimeId ?? ''
    message.clientUuid = object.clientUuid ?? ''
    message.clientType = object.clientType ?? 0
    return message
  },
}

/**
 * WebRuntimeHost is the API exposed by the Go runtime to the WebRuntime.
 *
 * Usually accessed by the WebRuntime.
 */
export interface WebRuntimeHost {
  /**
   * WebDocumentRpc opens a stream for a RPC call to a WebDocument.
   * Exposes the WebDocument service.
   * Id is the webDocumentId.
   */
  WebDocumentRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket>
  /**
   * ServiceWorkerRpc opens a stream for a RPC call from the ServiceWorker.
   * Exposes the ServiceWorkerHost service.
   * Id is the service worker id.
   */
  ServiceWorkerRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket>
}

export const WebRuntimeHostServiceName = 'web.runtime.WebRuntimeHost'
export class WebRuntimeHostClientImpl implements WebRuntimeHost {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || WebRuntimeHostServiceName
    this.rpc = rpc
    this.WebDocumentRpc = this.WebDocumentRpc.bind(this)
    this.ServiceWorkerRpc = this.ServiceWorkerRpc.bind(this)
  }
  WebDocumentRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket> {
    const data = RpcStreamPacket.encodeTransform(request)
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      'WebDocumentRpc',
      data,
      abortSignal || undefined,
    )
    return RpcStreamPacket.decodeTransform(result)
  }

  ServiceWorkerRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket> {
    const data = RpcStreamPacket.encodeTransform(request)
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      'ServiceWorkerRpc',
      data,
      abortSignal || undefined,
    )
    return RpcStreamPacket.decodeTransform(result)
  }
}

/**
 * WebRuntimeHost is the API exposed by the Go runtime to the WebRuntime.
 *
 * Usually accessed by the WebRuntime.
 */
export type WebRuntimeHostDefinition = typeof WebRuntimeHostDefinition
export const WebRuntimeHostDefinition = {
  name: 'WebRuntimeHost',
  fullName: 'web.runtime.WebRuntimeHost',
  methods: {
    /**
     * WebDocumentRpc opens a stream for a RPC call to a WebDocument.
     * Exposes the WebDocument service.
     * Id is the webDocumentId.
     */
    webDocumentRpc: {
      name: 'WebDocumentRpc',
      requestType: RpcStreamPacket,
      requestStream: true,
      responseType: RpcStreamPacket,
      responseStream: true,
      options: {},
    },
    /**
     * ServiceWorkerRpc opens a stream for a RPC call from the ServiceWorker.
     * Exposes the ServiceWorkerHost service.
     * Id is the service worker id.
     */
    serviceWorkerRpc: {
      name: 'ServiceWorkerRpc',
      requestType: RpcStreamPacket,
      requestStream: true,
      responseType: RpcStreamPacket,
      responseStream: true,
      options: {},
    },
  },
} as const

/**
 * WebRuntime is the API exposed by the TypeScript WebRuntime managing WebDocument.
 *
 * Usually accessed by the WebRuntimeHost.
 */
export interface WebRuntime {
  /** WatchWebRuntimeStatus returns an initial snapshot of WebRuntimes followed by updates. */
  WatchWebRuntimeStatus(
    request: WatchWebRuntimeStatusRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WebRuntimeStatus>
  /**
   * CreateWebDocument requests to create a new WebDocument.
   * Returns created: false if unable to create WebDocuments.
   * This usually creates a new Tab or Window.
   */
  CreateWebDocument(
    request: CreateWebDocumentRequest,
    abortSignal?: AbortSignal,
  ): Promise<CreateWebDocumentResponse>
  /**
   * RemoveWebDocument requests to delete a WebDocument.
   * Returns created: false if unable to create WebDocuments.
   * This usually creates a new Tab or Window.
   */
  RemoveWebDocument(
    request: RemoveWebDocumentRequest,
    abortSignal?: AbortSignal,
  ): Promise<RemoveWebDocumentResponse>
  /**
   * WebDocumentRpc opens a stream for a RPC call to a WebDocument.
   * Exposes the WebDocument service.
   * Id is the webDocumentId.
   */
  WebDocumentRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket>
}

export const WebRuntimeServiceName = 'web.runtime.WebRuntime'
export class WebRuntimeClientImpl implements WebRuntime {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || WebRuntimeServiceName
    this.rpc = rpc
    this.WatchWebRuntimeStatus = this.WatchWebRuntimeStatus.bind(this)
    this.CreateWebDocument = this.CreateWebDocument.bind(this)
    this.RemoveWebDocument = this.RemoveWebDocument.bind(this)
    this.WebDocumentRpc = this.WebDocumentRpc.bind(this)
  }
  WatchWebRuntimeStatus(
    request: WatchWebRuntimeStatusRequest,
    abortSignal?: AbortSignal,
  ): AsyncIterable<WebRuntimeStatus> {
    const data = WatchWebRuntimeStatusRequest.encode(request).finish()
    const result = this.rpc.serverStreamingRequest(
      this.service,
      'WatchWebRuntimeStatus',
      data,
      abortSignal || undefined,
    )
    return WebRuntimeStatus.decodeTransform(result)
  }

  CreateWebDocument(
    request: CreateWebDocumentRequest,
    abortSignal?: AbortSignal,
  ): Promise<CreateWebDocumentResponse> {
    const data = CreateWebDocumentRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'CreateWebDocument',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      CreateWebDocumentResponse.decode(_m0.Reader.create(data)),
    )
  }

  RemoveWebDocument(
    request: RemoveWebDocumentRequest,
    abortSignal?: AbortSignal,
  ): Promise<RemoveWebDocumentResponse> {
    const data = RemoveWebDocumentRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'RemoveWebDocument',
      data,
      abortSignal || undefined,
    )
    return promise.then((data) =>
      RemoveWebDocumentResponse.decode(_m0.Reader.create(data)),
    )
  }

  WebDocumentRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal,
  ): AsyncIterable<RpcStreamPacket> {
    const data = RpcStreamPacket.encodeTransform(request)
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      'WebDocumentRpc',
      data,
      abortSignal || undefined,
    )
    return RpcStreamPacket.decodeTransform(result)
  }
}

/**
 * WebRuntime is the API exposed by the TypeScript WebRuntime managing WebDocument.
 *
 * Usually accessed by the WebRuntimeHost.
 */
export type WebRuntimeDefinition = typeof WebRuntimeDefinition
export const WebRuntimeDefinition = {
  name: 'WebRuntime',
  fullName: 'web.runtime.WebRuntime',
  methods: {
    /** WatchWebRuntimeStatus returns an initial snapshot of WebRuntimes followed by updates. */
    watchWebRuntimeStatus: {
      name: 'WatchWebRuntimeStatus',
      requestType: WatchWebRuntimeStatusRequest,
      requestStream: false,
      responseType: WebRuntimeStatus,
      responseStream: true,
      options: {},
    },
    /**
     * CreateWebDocument requests to create a new WebDocument.
     * Returns created: false if unable to create WebDocuments.
     * This usually creates a new Tab or Window.
     */
    createWebDocument: {
      name: 'CreateWebDocument',
      requestType: CreateWebDocumentRequest,
      requestStream: false,
      responseType: CreateWebDocumentResponse,
      responseStream: false,
      options: {},
    },
    /**
     * RemoveWebDocument requests to delete a WebDocument.
     * Returns created: false if unable to create WebDocuments.
     * This usually creates a new Tab or Window.
     */
    removeWebDocument: {
      name: 'RemoveWebDocument',
      requestType: RemoveWebDocumentRequest,
      requestStream: false,
      responseType: RemoveWebDocumentResponse,
      responseStream: false,
      options: {},
    },
    /**
     * WebDocumentRpc opens a stream for a RPC call to a WebDocument.
     * Exposes the WebDocument service.
     * Id is the webDocumentId.
     */
    webDocumentRpc: {
      name: 'WebDocumentRpc',
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
