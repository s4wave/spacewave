/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Quad } from '../../../block/quad/quad.pb.js'
import { ObjectRef } from '../../../bucket/bucket.pb.js'

export const protobufPackage = 'world.block.tx'

/** TxType indicates the kind of transaction. */
export enum TxType {
  TxType_INVALID = 0,
  /** TxType_APPLY_WORLD_OP - TxType_APPLY_WORLD_OP applies a world operation. */
  TxType_APPLY_WORLD_OP = 1,
  /** TxType_APPLY_OBJECT_OP - TxType_APPLY_OBJECT_OP applies a object operation. */
  TxType_APPLY_OBJECT_OP = 2,
  /** TxType_CREATE_OBJECT - TxType_CREATE_OBJECT creates a new object with a key and root ref. */
  TxType_CREATE_OBJECT = 3,
  /** TxType_OBJECT_SET - TxType_OBJECT_SET sets the root ref on an object. */
  TxType_OBJECT_SET = 4,
  /** TxType_OBJECT_INC_REV - TxType_OBJECT_INC_REV increments the rev on a object. */
  TxType_OBJECT_INC_REV = 5,
  /** TxType_DELETE_OBJECT - TxType_DELETE_OBJECT deletes a object with a key. */
  TxType_DELETE_OBJECT = 6,
  /** TxType_SET_GRAPH_QUAD - TxType_SET_GRAPH_QUAD sets a graph quad in the graph store. */
  TxType_SET_GRAPH_QUAD = 7,
  /** TxType_DELETE_GRAPH_QUAD - TxType_DELETE_GRAPH_QUAD deletes a graph quad from the store. */
  TxType_DELETE_GRAPH_QUAD = 8,
  /** TxType_BATCH - TxType_BATCH applies multiple sub-transactions. */
  TxType_BATCH = 9,
  UNRECOGNIZED = -1,
}

export function txTypeFromJSON(object: any): TxType {
  switch (object) {
    case 0:
    case 'TxType_INVALID':
      return TxType.TxType_INVALID
    case 1:
    case 'TxType_APPLY_WORLD_OP':
      return TxType.TxType_APPLY_WORLD_OP
    case 2:
    case 'TxType_APPLY_OBJECT_OP':
      return TxType.TxType_APPLY_OBJECT_OP
    case 3:
    case 'TxType_CREATE_OBJECT':
      return TxType.TxType_CREATE_OBJECT
    case 4:
    case 'TxType_OBJECT_SET':
      return TxType.TxType_OBJECT_SET
    case 5:
    case 'TxType_OBJECT_INC_REV':
      return TxType.TxType_OBJECT_INC_REV
    case 6:
    case 'TxType_DELETE_OBJECT':
      return TxType.TxType_DELETE_OBJECT
    case 7:
    case 'TxType_SET_GRAPH_QUAD':
      return TxType.TxType_SET_GRAPH_QUAD
    case 8:
    case 'TxType_DELETE_GRAPH_QUAD':
      return TxType.TxType_DELETE_GRAPH_QUAD
    case 9:
    case 'TxType_BATCH':
      return TxType.TxType_BATCH
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
    case TxType.TxType_APPLY_WORLD_OP:
      return 'TxType_APPLY_WORLD_OP'
    case TxType.TxType_APPLY_OBJECT_OP:
      return 'TxType_APPLY_OBJECT_OP'
    case TxType.TxType_CREATE_OBJECT:
      return 'TxType_CREATE_OBJECT'
    case TxType.TxType_OBJECT_SET:
      return 'TxType_OBJECT_SET'
    case TxType.TxType_OBJECT_INC_REV:
      return 'TxType_OBJECT_INC_REV'
    case TxType.TxType_DELETE_OBJECT:
      return 'TxType_DELETE_OBJECT'
    case TxType.TxType_SET_GRAPH_QUAD:
      return 'TxType_SET_GRAPH_QUAD'
    case TxType.TxType_DELETE_GRAPH_QUAD:
      return 'TxType_DELETE_GRAPH_QUAD'
    case TxType.TxType_BATCH:
      return 'TxType_BATCH'
    case TxType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** Tx is the on-the-wire representation of a World transaction. */
export interface Tx {
  /** TxType is the kind of transaction this is. */
  txType: TxType
  /**
   * TxApplyWorldOp is an operation to apply a world operation
   * TxType_APPLY_WORLD_OP
   */
  txApplyWorldOp: TxApplyWorldOp | undefined
  /**
   * TxApplyObjectOp is an operation to apply a object operation
   * TxType_APPLY_OBJECT_OP
   */
  txApplyObjectOp: TxApplyObjectOp | undefined
  /**
   * TxCreateObject is an operation to create a new object.
   * TxType_CREATE_OBJECT
   */
  txCreateObject: TxCreateObject | undefined
  /**
   * TxObjectSet sets the root ref of an object.
   * TxType_OBJECT_SET
   */
  txObjectSet: TxObjectSet | undefined
  /**
   * TxObjectIncRev increments the revision of an object.
   * TxType_OBJECT_INC_REV
   */
  txObjectIncRev: TxObjectIncRev | undefined
  /**
   * TxDeleteObject to delete a object.
   * TxType_DELETE_OBJECT
   */
  txDeleteObject: TxDeleteObject | undefined
  /** TxSetGraphQuad sets a graph quad. */
  txSetGraphQuad: TxSetGraphQuad | undefined
  /** TxDeleteGraphQuad deletes a graph quad. */
  txDeleteGraphQuad: TxDeleteGraphQuad | undefined
  /** TxBatch is a batch of multiple txs. */
  txBatch: TxBatch | undefined
}

/** TxBatch is a batch of multiple transactions. */
export interface TxBatch {
  /** Txs is the list of transactions. */
  txs: Tx[]
}

/**
 * TxApplyWorldOp applies a world operation.
 * TxType: TxType_APPLY_WORLD_OP
 */
export interface TxApplyWorldOp {
  /** OperationTypeId is the operation type identifier. */
  operationTypeId: string
  /** OperationBody is the encoded operation Block. */
  operationBody: Uint8Array
}

/**
 * TxApplyObjectOp applies a object operation.
 * TxType: TxType_APPLY_OBJECT_OP
 */
export interface TxApplyObjectOp {
  /** OperationTypeId is the operation type identifier. */
  operationTypeId: string
  /** OperationBody is the encoded operation Block. */
  operationBody: Uint8Array
  /** ObjectKey is the object key to apply the operation to. */
  objectKey: string
}

/**
 * TxCreateObject creates a new object with a key and ref.
 * TxType: TxType_CREATE_OBJECT
 */
export interface TxCreateObject {
  /** ObjectKey is the object key to apply the operation to. */
  objectKey: string
  /** RootRef is the bucket object ref to set as the value. */
  rootRef: ObjectRef | undefined
}

/**
 * TxObjectSet sets the root ref of an existing object.
 * TxType: TxType_OBJECT_SET
 */
export interface TxObjectSet {
  /** ObjectKey is the object key to apply the operation to. */
  objectKey: string
  /** RootRef is the bucket object ref to set as the value. */
  rootRef: ObjectRef | undefined
}

/**
 * TxObjectIncRev increments the revision of a object.
 * TxType: TxType_OBJECT_INC_REV
 */
export interface TxObjectIncRev {
  /** ObjectKey is the object key to apply the operation to. */
  objectKey: string
}

/**
 * TxDeleteObject deletes an object with a given key.
 * TxType: TxType_DELETE_OBJECT
 */
export interface TxDeleteObject {
  /** ObjectKey is the object key to delete. */
  objectKey: string
  /** FailIfNotFound indicates to error if not found. */
  failIfNotFound: boolean
}

/**
 * TxSetGraphQuad sets a graph quad.
 * TxType: TxType_SET_GRAPH_QUAD
 */
export interface TxSetGraphQuad {
  /** Quad is the graph quad to create. */
  quad: Quad | undefined
}

/**
 * TxDeleteGraphQuad deletes a graph quad.
 * TxType: TxType_DELETE_GRAPH_QUAD
 */
export interface TxDeleteGraphQuad {
  /** Quad is the graph quad to delete. */
  quad: Quad | undefined
}

function createBaseTx(): Tx {
  return {
    txType: 0,
    txApplyWorldOp: undefined,
    txApplyObjectOp: undefined,
    txCreateObject: undefined,
    txObjectSet: undefined,
    txObjectIncRev: undefined,
    txDeleteObject: undefined,
    txSetGraphQuad: undefined,
    txDeleteGraphQuad: undefined,
    txBatch: undefined,
  }
}

export const Tx = {
  encode(message: Tx, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.txType !== 0) {
      writer.uint32(8).int32(message.txType)
    }
    if (message.txApplyWorldOp !== undefined) {
      TxApplyWorldOp.encode(
        message.txApplyWorldOp,
        writer.uint32(18).fork()
      ).ldelim()
    }
    if (message.txApplyObjectOp !== undefined) {
      TxApplyObjectOp.encode(
        message.txApplyObjectOp,
        writer.uint32(26).fork()
      ).ldelim()
    }
    if (message.txCreateObject !== undefined) {
      TxCreateObject.encode(
        message.txCreateObject,
        writer.uint32(34).fork()
      ).ldelim()
    }
    if (message.txObjectSet !== undefined) {
      TxObjectSet.encode(message.txObjectSet, writer.uint32(42).fork()).ldelim()
    }
    if (message.txObjectIncRev !== undefined) {
      TxObjectIncRev.encode(
        message.txObjectIncRev,
        writer.uint32(50).fork()
      ).ldelim()
    }
    if (message.txDeleteObject !== undefined) {
      TxDeleteObject.encode(
        message.txDeleteObject,
        writer.uint32(58).fork()
      ).ldelim()
    }
    if (message.txSetGraphQuad !== undefined) {
      TxSetGraphQuad.encode(
        message.txSetGraphQuad,
        writer.uint32(66).fork()
      ).ldelim()
    }
    if (message.txDeleteGraphQuad !== undefined) {
      TxDeleteGraphQuad.encode(
        message.txDeleteGraphQuad,
        writer.uint32(74).fork()
      ).ldelim()
    }
    if (message.txBatch !== undefined) {
      TxBatch.encode(message.txBatch, writer.uint32(82).fork()).ldelim()
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

          message.txApplyWorldOp = TxApplyWorldOp.decode(
            reader,
            reader.uint32()
          )
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.txApplyObjectOp = TxApplyObjectOp.decode(
            reader,
            reader.uint32()
          )
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.txCreateObject = TxCreateObject.decode(
            reader,
            reader.uint32()
          )
          continue
        case 5:
          if (tag !== 42) {
            break
          }

          message.txObjectSet = TxObjectSet.decode(reader, reader.uint32())
          continue
        case 6:
          if (tag !== 50) {
            break
          }

          message.txObjectIncRev = TxObjectIncRev.decode(
            reader,
            reader.uint32()
          )
          continue
        case 7:
          if (tag !== 58) {
            break
          }

          message.txDeleteObject = TxDeleteObject.decode(
            reader,
            reader.uint32()
          )
          continue
        case 8:
          if (tag !== 66) {
            break
          }

          message.txSetGraphQuad = TxSetGraphQuad.decode(
            reader,
            reader.uint32()
          )
          continue
        case 9:
          if (tag !== 74) {
            break
          }

          message.txDeleteGraphQuad = TxDeleteGraphQuad.decode(
            reader,
            reader.uint32()
          )
          continue
        case 10:
          if (tag !== 82) {
            break
          }

          message.txBatch = TxBatch.decode(reader, reader.uint32())
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
    source: AsyncIterable<Tx | Tx[]> | Iterable<Tx | Tx[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Tx.encode(p).finish()]
        }
      } else {
        yield* [Tx.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Tx>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Tx> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Tx.decode(p)]
        }
      } else {
        yield* [Tx.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Tx {
    return {
      txType: isSet(object.txType) ? txTypeFromJSON(object.txType) : 0,
      txApplyWorldOp: isSet(object.txApplyWorldOp)
        ? TxApplyWorldOp.fromJSON(object.txApplyWorldOp)
        : undefined,
      txApplyObjectOp: isSet(object.txApplyObjectOp)
        ? TxApplyObjectOp.fromJSON(object.txApplyObjectOp)
        : undefined,
      txCreateObject: isSet(object.txCreateObject)
        ? TxCreateObject.fromJSON(object.txCreateObject)
        : undefined,
      txObjectSet: isSet(object.txObjectSet)
        ? TxObjectSet.fromJSON(object.txObjectSet)
        : undefined,
      txObjectIncRev: isSet(object.txObjectIncRev)
        ? TxObjectIncRev.fromJSON(object.txObjectIncRev)
        : undefined,
      txDeleteObject: isSet(object.txDeleteObject)
        ? TxDeleteObject.fromJSON(object.txDeleteObject)
        : undefined,
      txSetGraphQuad: isSet(object.txSetGraphQuad)
        ? TxSetGraphQuad.fromJSON(object.txSetGraphQuad)
        : undefined,
      txDeleteGraphQuad: isSet(object.txDeleteGraphQuad)
        ? TxDeleteGraphQuad.fromJSON(object.txDeleteGraphQuad)
        : undefined,
      txBatch: isSet(object.txBatch)
        ? TxBatch.fromJSON(object.txBatch)
        : undefined,
    }
  },

  toJSON(message: Tx): unknown {
    const obj: any = {}
    message.txType !== undefined && (obj.txType = txTypeToJSON(message.txType))
    message.txApplyWorldOp !== undefined &&
      (obj.txApplyWorldOp = message.txApplyWorldOp
        ? TxApplyWorldOp.toJSON(message.txApplyWorldOp)
        : undefined)
    message.txApplyObjectOp !== undefined &&
      (obj.txApplyObjectOp = message.txApplyObjectOp
        ? TxApplyObjectOp.toJSON(message.txApplyObjectOp)
        : undefined)
    message.txCreateObject !== undefined &&
      (obj.txCreateObject = message.txCreateObject
        ? TxCreateObject.toJSON(message.txCreateObject)
        : undefined)
    message.txObjectSet !== undefined &&
      (obj.txObjectSet = message.txObjectSet
        ? TxObjectSet.toJSON(message.txObjectSet)
        : undefined)
    message.txObjectIncRev !== undefined &&
      (obj.txObjectIncRev = message.txObjectIncRev
        ? TxObjectIncRev.toJSON(message.txObjectIncRev)
        : undefined)
    message.txDeleteObject !== undefined &&
      (obj.txDeleteObject = message.txDeleteObject
        ? TxDeleteObject.toJSON(message.txDeleteObject)
        : undefined)
    message.txSetGraphQuad !== undefined &&
      (obj.txSetGraphQuad = message.txSetGraphQuad
        ? TxSetGraphQuad.toJSON(message.txSetGraphQuad)
        : undefined)
    message.txDeleteGraphQuad !== undefined &&
      (obj.txDeleteGraphQuad = message.txDeleteGraphQuad
        ? TxDeleteGraphQuad.toJSON(message.txDeleteGraphQuad)
        : undefined)
    message.txBatch !== undefined &&
      (obj.txBatch = message.txBatch
        ? TxBatch.toJSON(message.txBatch)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<Tx>, I>>(base?: I): Tx {
    return Tx.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Tx>, I>>(object: I): Tx {
    const message = createBaseTx()
    message.txType = object.txType ?? 0
    message.txApplyWorldOp =
      object.txApplyWorldOp !== undefined && object.txApplyWorldOp !== null
        ? TxApplyWorldOp.fromPartial(object.txApplyWorldOp)
        : undefined
    message.txApplyObjectOp =
      object.txApplyObjectOp !== undefined && object.txApplyObjectOp !== null
        ? TxApplyObjectOp.fromPartial(object.txApplyObjectOp)
        : undefined
    message.txCreateObject =
      object.txCreateObject !== undefined && object.txCreateObject !== null
        ? TxCreateObject.fromPartial(object.txCreateObject)
        : undefined
    message.txObjectSet =
      object.txObjectSet !== undefined && object.txObjectSet !== null
        ? TxObjectSet.fromPartial(object.txObjectSet)
        : undefined
    message.txObjectIncRev =
      object.txObjectIncRev !== undefined && object.txObjectIncRev !== null
        ? TxObjectIncRev.fromPartial(object.txObjectIncRev)
        : undefined
    message.txDeleteObject =
      object.txDeleteObject !== undefined && object.txDeleteObject !== null
        ? TxDeleteObject.fromPartial(object.txDeleteObject)
        : undefined
    message.txSetGraphQuad =
      object.txSetGraphQuad !== undefined && object.txSetGraphQuad !== null
        ? TxSetGraphQuad.fromPartial(object.txSetGraphQuad)
        : undefined
    message.txDeleteGraphQuad =
      object.txDeleteGraphQuad !== undefined &&
      object.txDeleteGraphQuad !== null
        ? TxDeleteGraphQuad.fromPartial(object.txDeleteGraphQuad)
        : undefined
    message.txBatch =
      object.txBatch !== undefined && object.txBatch !== null
        ? TxBatch.fromPartial(object.txBatch)
        : undefined
    return message
  },
}

function createBaseTxBatch(): TxBatch {
  return { txs: [] }
}

export const TxBatch = {
  encode(
    message: TxBatch,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    for (const v of message.txs) {
      Tx.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxBatch {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTxBatch()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.txs.push(Tx.decode(reader, reader.uint32()))
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
  // Transform<TxBatch, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<TxBatch | TxBatch[]> | Iterable<TxBatch | TxBatch[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxBatch.encode(p).finish()]
        }
      } else {
        yield* [TxBatch.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxBatch>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TxBatch> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxBatch.decode(p)]
        }
      } else {
        yield* [TxBatch.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TxBatch {
    return {
      txs: Array.isArray(object?.txs)
        ? object.txs.map((e: any) => Tx.fromJSON(e))
        : [],
    }
  },

  toJSON(message: TxBatch): unknown {
    const obj: any = {}
    if (message.txs) {
      obj.txs = message.txs.map((e) => (e ? Tx.toJSON(e) : undefined))
    } else {
      obj.txs = []
    }
    return obj
  },

  create<I extends Exact<DeepPartial<TxBatch>, I>>(base?: I): TxBatch {
    return TxBatch.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<TxBatch>, I>>(object: I): TxBatch {
    const message = createBaseTxBatch()
    message.txs = object.txs?.map((e) => Tx.fromPartial(e)) || []
    return message
  },
}

function createBaseTxApplyWorldOp(): TxApplyWorldOp {
  return { operationTypeId: '', operationBody: new Uint8Array(0) }
}

export const TxApplyWorldOp = {
  encode(
    message: TxApplyWorldOp,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.operationTypeId !== '') {
      writer.uint32(10).string(message.operationTypeId)
    }
    if (message.operationBody.length !== 0) {
      writer.uint32(18).bytes(message.operationBody)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxApplyWorldOp {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTxApplyWorldOp()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.operationTypeId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.operationBody = reader.bytes()
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
  // Transform<TxApplyWorldOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TxApplyWorldOp | TxApplyWorldOp[]>
      | Iterable<TxApplyWorldOp | TxApplyWorldOp[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxApplyWorldOp.encode(p).finish()]
        }
      } else {
        yield* [TxApplyWorldOp.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxApplyWorldOp>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TxApplyWorldOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxApplyWorldOp.decode(p)]
        }
      } else {
        yield* [TxApplyWorldOp.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TxApplyWorldOp {
    return {
      operationTypeId: isSet(object.operationTypeId)
        ? String(object.operationTypeId)
        : '',
      operationBody: isSet(object.operationBody)
        ? bytesFromBase64(object.operationBody)
        : new Uint8Array(0),
    }
  },

  toJSON(message: TxApplyWorldOp): unknown {
    const obj: any = {}
    message.operationTypeId !== undefined &&
      (obj.operationTypeId = message.operationTypeId)
    message.operationBody !== undefined &&
      (obj.operationBody = base64FromBytes(
        message.operationBody !== undefined
          ? message.operationBody
          : new Uint8Array(0)
      ))
    return obj
  },

  create<I extends Exact<DeepPartial<TxApplyWorldOp>, I>>(
    base?: I
  ): TxApplyWorldOp {
    return TxApplyWorldOp.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<TxApplyWorldOp>, I>>(
    object: I
  ): TxApplyWorldOp {
    const message = createBaseTxApplyWorldOp()
    message.operationTypeId = object.operationTypeId ?? ''
    message.operationBody = object.operationBody ?? new Uint8Array(0)
    return message
  },
}

function createBaseTxApplyObjectOp(): TxApplyObjectOp {
  return {
    operationTypeId: '',
    operationBody: new Uint8Array(0),
    objectKey: '',
  }
}

export const TxApplyObjectOp = {
  encode(
    message: TxApplyObjectOp,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.operationTypeId !== '') {
      writer.uint32(10).string(message.operationTypeId)
    }
    if (message.operationBody.length !== 0) {
      writer.uint32(18).bytes(message.operationBody)
    }
    if (message.objectKey !== '') {
      writer.uint32(26).string(message.objectKey)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxApplyObjectOp {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTxApplyObjectOp()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.operationTypeId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.operationBody = reader.bytes()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.objectKey = reader.string()
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
  // Transform<TxApplyObjectOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TxApplyObjectOp | TxApplyObjectOp[]>
      | Iterable<TxApplyObjectOp | TxApplyObjectOp[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxApplyObjectOp.encode(p).finish()]
        }
      } else {
        yield* [TxApplyObjectOp.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxApplyObjectOp>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TxApplyObjectOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxApplyObjectOp.decode(p)]
        }
      } else {
        yield* [TxApplyObjectOp.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TxApplyObjectOp {
    return {
      operationTypeId: isSet(object.operationTypeId)
        ? String(object.operationTypeId)
        : '',
      operationBody: isSet(object.operationBody)
        ? bytesFromBase64(object.operationBody)
        : new Uint8Array(0),
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : '',
    }
  },

  toJSON(message: TxApplyObjectOp): unknown {
    const obj: any = {}
    message.operationTypeId !== undefined &&
      (obj.operationTypeId = message.operationTypeId)
    message.operationBody !== undefined &&
      (obj.operationBody = base64FromBytes(
        message.operationBody !== undefined
          ? message.operationBody
          : new Uint8Array(0)
      ))
    message.objectKey !== undefined && (obj.objectKey = message.objectKey)
    return obj
  },

  create<I extends Exact<DeepPartial<TxApplyObjectOp>, I>>(
    base?: I
  ): TxApplyObjectOp {
    return TxApplyObjectOp.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<TxApplyObjectOp>, I>>(
    object: I
  ): TxApplyObjectOp {
    const message = createBaseTxApplyObjectOp()
    message.operationTypeId = object.operationTypeId ?? ''
    message.operationBody = object.operationBody ?? new Uint8Array(0)
    message.objectKey = object.objectKey ?? ''
    return message
  },
}

function createBaseTxCreateObject(): TxCreateObject {
  return { objectKey: '', rootRef: undefined }
}

export const TxCreateObject = {
  encode(
    message: TxCreateObject,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.objectKey !== '') {
      writer.uint32(10).string(message.objectKey)
    }
    if (message.rootRef !== undefined) {
      ObjectRef.encode(message.rootRef, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxCreateObject {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTxCreateObject()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.objectKey = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.rootRef = ObjectRef.decode(reader, reader.uint32())
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
  // Transform<TxCreateObject, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TxCreateObject | TxCreateObject[]>
      | Iterable<TxCreateObject | TxCreateObject[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxCreateObject.encode(p).finish()]
        }
      } else {
        yield* [TxCreateObject.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxCreateObject>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TxCreateObject> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxCreateObject.decode(p)]
        }
      } else {
        yield* [TxCreateObject.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TxCreateObject {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : '',
      rootRef: isSet(object.rootRef)
        ? ObjectRef.fromJSON(object.rootRef)
        : undefined,
    }
  },

  toJSON(message: TxCreateObject): unknown {
    const obj: any = {}
    message.objectKey !== undefined && (obj.objectKey = message.objectKey)
    message.rootRef !== undefined &&
      (obj.rootRef = message.rootRef
        ? ObjectRef.toJSON(message.rootRef)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<TxCreateObject>, I>>(
    base?: I
  ): TxCreateObject {
    return TxCreateObject.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<TxCreateObject>, I>>(
    object: I
  ): TxCreateObject {
    const message = createBaseTxCreateObject()
    message.objectKey = object.objectKey ?? ''
    message.rootRef =
      object.rootRef !== undefined && object.rootRef !== null
        ? ObjectRef.fromPartial(object.rootRef)
        : undefined
    return message
  },
}

function createBaseTxObjectSet(): TxObjectSet {
  return { objectKey: '', rootRef: undefined }
}

export const TxObjectSet = {
  encode(
    message: TxObjectSet,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.objectKey !== '') {
      writer.uint32(10).string(message.objectKey)
    }
    if (message.rootRef !== undefined) {
      ObjectRef.encode(message.rootRef, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxObjectSet {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTxObjectSet()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.objectKey = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.rootRef = ObjectRef.decode(reader, reader.uint32())
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
  // Transform<TxObjectSet, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TxObjectSet | TxObjectSet[]>
      | Iterable<TxObjectSet | TxObjectSet[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxObjectSet.encode(p).finish()]
        }
      } else {
        yield* [TxObjectSet.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxObjectSet>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TxObjectSet> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxObjectSet.decode(p)]
        }
      } else {
        yield* [TxObjectSet.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TxObjectSet {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : '',
      rootRef: isSet(object.rootRef)
        ? ObjectRef.fromJSON(object.rootRef)
        : undefined,
    }
  },

  toJSON(message: TxObjectSet): unknown {
    const obj: any = {}
    message.objectKey !== undefined && (obj.objectKey = message.objectKey)
    message.rootRef !== undefined &&
      (obj.rootRef = message.rootRef
        ? ObjectRef.toJSON(message.rootRef)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<TxObjectSet>, I>>(base?: I): TxObjectSet {
    return TxObjectSet.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<TxObjectSet>, I>>(
    object: I
  ): TxObjectSet {
    const message = createBaseTxObjectSet()
    message.objectKey = object.objectKey ?? ''
    message.rootRef =
      object.rootRef !== undefined && object.rootRef !== null
        ? ObjectRef.fromPartial(object.rootRef)
        : undefined
    return message
  },
}

function createBaseTxObjectIncRev(): TxObjectIncRev {
  return { objectKey: '' }
}

export const TxObjectIncRev = {
  encode(
    message: TxObjectIncRev,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.objectKey !== '') {
      writer.uint32(10).string(message.objectKey)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxObjectIncRev {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTxObjectIncRev()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.objectKey = reader.string()
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
  // Transform<TxObjectIncRev, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TxObjectIncRev | TxObjectIncRev[]>
      | Iterable<TxObjectIncRev | TxObjectIncRev[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxObjectIncRev.encode(p).finish()]
        }
      } else {
        yield* [TxObjectIncRev.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxObjectIncRev>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TxObjectIncRev> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxObjectIncRev.decode(p)]
        }
      } else {
        yield* [TxObjectIncRev.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TxObjectIncRev {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : '',
    }
  },

  toJSON(message: TxObjectIncRev): unknown {
    const obj: any = {}
    message.objectKey !== undefined && (obj.objectKey = message.objectKey)
    return obj
  },

  create<I extends Exact<DeepPartial<TxObjectIncRev>, I>>(
    base?: I
  ): TxObjectIncRev {
    return TxObjectIncRev.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<TxObjectIncRev>, I>>(
    object: I
  ): TxObjectIncRev {
    const message = createBaseTxObjectIncRev()
    message.objectKey = object.objectKey ?? ''
    return message
  },
}

function createBaseTxDeleteObject(): TxDeleteObject {
  return { objectKey: '', failIfNotFound: false }
}

export const TxDeleteObject = {
  encode(
    message: TxDeleteObject,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.objectKey !== '') {
      writer.uint32(10).string(message.objectKey)
    }
    if (message.failIfNotFound === true) {
      writer.uint32(16).bool(message.failIfNotFound)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxDeleteObject {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTxDeleteObject()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.objectKey = reader.string()
          continue
        case 2:
          if (tag !== 16) {
            break
          }

          message.failIfNotFound = reader.bool()
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
  // Transform<TxDeleteObject, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TxDeleteObject | TxDeleteObject[]>
      | Iterable<TxDeleteObject | TxDeleteObject[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxDeleteObject.encode(p).finish()]
        }
      } else {
        yield* [TxDeleteObject.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxDeleteObject>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TxDeleteObject> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxDeleteObject.decode(p)]
        }
      } else {
        yield* [TxDeleteObject.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TxDeleteObject {
    return {
      objectKey: isSet(object.objectKey) ? String(object.objectKey) : '',
      failIfNotFound: isSet(object.failIfNotFound)
        ? Boolean(object.failIfNotFound)
        : false,
    }
  },

  toJSON(message: TxDeleteObject): unknown {
    const obj: any = {}
    message.objectKey !== undefined && (obj.objectKey = message.objectKey)
    message.failIfNotFound !== undefined &&
      (obj.failIfNotFound = message.failIfNotFound)
    return obj
  },

  create<I extends Exact<DeepPartial<TxDeleteObject>, I>>(
    base?: I
  ): TxDeleteObject {
    return TxDeleteObject.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<TxDeleteObject>, I>>(
    object: I
  ): TxDeleteObject {
    const message = createBaseTxDeleteObject()
    message.objectKey = object.objectKey ?? ''
    message.failIfNotFound = object.failIfNotFound ?? false
    return message
  },
}

function createBaseTxSetGraphQuad(): TxSetGraphQuad {
  return { quad: undefined }
}

export const TxSetGraphQuad = {
  encode(
    message: TxSetGraphQuad,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.quad !== undefined) {
      Quad.encode(message.quad, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxSetGraphQuad {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTxSetGraphQuad()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.quad = Quad.decode(reader, reader.uint32())
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
  // Transform<TxSetGraphQuad, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TxSetGraphQuad | TxSetGraphQuad[]>
      | Iterable<TxSetGraphQuad | TxSetGraphQuad[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxSetGraphQuad.encode(p).finish()]
        }
      } else {
        yield* [TxSetGraphQuad.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxSetGraphQuad>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TxSetGraphQuad> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxSetGraphQuad.decode(p)]
        }
      } else {
        yield* [TxSetGraphQuad.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TxSetGraphQuad {
    return { quad: isSet(object.quad) ? Quad.fromJSON(object.quad) : undefined }
  },

  toJSON(message: TxSetGraphQuad): unknown {
    const obj: any = {}
    message.quad !== undefined &&
      (obj.quad = message.quad ? Quad.toJSON(message.quad) : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<TxSetGraphQuad>, I>>(
    base?: I
  ): TxSetGraphQuad {
    return TxSetGraphQuad.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<TxSetGraphQuad>, I>>(
    object: I
  ): TxSetGraphQuad {
    const message = createBaseTxSetGraphQuad()
    message.quad =
      object.quad !== undefined && object.quad !== null
        ? Quad.fromPartial(object.quad)
        : undefined
    return message
  },
}

function createBaseTxDeleteGraphQuad(): TxDeleteGraphQuad {
  return { quad: undefined }
}

export const TxDeleteGraphQuad = {
  encode(
    message: TxDeleteGraphQuad,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.quad !== undefined) {
      Quad.encode(message.quad, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TxDeleteGraphQuad {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTxDeleteGraphQuad()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.quad = Quad.decode(reader, reader.uint32())
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
  // Transform<TxDeleteGraphQuad, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TxDeleteGraphQuad | TxDeleteGraphQuad[]>
      | Iterable<TxDeleteGraphQuad | TxDeleteGraphQuad[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxDeleteGraphQuad.encode(p).finish()]
        }
      } else {
        yield* [TxDeleteGraphQuad.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TxDeleteGraphQuad>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TxDeleteGraphQuad> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TxDeleteGraphQuad.decode(p)]
        }
      } else {
        yield* [TxDeleteGraphQuad.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TxDeleteGraphQuad {
    return { quad: isSet(object.quad) ? Quad.fromJSON(object.quad) : undefined }
  },

  toJSON(message: TxDeleteGraphQuad): unknown {
    const obj: any = {}
    message.quad !== undefined &&
      (obj.quad = message.quad ? Quad.toJSON(message.quad) : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<TxDeleteGraphQuad>, I>>(
    base?: I
  ): TxDeleteGraphQuad {
    return TxDeleteGraphQuad.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<TxDeleteGraphQuad>, I>>(
    object: I
  ): TxDeleteGraphQuad {
    const message = createBaseTxDeleteGraphQuad()
    message.quad =
      object.quad !== undefined && object.quad !== null
        ? Quad.fromPartial(object.quad)
        : undefined
    return message
  },
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

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
