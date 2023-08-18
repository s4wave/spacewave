/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'auth.method.triplesec'

/** Parameters are stored with the user record. */
export interface Parameters {
  /**
   * Salt is the salt used for the keypair.
   * Should be 16 bytes.
   */
  salt: Uint8Array
  /**
   * Version is the triplesec version to use.
   * If zero, latest is used.
   */
  version: number
}

/** Config is configuration for the auth method. */
export interface Config {}

function createBaseParameters(): Parameters {
  return { salt: new Uint8Array(0), version: 0 }
}

export const Parameters = {
  encode(
    message: Parameters,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.salt.length !== 0) {
      writer.uint32(10).bytes(message.salt)
    }
    if (message.version !== 0) {
      writer.uint32(16).uint32(message.version)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Parameters {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseParameters()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.salt = reader.bytes()
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.version = reader.uint32()
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
  // Transform<Parameters, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<Parameters | Parameters[]>
      | Iterable<Parameters | Parameters[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Parameters.encode(p).finish()]
        }
      } else {
        yield* [Parameters.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Parameters>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Parameters> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Parameters.decode(p)]
        }
      } else {
        yield* [Parameters.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Parameters {
    return {
      salt: isSet(object.salt)
        ? bytesFromBase64(object.salt)
        : new Uint8Array(0),
      version: isSet(object.version) ? Number(object.version) : 0,
    }
  },

  toJSON(message: Parameters): unknown {
    const obj: any = {}
    if (message.salt.length !== 0) {
      obj.salt = base64FromBytes(message.salt)
    }
    if (message.version !== 0) {
      obj.version = Math.round(message.version)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Parameters>, I>>(base?: I): Parameters {
    return Parameters.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Parameters>, I>>(
    object: I,
  ): Parameters {
    const message = createBaseParameters()
    message.salt = object.salt ?? new Uint8Array(0)
    message.version = object.version ?? 0
    return message
  },
}

function createBaseConfig(): Config {
  return {}
}

export const Config = {
  encode(_: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
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

  fromJSON(_: any): Config {
    return {}
  },

  toJSON(_: Config): unknown {
    const obj: any = {}
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Config>, I>>(_: I): Config {
    const message = createBaseConfig()
    return message
  },
}

declare const self: any | undefined
declare const window: any | undefined
declare const global: any | undefined
const tsProtoGlobalThis: any = (() => {
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

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
