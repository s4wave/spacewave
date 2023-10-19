/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'cluster.tx'

/** TxType indicates the kind of transaction. */
export enum TxType {
  TxType_INVALID = 0,
  UNRECOGNIZED = -1,
}

export function txTypeFromJSON(object: any): TxType {
  switch (object) {
    case 0:
    case 'TxType_INVALID':
      return TxType.TxType_INVALID
    case -1:
    case 'UNRECOGNIZED':
    default:
      return TxType.UNRECOGNIZED
  }
}

export function txTypeToJSON(object: TxType): string {
  switch (object) {
    case TxType.TxType_INVALID:
      return 'TxType_INVALID'
    case TxType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** Tx is the on-the-wire representation of a transaction. */
export interface Tx {
  /** TxType is the kind of transaction this is. */
  txType: TxType
  /**
   * ClusterObjectKey is the Cluster object ID this is associated with.
   * The Cluster object must already exist.
   */
  clusterObjectKey: string
}

function createBaseTx(): Tx {
  return { txType: 0, clusterObjectKey: '' }
}

export const Tx = {
  encode(message: Tx, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.txType !== 0) {
      writer.uint32(8).int32(message.txType)
    }
    if (message.clusterObjectKey !== '') {
      writer.uint32(18).string(message.clusterObjectKey)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Tx {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTx()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 8) {
            break
          }

          message.txType = reader.int32() as any
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.clusterObjectKey = reader.string()
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
  // Transform<Tx, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Tx | Tx[]> | Iterable<Tx | Tx[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Tx.encode(p).finish()]
        }
      } else {
        yield* [Tx.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Tx>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Tx> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Tx.decode(p)]
        }
      } else {
        yield* [Tx.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Tx {
    return {
      txType: isSet(object.txType) ? txTypeFromJSON(object.txType) : 0,
      clusterObjectKey: isSet(object.clusterObjectKey)
        ? globalThis.String(object.clusterObjectKey)
        : '',
    }
  },

  toJSON(message: Tx): unknown {
    const obj: any = {}
    if (message.txType !== 0) {
      obj.txType = txTypeToJSON(message.txType)
    }
    if (message.clusterObjectKey !== '') {
      obj.clusterObjectKey = message.clusterObjectKey
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Tx>, I>>(base?: I): Tx {
    return Tx.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<Tx>, I>>(object: I): Tx {
    const message = createBaseTx()
    message.txType = object.txType ?? 0
    message.clusterObjectKey = object.clusterObjectKey ?? ''
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
