/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Config as Config1 } from '../bucket.pb.js'

export const protobufPackage = 'bucket.setup'

/** Config is the setup configuration. */
export interface Config {
  /**
   * ApplyBucketConfigs is a list of bucket configurations to apply.
   * Note: does not apply if existing config has a higher revision number.
   * Optional.
   */
  applyBucketConfigs: ApplyBucketConfig[]
}

/** ApplyBucketConfig is the configuration for applying a bucket config. */
export interface ApplyBucketConfig {
  /** Config is the bucket config. */
  config: Config1 | undefined
  /**
   * VolumeIdRe is a regex string to match volume IDs.
   * Set to '.*' to match all volumes.
   * If empty, will update volumes that already have the config only.
   * If VolumeIDList is set, it will override this field.
   * Cannot be specified if VolumeIDList is set.
   */
  volumeIdRe: string
  /**
   * VolumeIdList is a list of volume IDs to match.
   * Cannot be specified if VolumeIDRe is set.
   */
  volumeIdList: string[]
}

function createBaseConfig(): Config {
  return { applyBucketConfigs: [] }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    for (const v of message.applyBucketConfigs) {
      ApplyBucketConfig.encode(v!, writer.uint32(10).fork()).ldelim()
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

          message.applyBucketConfigs.push(
            ApplyBucketConfig.decode(reader, reader.uint32())
          )
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
      applyBucketConfigs: Array.isArray(object?.applyBucketConfigs)
        ? object.applyBucketConfigs.map((e: any) =>
            ApplyBucketConfig.fromJSON(e)
          )
        : [],
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.applyBucketConfigs) {
      obj.applyBucketConfigs = message.applyBucketConfigs.map((e) =>
        e ? ApplyBucketConfig.toJSON(e) : undefined
      )
    } else {
      obj.applyBucketConfigs = []
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.applyBucketConfigs =
      object.applyBucketConfigs?.map((e) => ApplyBucketConfig.fromPartial(e)) ||
      []
    return message
  },
}

function createBaseApplyBucketConfig(): ApplyBucketConfig {
  return { config: undefined, volumeIdRe: '', volumeIdList: [] }
}

export const ApplyBucketConfig = {
  encode(
    message: ApplyBucketConfig,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.config !== undefined) {
      Config1.encode(message.config, writer.uint32(10).fork()).ldelim()
    }
    if (message.volumeIdRe !== '') {
      writer.uint32(18).string(message.volumeIdRe)
    }
    for (const v of message.volumeIdList) {
      writer.uint32(26).string(v!)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ApplyBucketConfig {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseApplyBucketConfig()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break
          }

          message.config = Config1.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.volumeIdRe = reader.string()
          continue
        case 3:
          if (tag != 26) {
            break
          }

          message.volumeIdList.push(reader.string())
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
  // Transform<ApplyBucketConfig, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ApplyBucketConfig | ApplyBucketConfig[]>
      | Iterable<ApplyBucketConfig | ApplyBucketConfig[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ApplyBucketConfig.encode(p).finish()]
        }
      } else {
        yield* [ApplyBucketConfig.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ApplyBucketConfig>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ApplyBucketConfig> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ApplyBucketConfig.decode(p)]
        }
      } else {
        yield* [ApplyBucketConfig.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ApplyBucketConfig {
    return {
      config: isSet(object.config)
        ? Config1.fromJSON(object.config)
        : undefined,
      volumeIdRe: isSet(object.volumeIdRe) ? String(object.volumeIdRe) : '',
      volumeIdList: Array.isArray(object?.volumeIdList)
        ? object.volumeIdList.map((e: any) => String(e))
        : [],
    }
  },

  toJSON(message: ApplyBucketConfig): unknown {
    const obj: any = {}
    message.config !== undefined &&
      (obj.config = message.config ? Config1.toJSON(message.config) : undefined)
    message.volumeIdRe !== undefined && (obj.volumeIdRe = message.volumeIdRe)
    if (message.volumeIdList) {
      obj.volumeIdList = message.volumeIdList.map((e) => e)
    } else {
      obj.volumeIdList = []
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ApplyBucketConfig>, I>>(
    base?: I
  ): ApplyBucketConfig {
    return ApplyBucketConfig.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ApplyBucketConfig>, I>>(
    object: I
  ): ApplyBucketConfig {
    const message = createBaseApplyBucketConfig()
    message.config =
      object.config !== undefined && object.config !== null
        ? Config1.fromPartial(object.config)
        : undefined
    message.volumeIdRe = object.volumeIdRe ?? ''
    message.volumeIdList = object.volumeIdList?.map((e) => e) || []
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
