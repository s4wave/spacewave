/* eslint-disable */
import Long from 'long'
import { BlockRef } from '../../block/block.pb.js'
import { MsgpackBlob } from '../../block/msgpack/msgpack.pb.js'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'mysql'

/**
 * PartitionImpl contains the sets of partition implementations.
 *
 * TODO: move to kvtx_block instead
 */
export enum PartitionImpl {
  /**
   * PartitionImpl_IAVL - PartitionImpl_IAVL is the default value.
   * Uses block/iavl/iavl.proto structure
   * Default value: readers should check the value is actually zero.
   */
  PartitionImpl_IAVL = 0,
  UNRECOGNIZED = -1,
}

export function partitionImplFromJSON(object: any): PartitionImpl {
  switch (object) {
    case 0:
    case 'PartitionImpl_IAVL':
      return PartitionImpl.PartitionImpl_IAVL
    case -1:
    case 'UNRECOGNIZED':
    default:
      return PartitionImpl.UNRECOGNIZED
  }
}

export function partitionImplToJSON(object: PartitionImpl): string {
  switch (object) {
    case PartitionImpl.PartitionImpl_IAVL:
      return 'PartitionImpl_IAVL'
    case PartitionImpl.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/** Root is the root object of the mysql database. */
export interface Root {
  /** Databases contains the root set of databases, sorted by name. */
  databases: RootDb[]
}

/** RootDb contains the root definition of a database. */
export interface RootDb {
  /** Name is the unique name of the database. */
  name: string
  /**
   * Ref is the block reference to the database root.
   * If empty, the database is empty.
   * Type: DatabaseRoot.
   */
  ref: BlockRef | undefined
}

/** DatabaseRoot is the root object of the database. */
export interface DatabaseRoot {
  /** Tables contains the table list sorted by name. */
  tables: DatabaseRootTable[]
}

/** DatabaseRootTable contains the reference to the TableRoot. */
export interface DatabaseRootTable {
  /** Name is the unique name of the table. */
  name: string
  /**
   * Ref is the block reference to the table root.
   * Type: TableRoot.
   */
  ref: BlockRef | undefined
}

/** TableRoot is the root object of the table. */
export interface TableRoot {
  /** TableSchema is the table schema. */
  tableSchema: TableSchema | undefined
  /** CollationId is the collation method id. */
  collationId: number
  /** PrimaryKeyOrdinals is the PkOrdinals field of PrimaryKeySchema. */
  primaryKeyOrdinals: number[]
  /** TablePartitions contains the set of table partitions. */
  tablePartitions: TablePartitionRoot[]
  /** RowNonce is the row identifier nonce, incremented when a row is inserted. */
  rowNonce: Long
  /**
   * AutoIncrVal is the auto increment value, if necessary.
   * Typically contains an integer or float.
   * Empty if auto_incr_index is zero.
   */
  autoIncrVal: TableColumn | undefined
}

/** TablePartitionRoot contains the root of a table partition. */
export interface TablePartitionRoot {
  /**
   * TreeRef contains a reference to the row tree.
   *
   * Key: row insertion index (nonce).
   * Value: TablePartitionRow (encoded).
   */
  treeRef: BlockRef | undefined
  /** PartitionImpl contains the partition implementation id. */
  partitionImpl: PartitionImpl
}

/** TablePartitionRow is an entry in the table partition row tree. */
export interface TablePartitionRow {
  /**
   * RowNonce is the row identifier nonce
   *
   * key in the tree: row_nonce encoded big endian uint64
   */
  rowNonce: Long
  /** TableRowRef is the reference to the TableRow. */
  tableRowRef: BlockRef | undefined
}

/** TableRow is a row in a table. */
export interface TableRow {
  /** Columns contains the set of columns. */
  columns: TableColumn[]
}

/** TableColumn is an entry in a table row. */
export interface TableColumn {
  /**
   * MsgpackBlob contains the data encoded with msgpack.
   * Data may be sharded into multiple blocks if necessary.
   */
  msgpackBlob: MsgpackBlob | undefined
}

/** TableSchema is the schema for a table. */
export interface TableSchema {
  /** Columns is the list of columns in the table, sorted by name. */
  columns: TableSchemaColumn[]
}

/** TableSchemaColumn is the definition of a column for a table schema. */
export interface TableSchemaColumn {
  /** Name is the name of the column. */
  name: string
  /** ColumnType is the data type of the column. */
  columnType: string
  /** DefaultValueExpr is the default value expression, encoded to a string. */
  defaultValueExpr: string
  /** AutoIncrement is true if the column auto-increments. */
  autoIncrement: boolean
  /** Nullable is true if the column can contain NULL values, or false otherwise. */
  nullable: boolean
  /** Source is the name of the table this column came from. */
  source: string
  /** PrimaryKey is true if the column is part of the primary key for its table. */
  primaryKey: boolean
  /** Comment contains the string comment for this column. */
  comment: string
  /** Extra contains any additional information to put in the `extra` column under `information_schema.columns`. */
  extra: string
}

function createBaseRoot(): Root {
  return { databases: [] }
}

export const Root = {
  encode(message: Root, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.databases) {
      RootDb.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Root {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRoot()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.databases.push(RootDb.decode(reader, reader.uint32()))
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Root, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Root | Root[]> | Iterable<Root | Root[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Root.encode(p).finish()]
        }
      } else {
        yield* [Root.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Root>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Root> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Root.decode(p)]
        }
      } else {
        yield* [Root.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Root {
    return {
      databases: Array.isArray(object?.databases)
        ? object.databases.map((e: any) => RootDb.fromJSON(e))
        : [],
    }
  },

  toJSON(message: Root): unknown {
    const obj: any = {}
    if (message.databases) {
      obj.databases = message.databases.map((e) =>
        e ? RootDb.toJSON(e) : undefined
      )
    } else {
      obj.databases = []
    }
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Root>, I>>(object: I): Root {
    const message = createBaseRoot()
    message.databases =
      object.databases?.map((e) => RootDb.fromPartial(e)) || []
    return message
  },
}

function createBaseRootDb(): RootDb {
  return { name: '', ref: undefined }
}

export const RootDb = {
  encode(
    message: RootDb,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.name !== '') {
      writer.uint32(10).string(message.name)
    }
    if (message.ref !== undefined) {
      BlockRef.encode(message.ref, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RootDb {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRootDb()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.name = reader.string()
          break
        case 2:
          message.ref = BlockRef.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<RootDb, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<RootDb | RootDb[]> | Iterable<RootDb | RootDb[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RootDb.encode(p).finish()]
        }
      } else {
        yield* [RootDb.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RootDb>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<RootDb> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RootDb.decode(p)]
        }
      } else {
        yield* [RootDb.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): RootDb {
    return {
      name: isSet(object.name) ? String(object.name) : '',
      ref: isSet(object.ref) ? BlockRef.fromJSON(object.ref) : undefined,
    }
  },

  toJSON(message: RootDb): unknown {
    const obj: any = {}
    message.name !== undefined && (obj.name = message.name)
    message.ref !== undefined &&
      (obj.ref = message.ref ? BlockRef.toJSON(message.ref) : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<RootDb>, I>>(object: I): RootDb {
    const message = createBaseRootDb()
    message.name = object.name ?? ''
    message.ref =
      object.ref !== undefined && object.ref !== null
        ? BlockRef.fromPartial(object.ref)
        : undefined
    return message
  },
}

function createBaseDatabaseRoot(): DatabaseRoot {
  return { tables: [] }
}

export const DatabaseRoot = {
  encode(
    message: DatabaseRoot,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    for (const v of message.tables) {
      DatabaseRootTable.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DatabaseRoot {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseDatabaseRoot()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.tables.push(DatabaseRootTable.decode(reader, reader.uint32()))
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<DatabaseRoot, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<DatabaseRoot | DatabaseRoot[]>
      | Iterable<DatabaseRoot | DatabaseRoot[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [DatabaseRoot.encode(p).finish()]
        }
      } else {
        yield* [DatabaseRoot.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, DatabaseRoot>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<DatabaseRoot> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [DatabaseRoot.decode(p)]
        }
      } else {
        yield* [DatabaseRoot.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): DatabaseRoot {
    return {
      tables: Array.isArray(object?.tables)
        ? object.tables.map((e: any) => DatabaseRootTable.fromJSON(e))
        : [],
    }
  },

  toJSON(message: DatabaseRoot): unknown {
    const obj: any = {}
    if (message.tables) {
      obj.tables = message.tables.map((e) =>
        e ? DatabaseRootTable.toJSON(e) : undefined
      )
    } else {
      obj.tables = []
    }
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<DatabaseRoot>, I>>(
    object: I
  ): DatabaseRoot {
    const message = createBaseDatabaseRoot()
    message.tables =
      object.tables?.map((e) => DatabaseRootTable.fromPartial(e)) || []
    return message
  },
}

function createBaseDatabaseRootTable(): DatabaseRootTable {
  return { name: '', ref: undefined }
}

export const DatabaseRootTable = {
  encode(
    message: DatabaseRootTable,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.name !== '') {
      writer.uint32(10).string(message.name)
    }
    if (message.ref !== undefined) {
      BlockRef.encode(message.ref, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DatabaseRootTable {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseDatabaseRootTable()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.name = reader.string()
          break
        case 2:
          message.ref = BlockRef.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<DatabaseRootTable, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<DatabaseRootTable | DatabaseRootTable[]>
      | Iterable<DatabaseRootTable | DatabaseRootTable[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [DatabaseRootTable.encode(p).finish()]
        }
      } else {
        yield* [DatabaseRootTable.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, DatabaseRootTable>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<DatabaseRootTable> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [DatabaseRootTable.decode(p)]
        }
      } else {
        yield* [DatabaseRootTable.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): DatabaseRootTable {
    return {
      name: isSet(object.name) ? String(object.name) : '',
      ref: isSet(object.ref) ? BlockRef.fromJSON(object.ref) : undefined,
    }
  },

  toJSON(message: DatabaseRootTable): unknown {
    const obj: any = {}
    message.name !== undefined && (obj.name = message.name)
    message.ref !== undefined &&
      (obj.ref = message.ref ? BlockRef.toJSON(message.ref) : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<DatabaseRootTable>, I>>(
    object: I
  ): DatabaseRootTable {
    const message = createBaseDatabaseRootTable()
    message.name = object.name ?? ''
    message.ref =
      object.ref !== undefined && object.ref !== null
        ? BlockRef.fromPartial(object.ref)
        : undefined
    return message
  },
}

function createBaseTableRoot(): TableRoot {
  return {
    tableSchema: undefined,
    collationId: 0,
    primaryKeyOrdinals: [],
    tablePartitions: [],
    rowNonce: Long.UZERO,
    autoIncrVal: undefined,
  }
}

export const TableRoot = {
  encode(
    message: TableRoot,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.tableSchema !== undefined) {
      TableSchema.encode(message.tableSchema, writer.uint32(10).fork()).ldelim()
    }
    if (message.collationId !== 0) {
      writer.uint32(48).uint32(message.collationId)
    }
    writer.uint32(42).fork()
    for (const v of message.primaryKeyOrdinals) {
      writer.int32(v)
    }
    writer.ldelim()
    for (const v of message.tablePartitions) {
      TablePartitionRoot.encode(v!, writer.uint32(18).fork()).ldelim()
    }
    if (!message.rowNonce.isZero()) {
      writer.uint32(24).uint64(message.rowNonce)
    }
    if (message.autoIncrVal !== undefined) {
      TableColumn.encode(message.autoIncrVal, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TableRoot {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTableRoot()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.tableSchema = TableSchema.decode(reader, reader.uint32())
          break
        case 6:
          message.collationId = reader.uint32()
          break
        case 5:
          if ((tag & 7) === 2) {
            const end2 = reader.uint32() + reader.pos
            while (reader.pos < end2) {
              message.primaryKeyOrdinals.push(reader.int32())
            }
          } else {
            message.primaryKeyOrdinals.push(reader.int32())
          }
          break
        case 2:
          message.tablePartitions.push(
            TablePartitionRoot.decode(reader, reader.uint32())
          )
          break
        case 3:
          message.rowNonce = reader.uint64() as Long
          break
        case 4:
          message.autoIncrVal = TableColumn.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<TableRoot, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TableRoot | TableRoot[]>
      | Iterable<TableRoot | TableRoot[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TableRoot.encode(p).finish()]
        }
      } else {
        yield* [TableRoot.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TableRoot>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TableRoot> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TableRoot.decode(p)]
        }
      } else {
        yield* [TableRoot.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TableRoot {
    return {
      tableSchema: isSet(object.tableSchema)
        ? TableSchema.fromJSON(object.tableSchema)
        : undefined,
      collationId: isSet(object.collationId) ? Number(object.collationId) : 0,
      primaryKeyOrdinals: Array.isArray(object?.primaryKeyOrdinals)
        ? object.primaryKeyOrdinals.map((e: any) => Number(e))
        : [],
      tablePartitions: Array.isArray(object?.tablePartitions)
        ? object.tablePartitions.map((e: any) => TablePartitionRoot.fromJSON(e))
        : [],
      rowNonce: isSet(object.rowNonce)
        ? Long.fromValue(object.rowNonce)
        : Long.UZERO,
      autoIncrVal: isSet(object.autoIncrVal)
        ? TableColumn.fromJSON(object.autoIncrVal)
        : undefined,
    }
  },

  toJSON(message: TableRoot): unknown {
    const obj: any = {}
    message.tableSchema !== undefined &&
      (obj.tableSchema = message.tableSchema
        ? TableSchema.toJSON(message.tableSchema)
        : undefined)
    message.collationId !== undefined &&
      (obj.collationId = Math.round(message.collationId))
    if (message.primaryKeyOrdinals) {
      obj.primaryKeyOrdinals = message.primaryKeyOrdinals.map((e) =>
        Math.round(e)
      )
    } else {
      obj.primaryKeyOrdinals = []
    }
    if (message.tablePartitions) {
      obj.tablePartitions = message.tablePartitions.map((e) =>
        e ? TablePartitionRoot.toJSON(e) : undefined
      )
    } else {
      obj.tablePartitions = []
    }
    message.rowNonce !== undefined &&
      (obj.rowNonce = (message.rowNonce || Long.UZERO).toString())
    message.autoIncrVal !== undefined &&
      (obj.autoIncrVal = message.autoIncrVal
        ? TableColumn.toJSON(message.autoIncrVal)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<TableRoot>, I>>(
    object: I
  ): TableRoot {
    const message = createBaseTableRoot()
    message.tableSchema =
      object.tableSchema !== undefined && object.tableSchema !== null
        ? TableSchema.fromPartial(object.tableSchema)
        : undefined
    message.collationId = object.collationId ?? 0
    message.primaryKeyOrdinals = object.primaryKeyOrdinals?.map((e) => e) || []
    message.tablePartitions =
      object.tablePartitions?.map((e) => TablePartitionRoot.fromPartial(e)) ||
      []
    message.rowNonce =
      object.rowNonce !== undefined && object.rowNonce !== null
        ? Long.fromValue(object.rowNonce)
        : Long.UZERO
    message.autoIncrVal =
      object.autoIncrVal !== undefined && object.autoIncrVal !== null
        ? TableColumn.fromPartial(object.autoIncrVal)
        : undefined
    return message
  },
}

function createBaseTablePartitionRoot(): TablePartitionRoot {
  return { treeRef: undefined, partitionImpl: 0 }
}

export const TablePartitionRoot = {
  encode(
    message: TablePartitionRoot,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.treeRef !== undefined) {
      BlockRef.encode(message.treeRef, writer.uint32(10).fork()).ldelim()
    }
    if (message.partitionImpl !== 0) {
      writer.uint32(16).int32(message.partitionImpl)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TablePartitionRoot {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTablePartitionRoot()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.treeRef = BlockRef.decode(reader, reader.uint32())
          break
        case 2:
          message.partitionImpl = reader.int32() as any
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<TablePartitionRoot, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TablePartitionRoot | TablePartitionRoot[]>
      | Iterable<TablePartitionRoot | TablePartitionRoot[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TablePartitionRoot.encode(p).finish()]
        }
      } else {
        yield* [TablePartitionRoot.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TablePartitionRoot>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TablePartitionRoot> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TablePartitionRoot.decode(p)]
        }
      } else {
        yield* [TablePartitionRoot.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TablePartitionRoot {
    return {
      treeRef: isSet(object.treeRef)
        ? BlockRef.fromJSON(object.treeRef)
        : undefined,
      partitionImpl: isSet(object.partitionImpl)
        ? partitionImplFromJSON(object.partitionImpl)
        : 0,
    }
  },

  toJSON(message: TablePartitionRoot): unknown {
    const obj: any = {}
    message.treeRef !== undefined &&
      (obj.treeRef = message.treeRef
        ? BlockRef.toJSON(message.treeRef)
        : undefined)
    message.partitionImpl !== undefined &&
      (obj.partitionImpl = partitionImplToJSON(message.partitionImpl))
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<TablePartitionRoot>, I>>(
    object: I
  ): TablePartitionRoot {
    const message = createBaseTablePartitionRoot()
    message.treeRef =
      object.treeRef !== undefined && object.treeRef !== null
        ? BlockRef.fromPartial(object.treeRef)
        : undefined
    message.partitionImpl = object.partitionImpl ?? 0
    return message
  },
}

function createBaseTablePartitionRow(): TablePartitionRow {
  return { rowNonce: Long.UZERO, tableRowRef: undefined }
}

export const TablePartitionRow = {
  encode(
    message: TablePartitionRow,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (!message.rowNonce.isZero()) {
      writer.uint32(8).uint64(message.rowNonce)
    }
    if (message.tableRowRef !== undefined) {
      BlockRef.encode(message.tableRowRef, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TablePartitionRow {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTablePartitionRow()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.rowNonce = reader.uint64() as Long
          break
        case 2:
          message.tableRowRef = BlockRef.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<TablePartitionRow, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TablePartitionRow | TablePartitionRow[]>
      | Iterable<TablePartitionRow | TablePartitionRow[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TablePartitionRow.encode(p).finish()]
        }
      } else {
        yield* [TablePartitionRow.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TablePartitionRow>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TablePartitionRow> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TablePartitionRow.decode(p)]
        }
      } else {
        yield* [TablePartitionRow.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TablePartitionRow {
    return {
      rowNonce: isSet(object.rowNonce)
        ? Long.fromValue(object.rowNonce)
        : Long.UZERO,
      tableRowRef: isSet(object.tableRowRef)
        ? BlockRef.fromJSON(object.tableRowRef)
        : undefined,
    }
  },

  toJSON(message: TablePartitionRow): unknown {
    const obj: any = {}
    message.rowNonce !== undefined &&
      (obj.rowNonce = (message.rowNonce || Long.UZERO).toString())
    message.tableRowRef !== undefined &&
      (obj.tableRowRef = message.tableRowRef
        ? BlockRef.toJSON(message.tableRowRef)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<TablePartitionRow>, I>>(
    object: I
  ): TablePartitionRow {
    const message = createBaseTablePartitionRow()
    message.rowNonce =
      object.rowNonce !== undefined && object.rowNonce !== null
        ? Long.fromValue(object.rowNonce)
        : Long.UZERO
    message.tableRowRef =
      object.tableRowRef !== undefined && object.tableRowRef !== null
        ? BlockRef.fromPartial(object.tableRowRef)
        : undefined
    return message
  },
}

function createBaseTableRow(): TableRow {
  return { columns: [] }
}

export const TableRow = {
  encode(
    message: TableRow,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    for (const v of message.columns) {
      TableColumn.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TableRow {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTableRow()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.columns.push(TableColumn.decode(reader, reader.uint32()))
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<TableRow, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TableRow | TableRow[]>
      | Iterable<TableRow | TableRow[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TableRow.encode(p).finish()]
        }
      } else {
        yield* [TableRow.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TableRow>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TableRow> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TableRow.decode(p)]
        }
      } else {
        yield* [TableRow.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TableRow {
    return {
      columns: Array.isArray(object?.columns)
        ? object.columns.map((e: any) => TableColumn.fromJSON(e))
        : [],
    }
  },

  toJSON(message: TableRow): unknown {
    const obj: any = {}
    if (message.columns) {
      obj.columns = message.columns.map((e) =>
        e ? TableColumn.toJSON(e) : undefined
      )
    } else {
      obj.columns = []
    }
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<TableRow>, I>>(object: I): TableRow {
    const message = createBaseTableRow()
    message.columns =
      object.columns?.map((e) => TableColumn.fromPartial(e)) || []
    return message
  },
}

function createBaseTableColumn(): TableColumn {
  return { msgpackBlob: undefined }
}

export const TableColumn = {
  encode(
    message: TableColumn,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.msgpackBlob !== undefined) {
      MsgpackBlob.encode(message.msgpackBlob, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TableColumn {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTableColumn()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.msgpackBlob = MsgpackBlob.decode(reader, reader.uint32())
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<TableColumn, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TableColumn | TableColumn[]>
      | Iterable<TableColumn | TableColumn[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TableColumn.encode(p).finish()]
        }
      } else {
        yield* [TableColumn.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TableColumn>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TableColumn> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TableColumn.decode(p)]
        }
      } else {
        yield* [TableColumn.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TableColumn {
    return {
      msgpackBlob: isSet(object.msgpackBlob)
        ? MsgpackBlob.fromJSON(object.msgpackBlob)
        : undefined,
    }
  },

  toJSON(message: TableColumn): unknown {
    const obj: any = {}
    message.msgpackBlob !== undefined &&
      (obj.msgpackBlob = message.msgpackBlob
        ? MsgpackBlob.toJSON(message.msgpackBlob)
        : undefined)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<TableColumn>, I>>(
    object: I
  ): TableColumn {
    const message = createBaseTableColumn()
    message.msgpackBlob =
      object.msgpackBlob !== undefined && object.msgpackBlob !== null
        ? MsgpackBlob.fromPartial(object.msgpackBlob)
        : undefined
    return message
  },
}

function createBaseTableSchema(): TableSchema {
  return { columns: [] }
}

export const TableSchema = {
  encode(
    message: TableSchema,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    for (const v of message.columns) {
      TableSchemaColumn.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TableSchema {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTableSchema()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.columns.push(
            TableSchemaColumn.decode(reader, reader.uint32())
          )
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<TableSchema, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TableSchema | TableSchema[]>
      | Iterable<TableSchema | TableSchema[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TableSchema.encode(p).finish()]
        }
      } else {
        yield* [TableSchema.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TableSchema>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TableSchema> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TableSchema.decode(p)]
        }
      } else {
        yield* [TableSchema.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TableSchema {
    return {
      columns: Array.isArray(object?.columns)
        ? object.columns.map((e: any) => TableSchemaColumn.fromJSON(e))
        : [],
    }
  },

  toJSON(message: TableSchema): unknown {
    const obj: any = {}
    if (message.columns) {
      obj.columns = message.columns.map((e) =>
        e ? TableSchemaColumn.toJSON(e) : undefined
      )
    } else {
      obj.columns = []
    }
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<TableSchema>, I>>(
    object: I
  ): TableSchema {
    const message = createBaseTableSchema()
    message.columns =
      object.columns?.map((e) => TableSchemaColumn.fromPartial(e)) || []
    return message
  },
}

function createBaseTableSchemaColumn(): TableSchemaColumn {
  return {
    name: '',
    columnType: '',
    defaultValueExpr: '',
    autoIncrement: false,
    nullable: false,
    source: '',
    primaryKey: false,
    comment: '',
    extra: '',
  }
}

export const TableSchemaColumn = {
  encode(
    message: TableSchemaColumn,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.name !== '') {
      writer.uint32(10).string(message.name)
    }
    if (message.columnType !== '') {
      writer.uint32(18).string(message.columnType)
    }
    if (message.defaultValueExpr !== '') {
      writer.uint32(26).string(message.defaultValueExpr)
    }
    if (message.autoIncrement === true) {
      writer.uint32(32).bool(message.autoIncrement)
    }
    if (message.nullable === true) {
      writer.uint32(40).bool(message.nullable)
    }
    if (message.source !== '') {
      writer.uint32(50).string(message.source)
    }
    if (message.primaryKey === true) {
      writer.uint32(56).bool(message.primaryKey)
    }
    if (message.comment !== '') {
      writer.uint32(66).string(message.comment)
    }
    if (message.extra !== '') {
      writer.uint32(74).string(message.extra)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TableSchemaColumn {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTableSchemaColumn()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.name = reader.string()
          break
        case 2:
          message.columnType = reader.string()
          break
        case 3:
          message.defaultValueExpr = reader.string()
          break
        case 4:
          message.autoIncrement = reader.bool()
          break
        case 5:
          message.nullable = reader.bool()
          break
        case 6:
          message.source = reader.string()
          break
        case 7:
          message.primaryKey = reader.bool()
          break
        case 8:
          message.comment = reader.string()
          break
        case 9:
          message.extra = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<TableSchemaColumn, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TableSchemaColumn | TableSchemaColumn[]>
      | Iterable<TableSchemaColumn | TableSchemaColumn[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TableSchemaColumn.encode(p).finish()]
        }
      } else {
        yield* [TableSchemaColumn.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TableSchemaColumn>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<TableSchemaColumn> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [TableSchemaColumn.decode(p)]
        }
      } else {
        yield* [TableSchemaColumn.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): TableSchemaColumn {
    return {
      name: isSet(object.name) ? String(object.name) : '',
      columnType: isSet(object.columnType) ? String(object.columnType) : '',
      defaultValueExpr: isSet(object.defaultValueExpr)
        ? String(object.defaultValueExpr)
        : '',
      autoIncrement: isSet(object.autoIncrement)
        ? Boolean(object.autoIncrement)
        : false,
      nullable: isSet(object.nullable) ? Boolean(object.nullable) : false,
      source: isSet(object.source) ? String(object.source) : '',
      primaryKey: isSet(object.primaryKey) ? Boolean(object.primaryKey) : false,
      comment: isSet(object.comment) ? String(object.comment) : '',
      extra: isSet(object.extra) ? String(object.extra) : '',
    }
  },

  toJSON(message: TableSchemaColumn): unknown {
    const obj: any = {}
    message.name !== undefined && (obj.name = message.name)
    message.columnType !== undefined && (obj.columnType = message.columnType)
    message.defaultValueExpr !== undefined &&
      (obj.defaultValueExpr = message.defaultValueExpr)
    message.autoIncrement !== undefined &&
      (obj.autoIncrement = message.autoIncrement)
    message.nullable !== undefined && (obj.nullable = message.nullable)
    message.source !== undefined && (obj.source = message.source)
    message.primaryKey !== undefined && (obj.primaryKey = message.primaryKey)
    message.comment !== undefined && (obj.comment = message.comment)
    message.extra !== undefined && (obj.extra = message.extra)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<TableSchemaColumn>, I>>(
    object: I
  ): TableSchemaColumn {
    const message = createBaseTableSchemaColumn()
    message.name = object.name ?? ''
    message.columnType = object.columnType ?? ''
    message.defaultValueExpr = object.defaultValueExpr ?? ''
    message.autoIncrement = object.autoIncrement ?? false
    message.nullable = object.nullable ?? false
    message.source = object.source ?? ''
    message.primaryKey = object.primaryKey ?? false
    message.comment = object.comment ?? ''
    message.extra = object.extra ?? ''
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
