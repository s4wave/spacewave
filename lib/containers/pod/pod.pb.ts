/* eslint-disable */
import Long from 'long'
import { Pod } from '@go/github.com/aperturerobotics/containers/pod/pod.pb.js'
import * as _m0 from 'protobufjs/minimal'

export const protobufPackage = 'forge.lib.containers.pod'

/** Config configures the containers pod exec controller. */
export interface Config {
  /** EngineId is the pod engine to run the pod on. */
  engineId: string
  /**
   * Name is the name of the pod.
   * Must be set if generate_name is not set.
   * Overrides meta field.
   */
  name: string
  /**
   * GenerateName is the base name for generating a unique pod name.
   * Must be set if name is not set.
   * Overrides meta field.
   */
  generateName: string
  /**
   * Meta contains a json or YAML k8s ObjectMeta object.
   * Optional if name or generate_name is set.
   */
  meta: string
  /** Pod is the pod configuration. */
  pod: Pod | undefined
  /** Quiet suppresses stdin/stdout logs to os stdio. */
  quiet: boolean
}

function createBaseConfig(): Config {
  return {
    engineId: '',
    name: '',
    generateName: '',
    meta: '',
    pod: undefined,
    quiet: false,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.engineId !== '') {
      writer.uint32(10).string(message.engineId)
    }
    if (message.name !== '') {
      writer.uint32(18).string(message.name)
    }
    if (message.generateName !== '') {
      writer.uint32(26).string(message.generateName)
    }
    if (message.meta !== '') {
      writer.uint32(34).string(message.meta)
    }
    if (message.pod !== undefined) {
      Pod.encode(message.pod, writer.uint32(42).fork()).ldelim()
    }
    if (message.quiet === true) {
      writer.uint32(48).bool(message.quiet)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseConfig()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.engineId = reader.string()
          break
        case 2:
          message.name = reader.string()
          break
        case 3:
          message.generateName = reader.string()
          break
        case 4:
          message.meta = reader.string()
          break
        case 5:
          message.pod = Pod.decode(reader, reader.uint32())
          break
        case 6:
          message.quiet = reader.bool()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
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
      engineId: isSet(object.engineId) ? String(object.engineId) : '',
      name: isSet(object.name) ? String(object.name) : '',
      generateName: isSet(object.generateName)
        ? String(object.generateName)
        : '',
      meta: isSet(object.meta) ? String(object.meta) : '',
      pod: isSet(object.pod) ? Pod.fromJSON(object.pod) : undefined,
      quiet: isSet(object.quiet) ? Boolean(object.quiet) : false,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.engineId !== undefined && (obj.engineId = message.engineId)
    message.name !== undefined && (obj.name = message.name)
    message.generateName !== undefined &&
      (obj.generateName = message.generateName)
    message.meta !== undefined && (obj.meta = message.meta)
    message.pod !== undefined &&
      (obj.pod = message.pod ? Pod.toJSON(message.pod) : undefined)
    message.quiet !== undefined && (obj.quiet = message.quiet)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.engineId = object.engineId ?? ''
    message.name = object.name ?? ''
    message.generateName = object.generateName ?? ''
    message.meta = object.meta ?? ''
    message.pod =
      object.pod !== undefined && object.pod !== null
        ? Pod.fromPartial(object.pod)
        : undefined
    message.quiet = object.quiet ?? false
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
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & Record<
        Exclude<keyof I, KeysOfUnion<P>>,
        never
      >

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any
  _m0.configure()
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
