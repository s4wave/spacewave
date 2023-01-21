/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'web.fetch'

/**
 * FetchRequest is the message to initialize a Fetch request.
 *
 * Note: many fields are optional.
 */
export interface FetchRequest {
  body?:
    | { $case: 'requestInfo'; requestInfo: FetchRequestInfo }
    | {
        $case: 'requestData'
        requestData: FetchRequestData
      }
}

/** FetchRequestInfo contains all information about the request excluding the body. */
export interface FetchRequestInfo {
  /**
   * Method is the request method.
   * i.e. "GET"
   */
  method: string
  /** Url is the request url. */
  url: string
  /** Headers is the map of request header key/value pairs. */
  headers: { [key: string]: string }
  /** HasBody indicates there will be follow up Data packets. */
  hasBody: boolean
  /** ClientId is the identifier of the client that sent the request. */
  clientId: string
  /**
   * Destination is the kind of resource requested.
   *
   * "audio" | "audioworklet" | "document" | "embed" | "font" | "frame" |
   * "iframe" | "image" | "manifest" | "object" | "paintworklet" | "report" |
   * "script" | "sharedworker" | "style" | "track" | "video" | "worker" | "xslt"
   */
  destination: string
  /**
   * Integrity contains a cryptographic hash of the resource being fetched.
   *
   * optional
   */
  integrity: string
  /**
   * Mode is the request mode, indicating if CORS should be allowed or not.
   *
   * "cors" | "navigate" | "no-cors" | "same-origin"
   * defaults to "cors"
   */
  mode: string
  /**
   * Redirect indicates the redirect policy for the request.
   *
   * "error" | "follow" | "manual"
   * defaults to "follow"
   */
  redirect: string
  /** Referrer is the request referrer. */
  referrer: string
  /**
   * ReferrerPolicy is the request referrer policy.
   *
   * "" | "no-referrer" | "no-referrer-when-downgrade" | "origin" |
   * "origin-when-cross-origin" | "same-origin" | "strict-origin" |
   * "strict-origin-when-cross-origin" | "unsafe-url"
   */
  referrerPolicy: string
}

export interface FetchRequestInfo_HeadersEntry {
  key: string
  value: string
}

/** FetchRequestData contains a streaming request data packet. */
export interface FetchRequestData {
  /** Data is the request data chunk. */
  data: Uint8Array
  /** Done indicates the stream is closed after data. */
  done: boolean
}

/**
 * FetchResponse is a message in a Fetch response stream.
 *
 * The first message in the stream has the ResponseInfo
 * Subsequent messages contain response data.
 */
export interface FetchResponse {
  body?:
    | { $case: 'responseInfo'; responseInfo: ResponseInfo }
    | { $case: 'responseData'; responseData: ResponseData }
}

/** ResponseInfo contains information about the response. */
export interface ResponseInfo {
  /** Headers is the map of response header key/value pairs. */
  headers: { [key: string]: string }
  /** Ok indicates if the response was ok. */
  ok: boolean
  /** Redirected indicates if the request was redirected */
  redirected: boolean
  /** Status is the HTTP status code. */
  status: number
  /** StatusText is the HTTP status text. */
  statusText: string
  /**
   * ResponseType is the type of response.
   * "basic" | "cors" | "default" | "error" | "opaque" | "opaqueredirect"
   */
  responseType: string
}

export interface ResponseInfo_HeadersEntry {
  key: string
  value: string
}

/** ResponseData contains a streaming response data packet. */
export interface ResponseData {
  /** Data is the response data chunk. */
  data: Uint8Array
  /** Done indicates the stream is closed after data. */
  done: boolean
}

function createBaseFetchRequest(): FetchRequest {
  return { body: undefined }
}

export const FetchRequest = {
  encode(
    message: FetchRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.body?.$case === 'requestInfo') {
      FetchRequestInfo.encode(
        message.body.requestInfo,
        writer.uint32(10).fork()
      ).ldelim()
    }
    if (message.body?.$case === 'requestData') {
      FetchRequestData.encode(
        message.body.requestData,
        writer.uint32(18).fork()
      ).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FetchRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFetchRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.body = {
            $case: 'requestInfo',
            requestInfo: FetchRequestInfo.decode(reader, reader.uint32()),
          }
          break
        case 2:
          message.body = {
            $case: 'requestData',
            requestData: FetchRequestData.decode(reader, reader.uint32()),
          }
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FetchRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FetchRequest | FetchRequest[]>
      | Iterable<FetchRequest | FetchRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FetchRequest.encode(p).finish()]
        }
      } else {
        yield* [FetchRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FetchRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<FetchRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FetchRequest.decode(p)]
        }
      } else {
        yield* [FetchRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): FetchRequest {
    return {
      body: isSet(object.requestInfo)
        ? {
            $case: 'requestInfo',
            requestInfo: FetchRequestInfo.fromJSON(object.requestInfo),
          }
        : isSet(object.requestData)
        ? {
            $case: 'requestData',
            requestData: FetchRequestData.fromJSON(object.requestData),
          }
        : undefined,
    }
  },

  toJSON(message: FetchRequest): unknown {
    const obj: any = {}
    message.body?.$case === 'requestInfo' &&
      (obj.requestInfo = message.body?.requestInfo
        ? FetchRequestInfo.toJSON(message.body?.requestInfo)
        : undefined)
    message.body?.$case === 'requestData' &&
      (obj.requestData = message.body?.requestData
        ? FetchRequestData.toJSON(message.body?.requestData)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<FetchRequest>, I>>(
    base?: I
  ): FetchRequest {
    return FetchRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<FetchRequest>, I>>(
    object: I
  ): FetchRequest {
    const message = createBaseFetchRequest()
    if (
      object.body?.$case === 'requestInfo' &&
      object.body?.requestInfo !== undefined &&
      object.body?.requestInfo !== null
    ) {
      message.body = {
        $case: 'requestInfo',
        requestInfo: FetchRequestInfo.fromPartial(object.body.requestInfo),
      }
    }
    if (
      object.body?.$case === 'requestData' &&
      object.body?.requestData !== undefined &&
      object.body?.requestData !== null
    ) {
      message.body = {
        $case: 'requestData',
        requestData: FetchRequestData.fromPartial(object.body.requestData),
      }
    }
    return message
  },
}

function createBaseFetchRequestInfo(): FetchRequestInfo {
  return {
    method: '',
    url: '',
    headers: {},
    hasBody: false,
    clientId: '',
    destination: '',
    integrity: '',
    mode: '',
    redirect: '',
    referrer: '',
    referrerPolicy: '',
  }
}

export const FetchRequestInfo = {
  encode(
    message: FetchRequestInfo,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.method !== '') {
      writer.uint32(10).string(message.method)
    }
    if (message.url !== '') {
      writer.uint32(18).string(message.url)
    }
    Object.entries(message.headers).forEach(([key, value]) => {
      FetchRequestInfo_HeadersEntry.encode(
        { key: key as any, value },
        writer.uint32(26).fork()
      ).ldelim()
    })
    if (message.hasBody === true) {
      writer.uint32(32).bool(message.hasBody)
    }
    if (message.clientId !== '') {
      writer.uint32(42).string(message.clientId)
    }
    if (message.destination !== '') {
      writer.uint32(50).string(message.destination)
    }
    if (message.integrity !== '') {
      writer.uint32(58).string(message.integrity)
    }
    if (message.mode !== '') {
      writer.uint32(66).string(message.mode)
    }
    if (message.redirect !== '') {
      writer.uint32(74).string(message.redirect)
    }
    if (message.referrer !== '') {
      writer.uint32(82).string(message.referrer)
    }
    if (message.referrerPolicy !== '') {
      writer.uint32(90).string(message.referrerPolicy)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FetchRequestInfo {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFetchRequestInfo()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.method = reader.string()
          break
        case 2:
          message.url = reader.string()
          break
        case 3:
          const entry3 = FetchRequestInfo_HeadersEntry.decode(
            reader,
            reader.uint32()
          )
          if (entry3.value !== undefined) {
            message.headers[entry3.key] = entry3.value
          }
          break
        case 4:
          message.hasBody = reader.bool()
          break
        case 5:
          message.clientId = reader.string()
          break
        case 6:
          message.destination = reader.string()
          break
        case 7:
          message.integrity = reader.string()
          break
        case 8:
          message.mode = reader.string()
          break
        case 9:
          message.redirect = reader.string()
          break
        case 10:
          message.referrer = reader.string()
          break
        case 11:
          message.referrerPolicy = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FetchRequestInfo, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FetchRequestInfo | FetchRequestInfo[]>
      | Iterable<FetchRequestInfo | FetchRequestInfo[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FetchRequestInfo.encode(p).finish()]
        }
      } else {
        yield* [FetchRequestInfo.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FetchRequestInfo>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<FetchRequestInfo> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FetchRequestInfo.decode(p)]
        }
      } else {
        yield* [FetchRequestInfo.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): FetchRequestInfo {
    return {
      method: isSet(object.method) ? String(object.method) : '',
      url: isSet(object.url) ? String(object.url) : '',
      headers: isObject(object.headers)
        ? Object.entries(object.headers).reduce<{ [key: string]: string }>(
            (acc, [key, value]) => {
              acc[key] = String(value)
              return acc
            },
            {}
          )
        : {},
      hasBody: isSet(object.hasBody) ? Boolean(object.hasBody) : false,
      clientId: isSet(object.clientId) ? String(object.clientId) : '',
      destination: isSet(object.destination) ? String(object.destination) : '',
      integrity: isSet(object.integrity) ? String(object.integrity) : '',
      mode: isSet(object.mode) ? String(object.mode) : '',
      redirect: isSet(object.redirect) ? String(object.redirect) : '',
      referrer: isSet(object.referrer) ? String(object.referrer) : '',
      referrerPolicy: isSet(object.referrerPolicy)
        ? String(object.referrerPolicy)
        : '',
    }
  },

  toJSON(message: FetchRequestInfo): unknown {
    const obj: any = {}
    message.method !== undefined && (obj.method = message.method)
    message.url !== undefined && (obj.url = message.url)
    obj.headers = {}
    if (message.headers) {
      Object.entries(message.headers).forEach(([k, v]) => {
        obj.headers[k] = v
      })
    }
    message.hasBody !== undefined && (obj.hasBody = message.hasBody)
    message.clientId !== undefined && (obj.clientId = message.clientId)
    message.destination !== undefined && (obj.destination = message.destination)
    message.integrity !== undefined && (obj.integrity = message.integrity)
    message.mode !== undefined && (obj.mode = message.mode)
    message.redirect !== undefined && (obj.redirect = message.redirect)
    message.referrer !== undefined && (obj.referrer = message.referrer)
    message.referrerPolicy !== undefined &&
      (obj.referrerPolicy = message.referrerPolicy)
    return obj
  },

  create<I extends Exact<DeepPartial<FetchRequestInfo>, I>>(
    base?: I
  ): FetchRequestInfo {
    return FetchRequestInfo.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<FetchRequestInfo>, I>>(
    object: I
  ): FetchRequestInfo {
    const message = createBaseFetchRequestInfo()
    message.method = object.method ?? ''
    message.url = object.url ?? ''
    message.headers = Object.entries(object.headers ?? {}).reduce<{
      [key: string]: string
    }>((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = String(value)
      }
      return acc
    }, {})
    message.hasBody = object.hasBody ?? false
    message.clientId = object.clientId ?? ''
    message.destination = object.destination ?? ''
    message.integrity = object.integrity ?? ''
    message.mode = object.mode ?? ''
    message.redirect = object.redirect ?? ''
    message.referrer = object.referrer ?? ''
    message.referrerPolicy = object.referrerPolicy ?? ''
    return message
  },
}

function createBaseFetchRequestInfo_HeadersEntry(): FetchRequestInfo_HeadersEntry {
  return { key: '', value: '' }
}

export const FetchRequestInfo_HeadersEntry = {
  encode(
    message: FetchRequestInfo_HeadersEntry,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.key !== '') {
      writer.uint32(10).string(message.key)
    }
    if (message.value !== '') {
      writer.uint32(18).string(message.value)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): FetchRequestInfo_HeadersEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFetchRequestInfo_HeadersEntry()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.key = reader.string()
          break
        case 2:
          message.value = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FetchRequestInfo_HeadersEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<
          FetchRequestInfo_HeadersEntry | FetchRequestInfo_HeadersEntry[]
        >
      | Iterable<
          FetchRequestInfo_HeadersEntry | FetchRequestInfo_HeadersEntry[]
        >
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FetchRequestInfo_HeadersEntry.encode(p).finish()]
        }
      } else {
        yield* [FetchRequestInfo_HeadersEntry.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FetchRequestInfo_HeadersEntry>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<FetchRequestInfo_HeadersEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FetchRequestInfo_HeadersEntry.decode(p)]
        }
      } else {
        yield* [FetchRequestInfo_HeadersEntry.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): FetchRequestInfo_HeadersEntry {
    return {
      key: isSet(object.key) ? String(object.key) : '',
      value: isSet(object.value) ? String(object.value) : '',
    }
  },

  toJSON(message: FetchRequestInfo_HeadersEntry): unknown {
    const obj: any = {}
    message.key !== undefined && (obj.key = message.key)
    message.value !== undefined && (obj.value = message.value)
    return obj
  },

  create<I extends Exact<DeepPartial<FetchRequestInfo_HeadersEntry>, I>>(
    base?: I
  ): FetchRequestInfo_HeadersEntry {
    return FetchRequestInfo_HeadersEntry.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<FetchRequestInfo_HeadersEntry>, I>>(
    object: I
  ): FetchRequestInfo_HeadersEntry {
    const message = createBaseFetchRequestInfo_HeadersEntry()
    message.key = object.key ?? ''
    message.value = object.value ?? ''
    return message
  },
}

function createBaseFetchRequestData(): FetchRequestData {
  return { data: new Uint8Array(), done: false }
}

export const FetchRequestData = {
  encode(
    message: FetchRequestData,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.data.length !== 0) {
      writer.uint32(10).bytes(message.data)
    }
    if (message.done === true) {
      writer.uint32(16).bool(message.done)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FetchRequestData {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFetchRequestData()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.data = reader.bytes()
          break
        case 2:
          message.done = reader.bool()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FetchRequestData, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FetchRequestData | FetchRequestData[]>
      | Iterable<FetchRequestData | FetchRequestData[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FetchRequestData.encode(p).finish()]
        }
      } else {
        yield* [FetchRequestData.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FetchRequestData>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<FetchRequestData> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FetchRequestData.decode(p)]
        }
      } else {
        yield* [FetchRequestData.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): FetchRequestData {
    return {
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(),
      done: isSet(object.done) ? Boolean(object.done) : false,
    }
  },

  toJSON(message: FetchRequestData): unknown {
    const obj: any = {}
    message.data !== undefined &&
      (obj.data = base64FromBytes(
        message.data !== undefined ? message.data : new Uint8Array()
      ))
    message.done !== undefined && (obj.done = message.done)
    return obj
  },

  create<I extends Exact<DeepPartial<FetchRequestData>, I>>(
    base?: I
  ): FetchRequestData {
    return FetchRequestData.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<FetchRequestData>, I>>(
    object: I
  ): FetchRequestData {
    const message = createBaseFetchRequestData()
    message.data = object.data ?? new Uint8Array()
    message.done = object.done ?? false
    return message
  },
}

function createBaseFetchResponse(): FetchResponse {
  return { body: undefined }
}

export const FetchResponse = {
  encode(
    message: FetchResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.body?.$case === 'responseInfo') {
      ResponseInfo.encode(
        message.body.responseInfo,
        writer.uint32(10).fork()
      ).ldelim()
    }
    if (message.body?.$case === 'responseData') {
      ResponseData.encode(
        message.body.responseData,
        writer.uint32(18).fork()
      ).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): FetchResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseFetchResponse()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.body = {
            $case: 'responseInfo',
            responseInfo: ResponseInfo.decode(reader, reader.uint32()),
          }
          break
        case 2:
          message.body = {
            $case: 'responseData',
            responseData: ResponseData.decode(reader, reader.uint32()),
          }
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<FetchResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<FetchResponse | FetchResponse[]>
      | Iterable<FetchResponse | FetchResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FetchResponse.encode(p).finish()]
        }
      } else {
        yield* [FetchResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, FetchResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<FetchResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [FetchResponse.decode(p)]
        }
      } else {
        yield* [FetchResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): FetchResponse {
    return {
      body: isSet(object.responseInfo)
        ? {
            $case: 'responseInfo',
            responseInfo: ResponseInfo.fromJSON(object.responseInfo),
          }
        : isSet(object.responseData)
        ? {
            $case: 'responseData',
            responseData: ResponseData.fromJSON(object.responseData),
          }
        : undefined,
    }
  },

  toJSON(message: FetchResponse): unknown {
    const obj: any = {}
    message.body?.$case === 'responseInfo' &&
      (obj.responseInfo = message.body?.responseInfo
        ? ResponseInfo.toJSON(message.body?.responseInfo)
        : undefined)
    message.body?.$case === 'responseData' &&
      (obj.responseData = message.body?.responseData
        ? ResponseData.toJSON(message.body?.responseData)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<FetchResponse>, I>>(
    base?: I
  ): FetchResponse {
    return FetchResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<FetchResponse>, I>>(
    object: I
  ): FetchResponse {
    const message = createBaseFetchResponse()
    if (
      object.body?.$case === 'responseInfo' &&
      object.body?.responseInfo !== undefined &&
      object.body?.responseInfo !== null
    ) {
      message.body = {
        $case: 'responseInfo',
        responseInfo: ResponseInfo.fromPartial(object.body.responseInfo),
      }
    }
    if (
      object.body?.$case === 'responseData' &&
      object.body?.responseData !== undefined &&
      object.body?.responseData !== null
    ) {
      message.body = {
        $case: 'responseData',
        responseData: ResponseData.fromPartial(object.body.responseData),
      }
    }
    return message
  },
}

function createBaseResponseInfo(): ResponseInfo {
  return {
    headers: {},
    ok: false,
    redirected: false,
    status: 0,
    statusText: '',
    responseType: '',
  }
}

export const ResponseInfo = {
  encode(
    message: ResponseInfo,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    Object.entries(message.headers).forEach(([key, value]) => {
      ResponseInfo_HeadersEntry.encode(
        { key: key as any, value },
        writer.uint32(10).fork()
      ).ldelim()
    })
    if (message.ok === true) {
      writer.uint32(16).bool(message.ok)
    }
    if (message.redirected === true) {
      writer.uint32(24).bool(message.redirected)
    }
    if (message.status !== 0) {
      writer.uint32(32).uint32(message.status)
    }
    if (message.statusText !== '') {
      writer.uint32(42).string(message.statusText)
    }
    if (message.responseType !== '') {
      writer.uint32(50).string(message.responseType)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ResponseInfo {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseResponseInfo()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          const entry1 = ResponseInfo_HeadersEntry.decode(
            reader,
            reader.uint32()
          )
          if (entry1.value !== undefined) {
            message.headers[entry1.key] = entry1.value
          }
          break
        case 2:
          message.ok = reader.bool()
          break
        case 3:
          message.redirected = reader.bool()
          break
        case 4:
          message.status = reader.uint32()
          break
        case 5:
          message.statusText = reader.string()
          break
        case 6:
          message.responseType = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ResponseInfo, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ResponseInfo | ResponseInfo[]>
      | Iterable<ResponseInfo | ResponseInfo[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ResponseInfo.encode(p).finish()]
        }
      } else {
        yield* [ResponseInfo.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ResponseInfo>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ResponseInfo> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ResponseInfo.decode(p)]
        }
      } else {
        yield* [ResponseInfo.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ResponseInfo {
    return {
      headers: isObject(object.headers)
        ? Object.entries(object.headers).reduce<{ [key: string]: string }>(
            (acc, [key, value]) => {
              acc[key] = String(value)
              return acc
            },
            {}
          )
        : {},
      ok: isSet(object.ok) ? Boolean(object.ok) : false,
      redirected: isSet(object.redirected) ? Boolean(object.redirected) : false,
      status: isSet(object.status) ? Number(object.status) : 0,
      statusText: isSet(object.statusText) ? String(object.statusText) : '',
      responseType: isSet(object.responseType)
        ? String(object.responseType)
        : '',
    }
  },

  toJSON(message: ResponseInfo): unknown {
    const obj: any = {}
    obj.headers = {}
    if (message.headers) {
      Object.entries(message.headers).forEach(([k, v]) => {
        obj.headers[k] = v
      })
    }
    message.ok !== undefined && (obj.ok = message.ok)
    message.redirected !== undefined && (obj.redirected = message.redirected)
    message.status !== undefined && (obj.status = Math.round(message.status))
    message.statusText !== undefined && (obj.statusText = message.statusText)
    message.responseType !== undefined &&
      (obj.responseType = message.responseType)
    return obj
  },

  create<I extends Exact<DeepPartial<ResponseInfo>, I>>(
    base?: I
  ): ResponseInfo {
    return ResponseInfo.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ResponseInfo>, I>>(
    object: I
  ): ResponseInfo {
    const message = createBaseResponseInfo()
    message.headers = Object.entries(object.headers ?? {}).reduce<{
      [key: string]: string
    }>((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = String(value)
      }
      return acc
    }, {})
    message.ok = object.ok ?? false
    message.redirected = object.redirected ?? false
    message.status = object.status ?? 0
    message.statusText = object.statusText ?? ''
    message.responseType = object.responseType ?? ''
    return message
  },
}

function createBaseResponseInfo_HeadersEntry(): ResponseInfo_HeadersEntry {
  return { key: '', value: '' }
}

export const ResponseInfo_HeadersEntry = {
  encode(
    message: ResponseInfo_HeadersEntry,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.key !== '') {
      writer.uint32(10).string(message.key)
    }
    if (message.value !== '') {
      writer.uint32(18).string(message.value)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): ResponseInfo_HeadersEntry {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseResponseInfo_HeadersEntry()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.key = reader.string()
          break
        case 2:
          message.value = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ResponseInfo_HeadersEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ResponseInfo_HeadersEntry | ResponseInfo_HeadersEntry[]>
      | Iterable<ResponseInfo_HeadersEntry | ResponseInfo_HeadersEntry[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ResponseInfo_HeadersEntry.encode(p).finish()]
        }
      } else {
        yield* [ResponseInfo_HeadersEntry.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ResponseInfo_HeadersEntry>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ResponseInfo_HeadersEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ResponseInfo_HeadersEntry.decode(p)]
        }
      } else {
        yield* [ResponseInfo_HeadersEntry.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ResponseInfo_HeadersEntry {
    return {
      key: isSet(object.key) ? String(object.key) : '',
      value: isSet(object.value) ? String(object.value) : '',
    }
  },

  toJSON(message: ResponseInfo_HeadersEntry): unknown {
    const obj: any = {}
    message.key !== undefined && (obj.key = message.key)
    message.value !== undefined && (obj.value = message.value)
    return obj
  },

  create<I extends Exact<DeepPartial<ResponseInfo_HeadersEntry>, I>>(
    base?: I
  ): ResponseInfo_HeadersEntry {
    return ResponseInfo_HeadersEntry.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ResponseInfo_HeadersEntry>, I>>(
    object: I
  ): ResponseInfo_HeadersEntry {
    const message = createBaseResponseInfo_HeadersEntry()
    message.key = object.key ?? ''
    message.value = object.value ?? ''
    return message
  },
}

function createBaseResponseData(): ResponseData {
  return { data: new Uint8Array(), done: false }
}

export const ResponseData = {
  encode(
    message: ResponseData,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.data.length !== 0) {
      writer.uint32(10).bytes(message.data)
    }
    if (message.done === true) {
      writer.uint32(16).bool(message.done)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ResponseData {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseResponseData()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.data = reader.bytes()
          break
        case 2:
          message.done = reader.bool()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ResponseData, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ResponseData | ResponseData[]>
      | Iterable<ResponseData | ResponseData[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ResponseData.encode(p).finish()]
        }
      } else {
        yield* [ResponseData.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ResponseData>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ResponseData> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ResponseData.decode(p)]
        }
      } else {
        yield* [ResponseData.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ResponseData {
    return {
      data: isSet(object.data)
        ? bytesFromBase64(object.data)
        : new Uint8Array(),
      done: isSet(object.done) ? Boolean(object.done) : false,
    }
  },

  toJSON(message: ResponseData): unknown {
    const obj: any = {}
    message.data !== undefined &&
      (obj.data = base64FromBytes(
        message.data !== undefined ? message.data : new Uint8Array()
      ))
    message.done !== undefined && (obj.done = message.done)
    return obj
  },

  create<I extends Exact<DeepPartial<ResponseData>, I>>(
    base?: I
  ): ResponseData {
    return ResponseData.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ResponseData>, I>>(
    object: I
  ): ResponseData {
    const message = createBaseResponseData()
    message.data = object.data ?? new Uint8Array()
    message.done = object.done ?? false
    return message
  },
}

/** FetchService is a host which can service Fetch requests. */
export interface FetchService {
  /** Fetch performs a Fetch request with a streaming response. */
  Fetch(
    request: AsyncIterable<FetchRequest>,
    abortSignal?: AbortSignal
  ): AsyncIterable<FetchResponse>
}

export class FetchServiceClientImpl implements FetchService {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || 'web.fetch.FetchService'
    this.rpc = rpc
    this.Fetch = this.Fetch.bind(this)
  }
  Fetch(
    request: AsyncIterable<FetchRequest>,
    abortSignal?: AbortSignal
  ): AsyncIterable<FetchResponse> {
    const data = FetchRequest.encodeTransform(request)
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      'Fetch',
      data,
      abortSignal || undefined
    )
    return FetchResponse.decodeTransform(result)
  }
}

/** FetchService is a host which can service Fetch requests. */
export type FetchServiceDefinition = typeof FetchServiceDefinition
export const FetchServiceDefinition = {
  name: 'FetchService',
  fullName: 'web.fetch.FetchService',
  methods: {
    /** Fetch performs a Fetch request with a streaming response. */
    fetch: {
      name: 'Fetch',
      requestType: FetchRequest,
      requestStream: true,
      responseType: FetchResponse,
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

declare var self: any | undefined
declare var window: any | undefined
declare var global: any | undefined
var tsProtoGlobalThis: any = (() => {
  if (typeof globalThis !== 'undefined') {
    return globalThis
  }
  if (typeof self !== 'undefined') {
    return self
  }
  if (typeof window !== 'undefined') {
    return window
  }
  if (typeof global !== 'undefined') {
    return global
  }
  throw 'Unable to locate global object'
})()

function bytesFromBase64(b64: string): Uint8Array {
  if (tsProtoGlobalThis.Buffer) {
    return Uint8Array.from(tsProtoGlobalThis.Buffer.from(b64, 'base64'))
  } else {
    const bin = tsProtoGlobalThis.atob(b64)
    const arr = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; ++i) {
      arr[i] = bin.charCodeAt(i)
    }
    return arr
  }
}

function base64FromBytes(arr: Uint8Array): string {
  if (tsProtoGlobalThis.Buffer) {
    return tsProtoGlobalThis.Buffer.from(arr).toString('base64')
  } else {
    const bin: string[] = []
    arr.forEach((byte) => {
      bin.push(String.fromCharCode(byte))
    })
    return tsProtoGlobalThis.btoa(bin.join(''))
  }
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

function isObject(value: any): boolean {
  return typeof value === 'object' && value !== null
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
