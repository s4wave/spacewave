/* eslint-disable */
import { RpcStreamPacket } from '@go/github.com/aperturerobotics/starpc/rpcstream/rpcstream.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'web.view'

/** RenderMode is the list of available WebView rendering modes. */
export enum RenderMode {
  /**
   * RenderMode_NONE - RenderMode_NONE selects no renderer (no contents).
   * When setting this mode, everything is "reset" in the view.
   */
  RenderMode_NONE = 0,
  /**
   * RenderMode_REACT_COMPONENT - RenderMode_REACT_COMPONENT renders a React component from a JS module.
   * Renders the default export of the JS module.
   */
  RenderMode_REACT_COMPONENT = 1,
  /**
   * RenderMode_FUNCTION - RenderMode_FUNCTION renders an init function with the following type:
   * (parent: HTMLDivElement) => (() => void)
   * Callback returns a function to call to shutdown the script.
   * Renders the default export of the JS module.
   */
  RenderMode_FUNCTION = 2,
  UNRECOGNIZED = -1,
}

export function renderModeFromJSON(object: any): RenderMode {
  switch (object) {
    case 0:
    case 'RenderMode_NONE':
      return RenderMode.RenderMode_NONE
    case 1:
    case 'RenderMode_REACT_COMPONENT':
      return RenderMode.RenderMode_REACT_COMPONENT
    case 2:
    case 'RenderMode_FUNCTION':
      return RenderMode.RenderMode_FUNCTION
    case -1:
    case 'UNRECOGNIZED':
    default:
      return RenderMode.UNRECOGNIZED
  }
}

export function renderModeToJSON(object: RenderMode): string {
  switch (object) {
    case RenderMode.RenderMode_NONE:
      return 'RenderMode_NONE'
    case RenderMode.RenderMode_REACT_COMPONENT:
      return 'RenderMode_REACT_COMPONENT'
    case RenderMode.RenderMode_FUNCTION:
      return 'RenderMode_FUNCTION'
    case RenderMode.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** SetRenderModeRequest is the request to change the render mode. */
export interface SetRenderModeRequest {
  /** RenderMode is the new render mode. */
  renderMode: RenderMode
  /**
   * Wait waits for the mode to become active before returning.
   * If loading a script: will wait for the script to load successfully.
   * If any error is encountered, returns it as the RPC result.
   */
  wait: boolean
  /**
   * ScriptPath is a path to a script to load to render.
   * RenderMode_REACT_COMPONENT: expects default export to be a Component.
   * RenderMode_FUNCTION: expects default export to be a function.
   */
  scriptPath: string
  /**
   * Props is an object passed as properties to the renderer/component.
   * RenderMode_REACT_COMPONENT: parsed as JSON & passed as the React props for the component.
   * RenderMode_FUNCTION: passed to the mount function as a Uint8Array.
   */
  props: Uint8Array
}

/** SetRenderModeResponse is the response to the SetRenderMode request. */
export interface SetRenderModeResponse {}

/** SetHtmlLinksRequest is the request to set a list of HtmlLink */
export interface SetHtmlLinksRequest {
  /** Clear clears the list of links before setting html_links. */
  clear: boolean
  /** Remove is the set of HTML link keys to remove. */
  remove: string[]
  /** SetLinks is the set of HTML links to add. */
  setLinks: { [key: string]: HtmlLink }
}

export interface SetHtmlLinksRequest_SetLinksEntry {
  key: string
  value: HtmlLink | undefined
}

/** HtmlLink is a html link element for loading css & other resources. */
export interface HtmlLink {
  /** Href is the URL to load. */
  href: string
  /**
   * Rel is the type of link this is.
   * Usually "stylesheet"
   */
  rel: string
}

/** SetHtmlLinksResponse is the response to the SetHtmlLinks request. */
export interface SetHtmlLinksResponse {}

/** RemoveWebViewRequest is a request to remove the web view. */
export interface RemoveWebViewRequest {}

/** RemoveWebViewResponse is the response to the RemoveWebView request. */
export interface RemoveWebViewResponse {
  /** Removed indicates the web view was removed. */
  removed: boolean
}

function createBaseSetRenderModeRequest(): SetRenderModeRequest {
  return {
    renderMode: 0,
    wait: false,
    scriptPath: '',
    props: new Uint8Array(0),
  }
}

export const SetRenderModeRequest = {
  encode(
    message: SetRenderModeRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.renderMode !== 0) {
      writer.uint32(8).int32(message.renderMode)
    }
    if (message.wait === true) {
      writer.uint32(16).bool(message.wait)
    }
    if (message.scriptPath !== '') {
      writer.uint32(26).string(message.scriptPath)
    }
    if (message.props.length !== 0) {
      writer.uint32(34).bytes(message.props)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): SetRenderModeRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseSetRenderModeRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.renderMode = reader.int32() as any
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.wait = reader.bool()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.scriptPath = reader.string()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.props = reader.bytes()
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
  // Transform<SetRenderModeRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<SetRenderModeRequest | SetRenderModeRequest[]>
      | Iterable<SetRenderModeRequest | SetRenderModeRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SetRenderModeRequest.encode(p).finish()]
        }
      } else {
        yield* [SetRenderModeRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, SetRenderModeRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<SetRenderModeRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SetRenderModeRequest.decode(p)]
        }
      } else {
        yield* [SetRenderModeRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): SetRenderModeRequest {
    return {
      renderMode: isSet(object.renderMode)
        ? renderModeFromJSON(object.renderMode)
        : 0,
      wait: isSet(object.wait) ? Boolean(object.wait) : false,
      scriptPath: isSet(object.scriptPath) ? String(object.scriptPath) : '',
      props: isSet(object.props)
        ? bytesFromBase64(object.props)
        : new Uint8Array(0),
    }
  },

  toJSON(message: SetRenderModeRequest): unknown {
    const obj: any = {}
    message.renderMode !== undefined &&
      (obj.renderMode = renderModeToJSON(message.renderMode))
    message.wait !== undefined && (obj.wait = message.wait)
    message.scriptPath !== undefined && (obj.scriptPath = message.scriptPath)
    message.props !== undefined &&
      (obj.props = base64FromBytes(
        message.props !== undefined ? message.props : new Uint8Array(0)
      ))
    return obj
  },

  create<I extends Exact<DeepPartial<SetRenderModeRequest>, I>>(
    base?: I
  ): SetRenderModeRequest {
    return SetRenderModeRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<SetRenderModeRequest>, I>>(
    object: I
  ): SetRenderModeRequest {
    const message = createBaseSetRenderModeRequest()
    message.renderMode = object.renderMode ?? 0
    message.wait = object.wait ?? false
    message.scriptPath = object.scriptPath ?? ''
    message.props = object.props ?? new Uint8Array(0)
    return message
  },
}

function createBaseSetRenderModeResponse(): SetRenderModeResponse {
  return {}
}

export const SetRenderModeResponse = {
  encode(
    _: SetRenderModeResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): SetRenderModeResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseSetRenderModeResponse()
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
  // Transform<SetRenderModeResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<SetRenderModeResponse | SetRenderModeResponse[]>
      | Iterable<SetRenderModeResponse | SetRenderModeResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SetRenderModeResponse.encode(p).finish()]
        }
      } else {
        yield* [SetRenderModeResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, SetRenderModeResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<SetRenderModeResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SetRenderModeResponse.decode(p)]
        }
      } else {
        yield* [SetRenderModeResponse.decode(pkt)]
      }
    }
  },

  fromJSON(_: any): SetRenderModeResponse {
    return {}
  },

  toJSON(_: SetRenderModeResponse): unknown {
    const obj: any = {}
    return obj
  },

  create<I extends Exact<DeepPartial<SetRenderModeResponse>, I>>(
    base?: I
  ): SetRenderModeResponse {
    return SetRenderModeResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<SetRenderModeResponse>, I>>(
    _: I
  ): SetRenderModeResponse {
    const message = createBaseSetRenderModeResponse()
    return message
  },
}

function createBaseSetHtmlLinksRequest(): SetHtmlLinksRequest {
  return { clear: false, remove: [], setLinks: {} }
}

export const SetHtmlLinksRequest = {
  encode(
    message: SetHtmlLinksRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.clear === true) {
      writer.uint32(8).bool(message.clear)
    }
    for (const v of message.remove) {
      writer.uint32(18).string(v!)
    }
    Object.entries(message.setLinks).forEach(([key, value]) => {
      SetHtmlLinksRequest_SetLinksEntry.encode(
        { key: key as any, value },
        writer.uint32(26).fork()
      ).ldelim()
    })
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): SetHtmlLinksRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseSetHtmlLinksRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.clear = reader.bool()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.remove.push(reader.string())
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          const entry3 = SetHtmlLinksRequest_SetLinksEntry.decode(
            reader,
            reader.uint32()
          )
          if (entry3.value !== undefined) {
            message.setLinks[entry3.key] = entry3.value
          }
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
  // Transform<SetHtmlLinksRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<SetHtmlLinksRequest | SetHtmlLinksRequest[]>
      | Iterable<SetHtmlLinksRequest | SetHtmlLinksRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SetHtmlLinksRequest.encode(p).finish()]
        }
      } else {
        yield* [SetHtmlLinksRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, SetHtmlLinksRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<SetHtmlLinksRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SetHtmlLinksRequest.decode(p)]
        }
      } else {
        yield* [SetHtmlLinksRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): SetHtmlLinksRequest {
    return {
      clear: isSet(object.clear) ? Boolean(object.clear) : false,
      remove: Array.isArray(object?.remove)
        ? object.remove.map((e: any) => String(e))
        : [],
      setLinks: isObject(object.setLinks)
        ? Object.entries(object.setLinks).reduce<{ [key: string]: HtmlLink }>(
            (acc, [key, value]) => {
              acc[key] = HtmlLink.fromJSON(value)
              return acc
            },
            {}
          )
        : {},
    }
  },

  toJSON(message: SetHtmlLinksRequest): unknown {
    const obj: any = {}
    message.clear !== undefined && (obj.clear = message.clear)
    if (message.remove) {
      obj.remove = message.remove.map((e) => e)
    } else {
      obj.remove = []
    }
    obj.setLinks = {}
    if (message.setLinks) {
      Object.entries(message.setLinks).forEach(([k, v]) => {
        obj.setLinks[k] = HtmlLink.toJSON(v)
      })
    }
    return obj
  },

  create<I extends Exact<DeepPartial<SetHtmlLinksRequest>, I>>(
    base?: I
  ): SetHtmlLinksRequest {
    return SetHtmlLinksRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<SetHtmlLinksRequest>, I>>(
    object: I
  ): SetHtmlLinksRequest {
    const message = createBaseSetHtmlLinksRequest()
    message.clear = object.clear ?? false
    message.remove = object.remove?.map((e) => e) || []
    message.setLinks = Object.entries(object.setLinks ?? {}).reduce<{
      [key: string]: HtmlLink
    }>((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = HtmlLink.fromPartial(value)
      }
      return acc
    }, {})
    return message
  },
}

function createBaseSetHtmlLinksRequest_SetLinksEntry(): SetHtmlLinksRequest_SetLinksEntry {
  return { key: '', value: undefined }
}

export const SetHtmlLinksRequest_SetLinksEntry = {
  encode(
    message: SetHtmlLinksRequest_SetLinksEntry,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.key !== '') {
      writer.uint32(10).string(message.key)
    }
    if (message.value !== undefined) {
      HtmlLink.encode(message.value, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): SetHtmlLinksRequest_SetLinksEntry {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseSetHtmlLinksRequest_SetLinksEntry()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.key = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.value = HtmlLink.decode(reader, reader.uint32())
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
  // Transform<SetHtmlLinksRequest_SetLinksEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<
          | SetHtmlLinksRequest_SetLinksEntry
          | SetHtmlLinksRequest_SetLinksEntry[]
        >
      | Iterable<
          | SetHtmlLinksRequest_SetLinksEntry
          | SetHtmlLinksRequest_SetLinksEntry[]
        >
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SetHtmlLinksRequest_SetLinksEntry.encode(p).finish()]
        }
      } else {
        yield* [SetHtmlLinksRequest_SetLinksEntry.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, SetHtmlLinksRequest_SetLinksEntry>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<SetHtmlLinksRequest_SetLinksEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SetHtmlLinksRequest_SetLinksEntry.decode(p)]
        }
      } else {
        yield* [SetHtmlLinksRequest_SetLinksEntry.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): SetHtmlLinksRequest_SetLinksEntry {
    return {
      key: isSet(object.key) ? String(object.key) : '',
      value: isSet(object.value) ? HtmlLink.fromJSON(object.value) : undefined,
    }
  },

  toJSON(message: SetHtmlLinksRequest_SetLinksEntry): unknown {
    const obj: any = {}
    message.key !== undefined && (obj.key = message.key)
    message.value !== undefined &&
      (obj.value = message.value ? HtmlLink.toJSON(message.value) : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<SetHtmlLinksRequest_SetLinksEntry>, I>>(
    base?: I
  ): SetHtmlLinksRequest_SetLinksEntry {
    return SetHtmlLinksRequest_SetLinksEntry.fromPartial(base ?? {})
  },

  fromPartial<
    I extends Exact<DeepPartial<SetHtmlLinksRequest_SetLinksEntry>, I>
  >(object: I): SetHtmlLinksRequest_SetLinksEntry {
    const message = createBaseSetHtmlLinksRequest_SetLinksEntry()
    message.key = object.key ?? ''
    message.value =
      object.value !== undefined && object.value !== null
        ? HtmlLink.fromPartial(object.value)
        : undefined
    return message
  },
}

function createBaseHtmlLink(): HtmlLink {
  return { href: '', rel: '' }
}

export const HtmlLink = {
  encode(
    message: HtmlLink,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.href !== '') {
      writer.uint32(10).string(message.href)
    }
    if (message.rel !== '') {
      writer.uint32(18).string(message.rel)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): HtmlLink {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseHtmlLink()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.href = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.rel = reader.string()
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
  // Transform<HtmlLink, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<HtmlLink | HtmlLink[]>
      | Iterable<HtmlLink | HtmlLink[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HtmlLink.encode(p).finish()]
        }
      } else {
        yield* [HtmlLink.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, HtmlLink>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<HtmlLink> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [HtmlLink.decode(p)]
        }
      } else {
        yield* [HtmlLink.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): HtmlLink {
    return {
      href: isSet(object.href) ? String(object.href) : '',
      rel: isSet(object.rel) ? String(object.rel) : '',
    }
  },

  toJSON(message: HtmlLink): unknown {
    const obj: any = {}
    message.href !== undefined && (obj.href = message.href)
    message.rel !== undefined && (obj.rel = message.rel)
    return obj
  },

  create<I extends Exact<DeepPartial<HtmlLink>, I>>(base?: I): HtmlLink {
    return HtmlLink.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<HtmlLink>, I>>(object: I): HtmlLink {
    const message = createBaseHtmlLink()
    message.href = object.href ?? ''
    message.rel = object.rel ?? ''
    return message
  },
}

function createBaseSetHtmlLinksResponse(): SetHtmlLinksResponse {
  return {}
}

export const SetHtmlLinksResponse = {
  encode(
    _: SetHtmlLinksResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): SetHtmlLinksResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseSetHtmlLinksResponse()
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
  // Transform<SetHtmlLinksResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<SetHtmlLinksResponse | SetHtmlLinksResponse[]>
      | Iterable<SetHtmlLinksResponse | SetHtmlLinksResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SetHtmlLinksResponse.encode(p).finish()]
        }
      } else {
        yield* [SetHtmlLinksResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, SetHtmlLinksResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<SetHtmlLinksResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SetHtmlLinksResponse.decode(p)]
        }
      } else {
        yield* [SetHtmlLinksResponse.decode(pkt)]
      }
    }
  },

  fromJSON(_: any): SetHtmlLinksResponse {
    return {}
  },

  toJSON(_: SetHtmlLinksResponse): unknown {
    const obj: any = {}
    return obj
  },

  create<I extends Exact<DeepPartial<SetHtmlLinksResponse>, I>>(
    base?: I
  ): SetHtmlLinksResponse {
    return SetHtmlLinksResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<SetHtmlLinksResponse>, I>>(
    _: I
  ): SetHtmlLinksResponse {
    const message = createBaseSetHtmlLinksResponse()
    return message
  },
}

function createBaseRemoveWebViewRequest(): RemoveWebViewRequest {
  return {}
}

export const RemoveWebViewRequest = {
  encode(
    _: RemoveWebViewRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): RemoveWebViewRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRemoveWebViewRequest()
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
  // Transform<RemoveWebViewRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RemoveWebViewRequest | RemoveWebViewRequest[]>
      | Iterable<RemoveWebViewRequest | RemoveWebViewRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoveWebViewRequest.encode(p).finish()]
        }
      } else {
        yield* [RemoveWebViewRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RemoveWebViewRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<RemoveWebViewRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoveWebViewRequest.decode(p)]
        }
      } else {
        yield* [RemoveWebViewRequest.decode(pkt)]
      }
    }
  },

  fromJSON(_: any): RemoveWebViewRequest {
    return {}
  },

  toJSON(_: RemoveWebViewRequest): unknown {
    const obj: any = {}
    return obj
  },

  create<I extends Exact<DeepPartial<RemoveWebViewRequest>, I>>(
    base?: I
  ): RemoveWebViewRequest {
    return RemoveWebViewRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<RemoveWebViewRequest>, I>>(
    _: I
  ): RemoveWebViewRequest {
    const message = createBaseRemoveWebViewRequest()
    return message
  },
}

function createBaseRemoveWebViewResponse(): RemoveWebViewResponse {
  return { removed: false }
}

export const RemoveWebViewResponse = {
  encode(
    message: RemoveWebViewResponse,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.removed === true) {
      writer.uint32(8).bool(message.removed)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): RemoveWebViewResponse {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRemoveWebViewResponse()
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
  // Transform<RemoveWebViewResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RemoveWebViewResponse | RemoveWebViewResponse[]>
      | Iterable<RemoveWebViewResponse | RemoveWebViewResponse[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoveWebViewResponse.encode(p).finish()]
        }
      } else {
        yield* [RemoveWebViewResponse.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RemoveWebViewResponse>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<RemoveWebViewResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoveWebViewResponse.decode(p)]
        }
      } else {
        yield* [RemoveWebViewResponse.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): RemoveWebViewResponse {
    return { removed: isSet(object.removed) ? Boolean(object.removed) : false }
  },

  toJSON(message: RemoveWebViewResponse): unknown {
    const obj: any = {}
    message.removed !== undefined && (obj.removed = message.removed)
    return obj
  },

  create<I extends Exact<DeepPartial<RemoveWebViewResponse>, I>>(
    base?: I
  ): RemoveWebViewResponse {
    return RemoveWebViewResponse.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<RemoveWebViewResponse>, I>>(
    object: I
  ): RemoveWebViewResponse {
    const message = createBaseRemoveWebViewResponse()
    message.removed = object.removed ?? false
    return message
  },
}

/**
 * WebViewHost is the service exposed by the Go runtime.
 *
 * Accessed by the WebView renderer.
 */
export interface WebViewHost {}

export const WebViewHostServiceName = 'web.view.WebViewHost'
export class WebViewHostClientImpl implements WebViewHost {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || WebViewHostServiceName
    this.rpc = rpc
  }
}

/**
 * WebViewHost is the service exposed by the Go runtime.
 *
 * Accessed by the WebView renderer.
 */
export type WebViewHostDefinition = typeof WebViewHostDefinition
export const WebViewHostDefinition = {
  name: 'WebViewHost',
  fullName: 'web.view.WebViewHost',
  methods: {},
} as const

/** WebView exposes a remote WebView via rpc. */
export interface WebView {
  /** SetRenderMode sets the rendering mode of the view. */
  SetRenderMode(
    request: SetRenderModeRequest,
    abortSignal?: AbortSignal
  ): Promise<SetRenderModeResponse>
  /** SetHtmlLinks sets a list of HTML Links (i.e. css bundles) to load. */
  SetHtmlLinks(
    request: SetHtmlLinksRequest,
    abortSignal?: AbortSignal
  ): Promise<SetHtmlLinksResponse>
  /** RemoveWebView requests to remove a WebView from the root level. */
  RemoveWebView(
    request: RemoveWebViewRequest,
    abortSignal?: AbortSignal
  ): Promise<RemoveWebViewResponse>
}

export const WebViewServiceName = 'web.view.WebView'
export class WebViewClientImpl implements WebView {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || WebViewServiceName
    this.rpc = rpc
    this.SetRenderMode = this.SetRenderMode.bind(this)
    this.SetHtmlLinks = this.SetHtmlLinks.bind(this)
    this.RemoveWebView = this.RemoveWebView.bind(this)
  }
  SetRenderMode(
    request: SetRenderModeRequest,
    abortSignal?: AbortSignal
  ): Promise<SetRenderModeResponse> {
    const data = SetRenderModeRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'SetRenderMode',
      data,
      abortSignal || undefined
    )
    return promise.then((data) =>
      SetRenderModeResponse.decode(_m0.Reader.create(data))
    )
  }

  SetHtmlLinks(
    request: SetHtmlLinksRequest,
    abortSignal?: AbortSignal
  ): Promise<SetHtmlLinksResponse> {
    const data = SetHtmlLinksRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'SetHtmlLinks',
      data,
      abortSignal || undefined
    )
    return promise.then((data) =>
      SetHtmlLinksResponse.decode(_m0.Reader.create(data))
    )
  }

  RemoveWebView(
    request: RemoveWebViewRequest,
    abortSignal?: AbortSignal
  ): Promise<RemoveWebViewResponse> {
    const data = RemoveWebViewRequest.encode(request).finish()
    const promise = this.rpc.request(
      this.service,
      'RemoveWebView',
      data,
      abortSignal || undefined
    )
    return promise.then((data) =>
      RemoveWebViewResponse.decode(_m0.Reader.create(data))
    )
  }
}

/** WebView exposes a remote WebView via rpc. */
export type WebViewDefinition = typeof WebViewDefinition
export const WebViewDefinition = {
  name: 'WebView',
  fullName: 'web.view.WebView',
  methods: {
    /** SetRenderMode sets the rendering mode of the view. */
    setRenderMode: {
      name: 'SetRenderMode',
      requestType: SetRenderModeRequest,
      requestStream: false,
      responseType: SetRenderModeResponse,
      responseStream: false,
      options: {},
    },
    /** SetHtmlLinks sets a list of HTML Links (i.e. css bundles) to load. */
    setHtmlLinks: {
      name: 'SetHtmlLinks',
      requestType: SetHtmlLinksRequest,
      requestStream: false,
      responseType: SetHtmlLinksResponse,
      responseStream: false,
      options: {},
    },
    /** RemoveWebView requests to remove a WebView from the root level. */
    removeWebView: {
      name: 'RemoveWebView',
      requestType: RemoveWebViewRequest,
      requestStream: false,
      responseType: RemoveWebViewResponse,
      responseStream: false,
      options: {},
    },
  },
} as const

/** AccessWebViews implements accessing WebViews via RPC. */
export interface AccessWebViews {
  /**
   * WebViewRpc accesses the WebView service for a view by ID.
   * Id: web view id
   */
  WebViewRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal
  ): AsyncIterable<RpcStreamPacket>
}

export const AccessWebViewsServiceName = 'web.view.AccessWebViews'
export class AccessWebViewsClientImpl implements AccessWebViews {
  private readonly rpc: Rpc
  private readonly service: string
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || AccessWebViewsServiceName
    this.rpc = rpc
    this.WebViewRpc = this.WebViewRpc.bind(this)
  }
  WebViewRpc(
    request: AsyncIterable<RpcStreamPacket>,
    abortSignal?: AbortSignal
  ): AsyncIterable<RpcStreamPacket> {
    const data = RpcStreamPacket.encodeTransform(request)
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      'WebViewRpc',
      data,
      abortSignal || undefined
    )
    return RpcStreamPacket.decodeTransform(result)
  }
}

/** AccessWebViews implements accessing WebViews via RPC. */
export type AccessWebViewsDefinition = typeof AccessWebViewsDefinition
export const AccessWebViewsDefinition = {
  name: 'AccessWebViews',
  fullName: 'web.view.AccessWebViews',
  methods: {
    /**
     * WebViewRpc accesses the WebView service for a view by ID.
     * Id: web view id
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
