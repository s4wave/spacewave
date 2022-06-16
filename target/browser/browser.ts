/* eslint-disable */
import Long from 'long'
import * as _m0 from 'protobufjs/minimal'

export const protobufPackage = 'browser'

/** Config is the configuration for the browser controller. */
export interface Config {
  /**
   * RuntimeId is the unique ID of the runtime.
   *
   * must be set
   * used to determine the broadcast channel ids
   * determined by the webpage that started the worker
   */
  runtimeId: string
  /** WorkerUuid is the uuid for this specific Worker instance. */
  workerUuid: string
}

function createBaseConfig(): Config {
  return { runtimeId: '', workerUuid: '' }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.runtimeId !== '') {
      writer.uint32(10).string(message.runtimeId)
    }
    if (message.workerUuid !== '') {
      writer.uint32(18).string(message.workerUuid)
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
          message.runtimeId = reader.string()
          break
        case 2:
          message.workerUuid = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): Config {
    return {
      runtimeId: isSet(object.runtimeId) ? String(object.runtimeId) : '',
      workerUuid: isSet(object.workerUuid) ? String(object.workerUuid) : '',
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.runtimeId !== undefined && (obj.runtimeId = message.runtimeId)
    message.workerUuid !== undefined && (obj.workerUuid = message.workerUuid)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.runtimeId = object.runtimeId ?? ''
    message.workerUuid = object.workerUuid ?? ''
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
