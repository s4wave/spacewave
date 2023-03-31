/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'bldr.web.plugin.handle_web_view'

/**
 * Config configures a controller to forward HandleWebView to a plugin.
 * Loads webPluginId with LoadPlugin and uses the RPC client.
 * webPluginId should correspond to a bldr/web/plugin plugin.
 * Resolves the HandleWebView directive on the web plugin bus.
 */
export interface Config {
  /** WebPluginId is the id of the web plugin. */
  webPluginId: string
  /** HandlePluginId is the plugin the web plugin should send WebViews to. */
  handlePluginId: string
  /**
   * WebViewidRegex is the regex of web view IDs to handle with handlePluginId.
   * If empty, will forward any.
   */
  webViewIdRegex: string
}

function createBaseConfig(): Config {
  return { webPluginId: '', handlePluginId: '', webViewIdRegex: '' }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.webPluginId !== '') {
      writer.uint32(10).string(message.webPluginId)
    }
    if (message.handlePluginId !== '') {
      writer.uint32(18).string(message.handlePluginId)
    }
    if (message.webViewIdRegex !== '') {
      writer.uint32(26).string(message.webViewIdRegex)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseConfig()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break
          }

          message.webPluginId = reader.string()
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.handlePluginId = reader.string()
          continue
        case 3:
          if (tag != 26) {
            break
          }

          message.webViewIdRegex = reader.string()
          continue
      }
      if ((tag & 7) == 4 || tag == 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.encode(p).finish()]
        }
      } else {
        yield* [Config.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Config> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.decode(p)]
        }
      } else {
        yield* [Config.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Config {
    return {
      webPluginId: isSet(object.webPluginId) ? String(object.webPluginId) : '',
      handlePluginId: isSet(object.handlePluginId)
        ? String(object.handlePluginId)
        : '',
      webViewIdRegex: isSet(object.webViewIdRegex)
        ? String(object.webViewIdRegex)
        : '',
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.webPluginId !== undefined && (obj.webPluginId = message.webPluginId)
    message.handlePluginId !== undefined &&
      (obj.handlePluginId = message.handlePluginId)
    message.webViewIdRegex !== undefined &&
      (obj.webViewIdRegex = message.webViewIdRegex)
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.webPluginId = object.webPluginId ?? ''
    message.handlePluginId = object.handlePluginId ?? ''
    message.webViewIdRegex = object.webViewIdRegex ?? ''
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
