/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'unixfs.mount.fuse'

/**
 * Config configures the FUSE mount controller.
 * The FUSE mount controller mounts directly with immediate writeback mode.
 */
export interface Config {
  /** MountPath is the destination mount path. */
  mountPath: string
  /**
   * Verbose enables verbose logging.
   * Volume attribute: verbose=true
   */
  verbose: boolean
  /**
   * AllowOther enables other users than the mounter to access the mount.
   * Volume attribute: allow_other=true
   */
  allowOther: boolean
  /** AllowDev enables device objects to exist on the FS. */
  allowDev: boolean
  /** AllowSuid allows set-user-identifier or set-group-identifier bits to take effect. */
  allowSuid: boolean
}

function createBaseConfig(): Config {
  return {
    mountPath: '',
    verbose: false,
    allowOther: false,
    allowDev: false,
    allowSuid: false,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.mountPath !== '') {
      writer.uint32(10).string(message.mountPath)
    }
    if (message.verbose === true) {
      writer.uint32(16).bool(message.verbose)
    }
    if (message.allowOther === true) {
      writer.uint32(24).bool(message.allowOther)
    }
    if (message.allowDev === true) {
      writer.uint32(32).bool(message.allowDev)
    }
    if (message.allowSuid === true) {
      writer.uint32(40).bool(message.allowSuid)
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

          message.mountPath = reader.string()
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.verbose = reader.bool()
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.allowOther = reader.bool()
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.allowDev = reader.bool()
          continue
        case 5:
          if (tag !== 40) {
            break
          }

          message.allowSuid = reader.bool()
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
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Config.encode(p).finish()]
        }
      } else {
        yield* [Config.encode(pkt as any).finish()]
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
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Config.decode(p)]
        }
      } else {
        yield* [Config.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Config {
    return {
      mountPath: isSet(object.mountPath)
        ? globalThis.String(object.mountPath)
        : '',
      verbose: isSet(object.verbose)
        ? globalThis.Boolean(object.verbose)
        : false,
      allowOther: isSet(object.allowOther)
        ? globalThis.Boolean(object.allowOther)
        : false,
      allowDev: isSet(object.allowDev)
        ? globalThis.Boolean(object.allowDev)
        : false,
      allowSuid: isSet(object.allowSuid)
        ? globalThis.Boolean(object.allowSuid)
        : false,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    if (message.mountPath !== '') {
      obj.mountPath = message.mountPath
    }
    if (message.verbose === true) {
      obj.verbose = message.verbose
    }
    if (message.allowOther === true) {
      obj.allowOther = message.allowOther
    }
    if (message.allowDev === true) {
      obj.allowDev = message.allowDev
    }
    if (message.allowSuid === true) {
      obj.allowSuid = message.allowSuid
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.mountPath = object.mountPath ?? ''
    message.verbose = object.verbose ?? false
    message.allowOther = object.allowOther ?? false
    message.allowDev = object.allowDev ?? false
    message.allowSuid = object.allowSuid ?? false
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
