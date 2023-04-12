/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { BlockRef } from '../../block/block.pb.js'
import { KeyFilters } from '../../block/filters/filters.pb.js'
import { Quad } from '../../block/quad/quad.pb.js'
import { ObjectRef } from '../../bucket/bucket.pb.js'
import { KeyValueStore } from '../../kvtx/block/kvtx.pb.js'

export const protobufPackage = 'world.block'

/** WorldChangeType is the list of possible change types for the world. */
export enum WorldChangeType {
  WorldChange_INVALID = 0,
  WorldChange_OBJECT_SET = 1,
  WorldChange_OBJECT_INC_REV = 2,
  WorldChange_OBJECT_DELETE = 3,
  /** WorldChange_GRAPH_SET - WorldChange_GRAPH_SET is fired when setting a graph quad. */
  WorldChange_GRAPH_SET = 5,
  /** WorldChange_GRAPH_DELETE - WorldChange_GRAPH_DELETE is fired when deleting a graph quad. */
  WorldChange_GRAPH_DELETE = 6,
  UNRECOGNIZED = -1,
}

export function worldChangeTypeFromJSON(object: any): WorldChangeType {
  switch (object) {
    case 0:
    case 'WorldChange_INVALID':
      return WorldChangeType.WorldChange_INVALID
    case 1:
    case 'WorldChange_OBJECT_SET':
      return WorldChangeType.WorldChange_OBJECT_SET
    case 2:
    case 'WorldChange_OBJECT_INC_REV':
      return WorldChangeType.WorldChange_OBJECT_INC_REV
    case 3:
    case 'WorldChange_OBJECT_DELETE':
      return WorldChangeType.WorldChange_OBJECT_DELETE
    case 5:
    case 'WorldChange_GRAPH_SET':
      return WorldChangeType.WorldChange_GRAPH_SET
    case 6:
    case 'WorldChange_GRAPH_DELETE':
      return WorldChangeType.WorldChange_GRAPH_DELETE
    case -1:
    case 'UNRECOGNIZED':
    default:
      return WorldChangeType.UNRECOGNIZED
  }
}

export function worldChangeTypeToJSON(object: WorldChangeType): string {
  switch (object) {
    case WorldChangeType.WorldChange_INVALID:
      return 'WorldChange_INVALID'
    case WorldChangeType.WorldChange_OBJECT_SET:
      return 'WorldChange_OBJECT_SET'
    case WorldChangeType.WorldChange_OBJECT_INC_REV:
      return 'WorldChange_OBJECT_INC_REV'
    case WorldChangeType.WorldChange_OBJECT_DELETE:
      return 'WorldChange_OBJECT_DELETE'
    case WorldChangeType.WorldChange_GRAPH_SET:
      return 'WorldChange_GRAPH_SET'
    case WorldChangeType.WorldChange_GRAPH_DELETE:
      return 'WorldChange_GRAPH_DELETE'
    case WorldChangeType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/**
 * World contains a key/value Object store, and a graph database with quads
 * <subject, predicate, object, value>. Optionally a 2D changelog is used for
 * efficient change detection without needing to download every change.
 */
export interface World {
  /**
   * ObjectKeyValue is the key/value tree of objects.
   * Key: string
   * Value: cid.BlockRef -> Object
   */
  objectKeyValue: KeyValueStore | undefined
  /**
   * GraphKeyValue is the key/value tree storing the graph.
   * k/v structure managed by cayley graph kvtx implementation
   * Value: cid.BlockRef -> Quad
   */
  graphKeyValue: KeyValueStore | undefined
  /**
   * LastChange is the current head of the changelog linked list.
   * If seqno == 0, this field is empty.
   */
  lastChange: ChangeLogLL | undefined
  /**
   * LastChangeDisable indicates the changelog is disabled for this world.
   * If set, last_change will be empty, except for the seqno field.
   * NOTE: the seqno field will not be empty on LastChange.
   */
  lastChangeDisable: boolean
}

/** Object is an atomic unit for a object in a World graph. */
export interface Object {
  /** Key is the unique Object key. */
  key: string
  /**
   * RootRef is the block ref to the root of the object structure.
   * Note: Object type is not stored. Type data is stored in the Graph, inline, or not at all.
   */
  rootRef: ObjectRef | undefined
  /**
   * Rev is the revision nonce of the object.
   * Incremented when a transaction is applied to the object.
   * Incremented when root_ref is changed (SetRootRef).
   * Incremented when adding or removing a graph quad referencing Object.
   */
  rev: Long
}

/**
 * WorldChange is an entry in the changelog.
 * A transaction may convert into multiple changes.
 */
export interface WorldChange {
  /** ChangeType is the type of change this is. */
  changeType: WorldChangeType
  /**
   * Key is the associated key of the change.
   * May be a key prefix, depending on change type.
   * If a Graph transaction, this will be empty.
   */
  key: string
  /**
   * Quad is the associated graph quad of the change.
   * If a Object transaction, this will be empty.
   */
  quad: Quad | undefined
  /**
   * TransactionRef is the reference to the associated transaction.
   * This is transparent to the core World code.
   */
  transactionRef: BlockRef | undefined
  /**
   * ObjectRef is the reference to the associated Object block.
   * Empty for graph operations.
   */
  objectRef: BlockRef | undefined
  /**
   * PrevObjectRef is the reference to the associated previous Object block.
   * If set, this will be the old object.
   * If deleted, this will be the object just before deletion.
   * Empty for graph operations.
   */
  prevObjectRef: BlockRef | undefined
  /**
   * ObjectRev is the updated revision of the Object.
   * If a Graph transaction, this will be empty.
   */
  objectRev: Long
}

/** WorldChangeLL is a linked-list of world change batches. */
export interface WorldChangeLL {
  /**
   * Height is the index in the batch linked-list.
   * The first change in the set is at height=0.
   */
  height: number
  /**
   * PrevRef is the reference to the previous WorldChangeLL in the linked list.
   * If height == 0, this field must be empty.
   */
  prevRef: BlockRef | undefined
  /** TotalSize is len(changes) + total_size of prev node in list. */
  totalSize: number
  /**
   * Changes is the set of changes in the world change batch.
   * Changes are added until the batch reaches the target batch size.
   */
  changes: WorldChange[]
}

/**
 * ChangeLogLL is a world change log linked-list entry.
 * The size of a ChangeLogLL entry is capped to 1MiB, targeting 512KiB.
 */
export interface ChangeLogLL {
  /** Seqno is the world sequence number after this changeset is applied. */
  seqno: Long
  /**
   * PrevRef is the reference to the previous change.
   * If seqno <= 1, this must be empty.
   */
  prevRef: BlockRef | undefined
  /**
   * ChangeBatch is the world change batch linked-list first node.
   * Linked-list is used to reduce the size of changelog entries.
   * The HEAD of the ChangeLogLL is limited to 5 embedded changes.
   * The nodes of the ChangeLogLL are limited to 2048 entries (~512KiB).
   */
  changeBatch: WorldChangeLL | undefined
  /**
   * ChangeType is the type of change applied.
   * If there are multiple changes, they will all be of this type.
   */
  changeType: WorldChangeType
  /**
   * KeyFilters contains filters to quickly check if a key was affected.
   * Bloom capacity is the object key count before changes are applied.
   * Bloom capacity: min 64 (38bytes), max 500k (300KiB).
   * Bloom false-negative rate 0%, false-positive rate ~10%.
   * Key prefix false-negative rate depends on common prefix of ops.
   * Should be used as a changelog filter for watchers.
   * If change_batch.prev_ref is empty, this field will also be empty.
   */
  keyFilters: KeyFilters | undefined
}

function createBaseWorld(): World {
  return {
    objectKeyValue: undefined,
    graphKeyValue: undefined,
    lastChange: undefined,
    lastChangeDisable: false,
  }
}

export const World = {
  encode(message: World, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.objectKeyValue !== undefined) {
      KeyValueStore.encode(
        message.objectKeyValue,
        writer.uint32(10).fork()
      ).ldelim()
    }
    if (message.graphKeyValue !== undefined) {
      KeyValueStore.encode(
        message.graphKeyValue,
        writer.uint32(18).fork()
      ).ldelim()
    }
    if (message.lastChange !== undefined) {
      ChangeLogLL.encode(message.lastChange, writer.uint32(26).fork()).ldelim()
    }
    if (message.lastChangeDisable === true) {
      writer.uint32(32).bool(message.lastChangeDisable)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): World {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWorld()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break
          }

          message.objectKeyValue = KeyValueStore.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.graphKeyValue = KeyValueStore.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag != 26) {
            break
          }

          message.lastChange = ChangeLogLL.decode(reader, reader.uint32())
          continue
        case 4:
          if (tag != 32) {
            break
          }

          message.lastChangeDisable = reader.bool()
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
  // Transform<World, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<World | World[]> | Iterable<World | World[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [World.encode(p).finish()]
        }
      } else {
        yield* [World.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, World>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<World> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [World.decode(p)]
        }
      } else {
        yield* [World.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): World {
    return {
      objectKeyValue: isSet(object.objectKeyValue)
        ? KeyValueStore.fromJSON(object.objectKeyValue)
        : undefined,
      graphKeyValue: isSet(object.graphKeyValue)
        ? KeyValueStore.fromJSON(object.graphKeyValue)
        : undefined,
      lastChange: isSet(object.lastChange)
        ? ChangeLogLL.fromJSON(object.lastChange)
        : undefined,
      lastChangeDisable: isSet(object.lastChangeDisable)
        ? Boolean(object.lastChangeDisable)
        : false,
    }
  },

  toJSON(message: World): unknown {
    const obj: any = {}
    message.objectKeyValue !== undefined &&
      (obj.objectKeyValue = message.objectKeyValue
        ? KeyValueStore.toJSON(message.objectKeyValue)
        : undefined)
    message.graphKeyValue !== undefined &&
      (obj.graphKeyValue = message.graphKeyValue
        ? KeyValueStore.toJSON(message.graphKeyValue)
        : undefined)
    message.lastChange !== undefined &&
      (obj.lastChange = message.lastChange
        ? ChangeLogLL.toJSON(message.lastChange)
        : undefined)
    message.lastChangeDisable !== undefined &&
      (obj.lastChangeDisable = message.lastChangeDisable)
    return obj
  },

  create<I extends Exact<DeepPartial<World>, I>>(base?: I): World {
    return World.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<World>, I>>(object: I): World {
    const message = createBaseWorld()
    message.objectKeyValue =
      object.objectKeyValue !== undefined && object.objectKeyValue !== null
        ? KeyValueStore.fromPartial(object.objectKeyValue)
        : undefined
    message.graphKeyValue =
      object.graphKeyValue !== undefined && object.graphKeyValue !== null
        ? KeyValueStore.fromPartial(object.graphKeyValue)
        : undefined
    message.lastChange =
      object.lastChange !== undefined && object.lastChange !== null
        ? ChangeLogLL.fromPartial(object.lastChange)
        : undefined
    message.lastChangeDisable = object.lastChangeDisable ?? false
    return message
  },
}

function createBaseObject(): Object {
  return { key: '', rootRef: undefined, rev: Long.UZERO }
}

export const Object = {
  encode(
    message: Object,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.key !== '') {
      writer.uint32(10).string(message.key)
    }
    if (message.rootRef !== undefined) {
      ObjectRef.encode(message.rootRef, writer.uint32(18).fork()).ldelim()
    }
    if (!message.rev.isZero()) {
      writer.uint32(24).uint64(message.rev)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Object {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseObject()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break
          }

          message.key = reader.string()
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.rootRef = ObjectRef.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag != 24) {
            break
          }

          message.rev = reader.uint64() as Long
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
  // Transform<Object, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Object | Object[]> | Iterable<Object | Object[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Object.encode(p).finish()]
        }
      } else {
        yield* [Object.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Object>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Object> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Object.decode(p)]
        }
      } else {
        yield* [Object.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Object {
    return {
      key: isSet(object.key) ? String(object.key) : '',
      rootRef: isSet(object.rootRef)
        ? ObjectRef.fromJSON(object.rootRef)
        : undefined,
      rev: isSet(object.rev) ? Long.fromValue(object.rev) : Long.UZERO,
    }
  },

  toJSON(message: Object): unknown {
    const obj: any = {}
    message.key !== undefined && (obj.key = message.key)
    message.rootRef !== undefined &&
      (obj.rootRef = message.rootRef
        ? ObjectRef.toJSON(message.rootRef)
        : undefined)
    message.rev !== undefined &&
      (obj.rev = (message.rev || Long.UZERO).toString())
    return obj
  },

  create<I extends Exact<DeepPartial<Object>, I>>(base?: I): Object {
    return Object.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Object>, I>>(object: I): Object {
    const message = createBaseObject()
    message.key = object.key ?? ''
    message.rootRef =
      object.rootRef !== undefined && object.rootRef !== null
        ? ObjectRef.fromPartial(object.rootRef)
        : undefined
    message.rev =
      object.rev !== undefined && object.rev !== null
        ? Long.fromValue(object.rev)
        : Long.UZERO
    return message
  },
}

function createBaseWorldChange(): WorldChange {
  return {
    changeType: 0,
    key: '',
    quad: undefined,
    transactionRef: undefined,
    objectRef: undefined,
    prevObjectRef: undefined,
    objectRev: Long.UZERO,
  }
}

export const WorldChange = {
  encode(
    message: WorldChange,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.changeType !== 0) {
      writer.uint32(8).int32(message.changeType)
    }
    if (message.key !== '') {
      writer.uint32(18).string(message.key)
    }
    if (message.quad !== undefined) {
      Quad.encode(message.quad, writer.uint32(26).fork()).ldelim()
    }
    if (message.transactionRef !== undefined) {
      BlockRef.encode(message.transactionRef, writer.uint32(34).fork()).ldelim()
    }
    if (message.objectRef !== undefined) {
      BlockRef.encode(message.objectRef, writer.uint32(42).fork()).ldelim()
    }
    if (message.prevObjectRef !== undefined) {
      BlockRef.encode(message.prevObjectRef, writer.uint32(50).fork()).ldelim()
    }
    if (!message.objectRev.isZero()) {
      writer.uint32(56).uint64(message.objectRev)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WorldChange {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWorldChange()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag != 8) {
            break
          }

          message.changeType = reader.int32() as any
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.key = reader.string()
          continue
        case 3:
          if (tag != 26) {
            break
          }

          message.quad = Quad.decode(reader, reader.uint32())
          continue
        case 4:
          if (tag != 34) {
            break
          }

          message.transactionRef = BlockRef.decode(reader, reader.uint32())
          continue
        case 5:
          if (tag != 42) {
            break
          }

          message.objectRef = BlockRef.decode(reader, reader.uint32())
          continue
        case 6:
          if (tag != 50) {
            break
          }

          message.prevObjectRef = BlockRef.decode(reader, reader.uint32())
          continue
        case 7:
          if (tag != 56) {
            break
          }

          message.objectRev = reader.uint64() as Long
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
  // Transform<WorldChange, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WorldChange | WorldChange[]>
      | Iterable<WorldChange | WorldChange[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WorldChange.encode(p).finish()]
        }
      } else {
        yield* [WorldChange.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WorldChange>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<WorldChange> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WorldChange.decode(p)]
        }
      } else {
        yield* [WorldChange.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): WorldChange {
    return {
      changeType: isSet(object.changeType)
        ? worldChangeTypeFromJSON(object.changeType)
        : 0,
      key: isSet(object.key) ? String(object.key) : '',
      quad: isSet(object.quad) ? Quad.fromJSON(object.quad) : undefined,
      transactionRef: isSet(object.transactionRef)
        ? BlockRef.fromJSON(object.transactionRef)
        : undefined,
      objectRef: isSet(object.objectRef)
        ? BlockRef.fromJSON(object.objectRef)
        : undefined,
      prevObjectRef: isSet(object.prevObjectRef)
        ? BlockRef.fromJSON(object.prevObjectRef)
        : undefined,
      objectRev: isSet(object.objectRev)
        ? Long.fromValue(object.objectRev)
        : Long.UZERO,
    }
  },

  toJSON(message: WorldChange): unknown {
    const obj: any = {}
    message.changeType !== undefined &&
      (obj.changeType = worldChangeTypeToJSON(message.changeType))
    message.key !== undefined && (obj.key = message.key)
    message.quad !== undefined &&
      (obj.quad = message.quad ? Quad.toJSON(message.quad) : undefined)
    message.transactionRef !== undefined &&
      (obj.transactionRef = message.transactionRef
        ? BlockRef.toJSON(message.transactionRef)
        : undefined)
    message.objectRef !== undefined &&
      (obj.objectRef = message.objectRef
        ? BlockRef.toJSON(message.objectRef)
        : undefined)
    message.prevObjectRef !== undefined &&
      (obj.prevObjectRef = message.prevObjectRef
        ? BlockRef.toJSON(message.prevObjectRef)
        : undefined)
    message.objectRev !== undefined &&
      (obj.objectRev = (message.objectRev || Long.UZERO).toString())
    return obj
  },

  create<I extends Exact<DeepPartial<WorldChange>, I>>(base?: I): WorldChange {
    return WorldChange.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<WorldChange>, I>>(
    object: I
  ): WorldChange {
    const message = createBaseWorldChange()
    message.changeType = object.changeType ?? 0
    message.key = object.key ?? ''
    message.quad =
      object.quad !== undefined && object.quad !== null
        ? Quad.fromPartial(object.quad)
        : undefined
    message.transactionRef =
      object.transactionRef !== undefined && object.transactionRef !== null
        ? BlockRef.fromPartial(object.transactionRef)
        : undefined
    message.objectRef =
      object.objectRef !== undefined && object.objectRef !== null
        ? BlockRef.fromPartial(object.objectRef)
        : undefined
    message.prevObjectRef =
      object.prevObjectRef !== undefined && object.prevObjectRef !== null
        ? BlockRef.fromPartial(object.prevObjectRef)
        : undefined
    message.objectRev =
      object.objectRev !== undefined && object.objectRev !== null
        ? Long.fromValue(object.objectRev)
        : Long.UZERO
    return message
  },
}

function createBaseWorldChangeLL(): WorldChangeLL {
  return { height: 0, prevRef: undefined, totalSize: 0, changes: [] }
}

export const WorldChangeLL = {
  encode(
    message: WorldChangeLL,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.height !== 0) {
      writer.uint32(8).uint32(message.height)
    }
    if (message.prevRef !== undefined) {
      BlockRef.encode(message.prevRef, writer.uint32(18).fork()).ldelim()
    }
    if (message.totalSize !== 0) {
      writer.uint32(24).uint32(message.totalSize)
    }
    for (const v of message.changes) {
      WorldChange.encode(v!, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WorldChangeLL {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseWorldChangeLL()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag != 8) {
            break
          }

          message.height = reader.uint32()
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.prevRef = BlockRef.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag != 24) {
            break
          }

          message.totalSize = reader.uint32()
          continue
        case 4:
          if (tag != 34) {
            break
          }

          message.changes.push(WorldChange.decode(reader, reader.uint32()))
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
  // Transform<WorldChangeLL, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WorldChangeLL | WorldChangeLL[]>
      | Iterable<WorldChangeLL | WorldChangeLL[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WorldChangeLL.encode(p).finish()]
        }
      } else {
        yield* [WorldChangeLL.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WorldChangeLL>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<WorldChangeLL> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [WorldChangeLL.decode(p)]
        }
      } else {
        yield* [WorldChangeLL.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): WorldChangeLL {
    return {
      height: isSet(object.height) ? Number(object.height) : 0,
      prevRef: isSet(object.prevRef)
        ? BlockRef.fromJSON(object.prevRef)
        : undefined,
      totalSize: isSet(object.totalSize) ? Number(object.totalSize) : 0,
      changes: Array.isArray(object?.changes)
        ? object.changes.map((e: any) => WorldChange.fromJSON(e))
        : [],
    }
  },

  toJSON(message: WorldChangeLL): unknown {
    const obj: any = {}
    message.height !== undefined && (obj.height = Math.round(message.height))
    message.prevRef !== undefined &&
      (obj.prevRef = message.prevRef
        ? BlockRef.toJSON(message.prevRef)
        : undefined)
    message.totalSize !== undefined &&
      (obj.totalSize = Math.round(message.totalSize))
    if (message.changes) {
      obj.changes = message.changes.map((e) =>
        e ? WorldChange.toJSON(e) : undefined
      )
    } else {
      obj.changes = []
    }
    return obj
  },

  create<I extends Exact<DeepPartial<WorldChangeLL>, I>>(
    base?: I
  ): WorldChangeLL {
    return WorldChangeLL.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<WorldChangeLL>, I>>(
    object: I
  ): WorldChangeLL {
    const message = createBaseWorldChangeLL()
    message.height = object.height ?? 0
    message.prevRef =
      object.prevRef !== undefined && object.prevRef !== null
        ? BlockRef.fromPartial(object.prevRef)
        : undefined
    message.totalSize = object.totalSize ?? 0
    message.changes =
      object.changes?.map((e) => WorldChange.fromPartial(e)) || []
    return message
  },
}

function createBaseChangeLogLL(): ChangeLogLL {
  return {
    seqno: Long.UZERO,
    prevRef: undefined,
    changeBatch: undefined,
    changeType: 0,
    keyFilters: undefined,
  }
}

export const ChangeLogLL = {
  encode(
    message: ChangeLogLL,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (!message.seqno.isZero()) {
      writer.uint32(8).uint64(message.seqno)
    }
    if (message.prevRef !== undefined) {
      BlockRef.encode(message.prevRef, writer.uint32(18).fork()).ldelim()
    }
    if (message.changeBatch !== undefined) {
      WorldChangeLL.encode(
        message.changeBatch,
        writer.uint32(26).fork()
      ).ldelim()
    }
    if (message.changeType !== 0) {
      writer.uint32(32).int32(message.changeType)
    }
    if (message.keyFilters !== undefined) {
      KeyFilters.encode(message.keyFilters, writer.uint32(42).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ChangeLogLL {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseChangeLogLL()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag != 8) {
            break
          }

          message.seqno = reader.uint64() as Long
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.prevRef = BlockRef.decode(reader, reader.uint32())
          continue
        case 3:
          if (tag != 26) {
            break
          }

          message.changeBatch = WorldChangeLL.decode(reader, reader.uint32())
          continue
        case 4:
          if (tag != 32) {
            break
          }

          message.changeType = reader.int32() as any
          continue
        case 5:
          if (tag != 42) {
            break
          }

          message.keyFilters = KeyFilters.decode(reader, reader.uint32())
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
  // Transform<ChangeLogLL, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ChangeLogLL | ChangeLogLL[]>
      | Iterable<ChangeLogLL | ChangeLogLL[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ChangeLogLL.encode(p).finish()]
        }
      } else {
        yield* [ChangeLogLL.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ChangeLogLL>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ChangeLogLL> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ChangeLogLL.decode(p)]
        }
      } else {
        yield* [ChangeLogLL.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ChangeLogLL {
    return {
      seqno: isSet(object.seqno) ? Long.fromValue(object.seqno) : Long.UZERO,
      prevRef: isSet(object.prevRef)
        ? BlockRef.fromJSON(object.prevRef)
        : undefined,
      changeBatch: isSet(object.changeBatch)
        ? WorldChangeLL.fromJSON(object.changeBatch)
        : undefined,
      changeType: isSet(object.changeType)
        ? worldChangeTypeFromJSON(object.changeType)
        : 0,
      keyFilters: isSet(object.keyFilters)
        ? KeyFilters.fromJSON(object.keyFilters)
        : undefined,
    }
  },

  toJSON(message: ChangeLogLL): unknown {
    const obj: any = {}
    message.seqno !== undefined &&
      (obj.seqno = (message.seqno || Long.UZERO).toString())
    message.prevRef !== undefined &&
      (obj.prevRef = message.prevRef
        ? BlockRef.toJSON(message.prevRef)
        : undefined)
    message.changeBatch !== undefined &&
      (obj.changeBatch = message.changeBatch
        ? WorldChangeLL.toJSON(message.changeBatch)
        : undefined)
    message.changeType !== undefined &&
      (obj.changeType = worldChangeTypeToJSON(message.changeType))
    message.keyFilters !== undefined &&
      (obj.keyFilters = message.keyFilters
        ? KeyFilters.toJSON(message.keyFilters)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<ChangeLogLL>, I>>(base?: I): ChangeLogLL {
    return ChangeLogLL.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ChangeLogLL>, I>>(
    object: I
  ): ChangeLogLL {
    const message = createBaseChangeLogLL()
    message.seqno =
      object.seqno !== undefined && object.seqno !== null
        ? Long.fromValue(object.seqno)
        : Long.UZERO
    message.prevRef =
      object.prevRef !== undefined && object.prevRef !== null
        ? BlockRef.fromPartial(object.prevRef)
        : undefined
    message.changeBatch =
      object.changeBatch !== undefined && object.changeBatch !== null
        ? WorldChangeLL.fromPartial(object.changeBatch)
        : undefined
    message.changeType = object.changeType ?? 0
    message.keyFilters =
      object.keyFilters !== undefined && object.keyFilters !== null
        ? KeyFilters.fromPartial(object.keyFilters)
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

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
