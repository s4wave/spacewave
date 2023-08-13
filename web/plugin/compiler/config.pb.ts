/* eslint-disable */
import { ControllerConfig } from '@go/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'bldr.web.plugin.compiler'

/** Config configures the web plugin builder. */
export interface Config {
  /**
   * ConfigSet is a ConfigSet to apply on plugin startup.
   * This ConfigSet is applied to the plugin bus.
   * This will be included in the plugin binary.
   */
  configSet: { [key: string]: ControllerConfig }
  /**
   * HostConfigSet is a ConfigSet to apply to the host on plugin startup.
   * This ConfigSet is applied to the plugin host bus.
   * This will be included in the plugin binary.
   * Adds a config to configSet with ID bldr/plugin/host/configset
   */
  hostConfigSet: { [key: string]: ControllerConfig }
  /**
   * DelveAddr is the address to listen for Delve remote connections.
   * If the build mode is dev and this is set, uses delve to run the plugin.
   * Ignored if build mode is not dev.
   * Special value: "wait" - waits for plugin entrypoint to be run manually.
   */
  delveAddr: string
}

export interface Config_ConfigSetEntry {
  key: string
  value: ControllerConfig | undefined
}

export interface Config_HostConfigSetEntry {
  key: string
  value: ControllerConfig | undefined
}

function createBaseConfig(): Config {
  return { configSet: {}, hostConfigSet: {}, delveAddr: '' }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    Object.entries(message.configSet).forEach(([key, value]) => {
      Config_ConfigSetEntry.encode(
        { key: key as any, value },
        writer.uint32(10).fork(),
      ).ldelim()
    })
    Object.entries(message.hostConfigSet).forEach(([key, value]) => {
      Config_HostConfigSetEntry.encode(
        { key: key as any, value },
        writer.uint32(18).fork(),
      ).ldelim()
    })
    if (message.delveAddr !== '') {
      writer.uint32(26).string(message.delveAddr)
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
          if (tag !== 10) {
            break
          }

          const entry1 = Config_ConfigSetEntry.decode(reader, reader.uint32())
          if (entry1.value !== undefined) {
            message.configSet[entry1.key] = entry1.value
          }
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          const entry2 = Config_HostConfigSetEntry.decode(
            reader,
            reader.uint32(),
          )
          if (entry2.value !== undefined) {
            message.hostConfigSet[entry2.key] = entry2.value
          }
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.delveAddr = reader.string()
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
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>,
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
      | Iterable<Uint8Array | Uint8Array[]>,
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
      configSet: isObject(object.configSet)
        ? Object.entries(object.configSet).reduce<{
            [key: string]: ControllerConfig
          }>((acc, [key, value]) => {
            acc[key] = ControllerConfig.fromJSON(value)
            return acc
          }, {})
        : {},
      hostConfigSet: isObject(object.hostConfigSet)
        ? Object.entries(object.hostConfigSet).reduce<{
            [key: string]: ControllerConfig
          }>((acc, [key, value]) => {
            acc[key] = ControllerConfig.fromJSON(value)
            return acc
          }, {})
        : {},
      delveAddr: isSet(object.delveAddr) ? String(object.delveAddr) : '',
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.configSet) {
      const entries = Object.entries(message.configSet)
      if (entries.length > 0) {
        obj.configSet = {}
        entries.forEach(([k, v]) => {
          obj.configSet[k] = ControllerConfig.toJSON(v)
        })
      }
    }
    if (message.hostConfigSet) {
      const entries = Object.entries(message.hostConfigSet)
      if (entries.length > 0) {
        obj.hostConfigSet = {}
        entries.forEach(([k, v]) => {
          obj.hostConfigSet[k] = ControllerConfig.toJSON(v)
        })
      }
    }
    if (message.delveAddr !== '') {
      obj.delveAddr = message.delveAddr
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.configSet = Object.entries(object.configSet ?? {}).reduce<{
      [key: string]: ControllerConfig
    }>((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = ControllerConfig.fromPartial(value)
      }
      return acc
    }, {})
    message.hostConfigSet = Object.entries(object.hostConfigSet ?? {}).reduce<{
      [key: string]: ControllerConfig
    }>((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = ControllerConfig.fromPartial(value)
      }
      return acc
    }, {})
    message.delveAddr = object.delveAddr ?? ''
    return message
  },
}

function createBaseConfig_ConfigSetEntry(): Config_ConfigSetEntry {
  return { key: '', value: undefined }
}

export const Config_ConfigSetEntry = {
  encode(
    message: Config_ConfigSetEntry,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.key !== '') {
      writer.uint32(10).string(message.key)
    }
    if (message.value !== undefined) {
      ControllerConfig.encode(message.value, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): Config_ConfigSetEntry {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseConfig_ConfigSetEntry()
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

          message.value = ControllerConfig.decode(reader, reader.uint32())
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
  // Transform<Config_ConfigSetEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<Config_ConfigSetEntry | Config_ConfigSetEntry[]>
      | Iterable<Config_ConfigSetEntry | Config_ConfigSetEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config_ConfigSetEntry.encode(p).finish()]
        }
      } else {
        yield* [Config_ConfigSetEntry.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config_ConfigSetEntry>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config_ConfigSetEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config_ConfigSetEntry.decode(p)]
        }
      } else {
        yield* [Config_ConfigSetEntry.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Config_ConfigSetEntry {
    return {
      key: isSet(object.key) ? String(object.key) : '',
      value: isSet(object.value)
        ? ControllerConfig.fromJSON(object.value)
        : undefined,
    }
  },

  toJSON(message: Config_ConfigSetEntry): unknown {
    const obj: any = {}
    if (message.key !== '') {
      obj.key = message.key
    }
    if (message.value !== undefined) {
      obj.value = ControllerConfig.toJSON(message.value)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config_ConfigSetEntry>, I>>(
    base?: I,
  ): Config_ConfigSetEntry {
    return Config_ConfigSetEntry.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config_ConfigSetEntry>, I>>(
    object: I,
  ): Config_ConfigSetEntry {
    const message = createBaseConfig_ConfigSetEntry()
    message.key = object.key ?? ''
    message.value =
      object.value !== undefined && object.value !== null
        ? ControllerConfig.fromPartial(object.value)
        : undefined
    return message
  },
}

function createBaseConfig_HostConfigSetEntry(): Config_HostConfigSetEntry {
  return { key: '', value: undefined }
}

export const Config_HostConfigSetEntry = {
  encode(
    message: Config_HostConfigSetEntry,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.key !== '') {
      writer.uint32(10).string(message.key)
    }
    if (message.value !== undefined) {
      ControllerConfig.encode(message.value, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number,
  ): Config_HostConfigSetEntry {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseConfig_HostConfigSetEntry()
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

          message.value = ControllerConfig.decode(reader, reader.uint32())
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
  // Transform<Config_HostConfigSetEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<Config_HostConfigSetEntry | Config_HostConfigSetEntry[]>
      | Iterable<Config_HostConfigSetEntry | Config_HostConfigSetEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config_HostConfigSetEntry.encode(p).finish()]
        }
      } else {
        yield* [Config_HostConfigSetEntry.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config_HostConfigSetEntry>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config_HostConfigSetEntry> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config_HostConfigSetEntry.decode(p)]
        }
      } else {
        yield* [Config_HostConfigSetEntry.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Config_HostConfigSetEntry {
    return {
      key: isSet(object.key) ? String(object.key) : '',
      value: isSet(object.value)
        ? ControllerConfig.fromJSON(object.value)
        : undefined,
    }
  },

  toJSON(message: Config_HostConfigSetEntry): unknown {
    const obj: any = {}
    if (message.key !== '') {
      obj.key = message.key
    }
    if (message.value !== undefined) {
      obj.value = ControllerConfig.toJSON(message.value)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config_HostConfigSetEntry>, I>>(
    base?: I,
  ): Config_HostConfigSetEntry {
    return Config_HostConfigSetEntry.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config_HostConfigSetEntry>, I>>(
    object: I,
  ): Config_HostConfigSetEntry {
    const message = createBaseConfig_HostConfigSetEntry()
    message.key = object.key ?? ''
    message.value =
      object.value !== undefined && object.value !== null
        ? ControllerConfig.fromPartial(object.value)
        : undefined
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

function isObject(value: any): boolean {
  return typeof value === 'object' && value !== null
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
