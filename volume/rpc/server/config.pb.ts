/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'volume.rpc.server'

/**
 * Config configures the rpc volume server.
 * Provides the AccessVolumes RPC service.
 * Accesses volumes with LookupVolume and other directives.
 */
export interface Config {
  /**
   * ServiceId is the service id to listen on.
   * Usually a prefix with: rpc.volume.AccessVolumes
   * Cannot be empty.
   */
  serviceId: string
  /**
   * VolumeIdRe is a regex string to match volume IDs.
   * Set to '.*' to match all volumes.
   * ignored if empty.
   */
  volumeIdRe: string
  /**
   * VolumeIdList is a list of volume IDs to match.
   * If the value is in this list, overrides volume_id_re.
   * ignored if empty.
   */
  volumeIdList: string[]
  /**
   * ExposePrivateKey enables callers to fetch the private key for the volume.
   * Defaults to false.
   */
  exposePrivateKey: boolean
  /**
   * ReleaseDelay is a delay duration to wait before releasing a unreferenced volume.
   * If empty string, defaults to 1s (1 second).
   */
  releaseDelay: string
}

function createBaseConfig(): Config {
  return {
    serviceId: '',
    volumeIdRe: '',
    volumeIdList: [],
    exposePrivateKey: false,
    releaseDelay: '',
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.serviceId !== '') {
      writer.uint32(10).string(message.serviceId)
    }
    if (message.volumeIdRe !== '') {
      writer.uint32(18).string(message.volumeIdRe)
    }
    for (const v of message.volumeIdList) {
      writer.uint32(26).string(v!)
    }
    if (message.exposePrivateKey === true) {
      writer.uint32(32).bool(message.exposePrivateKey)
    }
    if (message.releaseDelay !== '') {
      writer.uint32(42).string(message.releaseDelay)
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

          message.serviceId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.volumeIdRe = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.volumeIdList.push(reader.string())
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.exposePrivateKey = reader.bool()
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.releaseDelay = reader.string()
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
      serviceId: isSet(object.serviceId) ? String(object.serviceId) : '',
      volumeIdRe: isSet(object.volumeIdRe) ? String(object.volumeIdRe) : '',
      volumeIdList: Array.isArray(object?.volumeIdList)
        ? object.volumeIdList.map((e: any) => String(e))
        : [],
      exposePrivateKey: isSet(object.exposePrivateKey)
        ? Boolean(object.exposePrivateKey)
        : false,
      releaseDelay: isSet(object.releaseDelay)
        ? String(object.releaseDelay)
        : '',
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.serviceId !== undefined && (obj.serviceId = message.serviceId)
    message.volumeIdRe !== undefined && (obj.volumeIdRe = message.volumeIdRe)
    if (message.volumeIdList) {
      obj.volumeIdList = message.volumeIdList.map((e) => e)
    } else {
      obj.volumeIdList = []
    }
    message.exposePrivateKey !== undefined &&
      (obj.exposePrivateKey = message.exposePrivateKey)
    message.releaseDelay !== undefined &&
      (obj.releaseDelay = message.releaseDelay)
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.serviceId = object.serviceId ?? ''
    message.volumeIdRe = object.volumeIdRe ?? ''
    message.volumeIdList = object.volumeIdList?.map((e) => e) || []
    message.exposePrivateKey = object.exposePrivateKey ?? false
    message.releaseDelay = object.releaseDelay ?? ''
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
