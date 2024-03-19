/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { BlockRef } from '../../block/block.pb.js'
import { MsgpackBlob } from '../../block/msgpack/msgpack.pb.js'
import { KeyValueStore } from '../../kvtx/block/kvtx.pb.js'

export const protobufPackage = 'mysql'

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
  /** CollationId is the method ID of the method used to control sorting. */
  collationId: number
}

/** TablePartitionRoot contains the root of a table partition. */
export interface TablePartitionRoot {
  /**
   * RowKeyValue is the key/value tree of objects.
   * Key: row_nonce uint64 encoded with big endian
   * Value: cid.BlockRef -> Object
   */
  rowKeyValue: KeyValueStore | undefined
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
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRoot()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.databases.push(RootDb.decode(reader, reader.uint32()))
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
  // Transform<Root, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Root | Root[]> | Iterable<Root | Root[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Root.encode(p).finish()]
        }
      } else {
        yield* [Root.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Root>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Root> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [Root.decode(p)]
        }
      } else {
        yield* [Root.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): Root {
    return {
      databases: globalThis.Array.isArray(object?.databases)
        ? object.databases.map((e: any) => RootDb.fromJSON(e))
        : [],
    }
  },

  toJSON(message: Root): unknown {
    const obj: any = {}
    if (message.databases?.length) {
      obj.databases = message.databases.map((e) => RootDb.toJSON(e))
    }
    return obj
  },

  create<I extends Exact<DeepPartial<Root>, I>>(base?: I): Root {
    return Root.fromPartial(base ?? ({} as any))
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
    writer: _m0.Writer = _m0.Writer.create(),
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
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseRootDb()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.name = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.ref = BlockRef.decode(reader, reader.uint32())
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
  // Transform<RootDb, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<RootDb | RootDb[]> | Iterable<RootDb | RootDb[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [RootDb.encode(p).finish()]
        }
      } else {
        yield* [RootDb.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RootDb>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RootDb> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [RootDb.decode(p)]
        }
      } else {
        yield* [RootDb.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): RootDb {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : '',
      ref: isSet(object.ref) ? BlockRef.fromJSON(object.ref) : undefined,
    }
  },

  toJSON(message: RootDb): unknown {
    const obj: any = {}
    if (message.name !== '') {
      obj.name = message.name
    }
    if (message.ref !== undefined) {
      obj.ref = BlockRef.toJSON(message.ref)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<RootDb>, I>>(base?: I): RootDb {
    return RootDb.fromPartial(base ?? ({} as any))
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
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.tables) {
      DatabaseRootTable.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DatabaseRoot {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseDatabaseRoot()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.tables.push(DatabaseRootTable.decode(reader, reader.uint32()))
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
  // Transform<DatabaseRoot, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<DatabaseRoot | DatabaseRoot[]>
      | Iterable<DatabaseRoot | DatabaseRoot[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [DatabaseRoot.encode(p).finish()]
        }
      } else {
        yield* [DatabaseRoot.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, DatabaseRoot>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<DatabaseRoot> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [DatabaseRoot.decode(p)]
        }
      } else {
        yield* [DatabaseRoot.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): DatabaseRoot {
    return {
      tables: globalThis.Array.isArray(object?.tables)
        ? object.tables.map((e: any) => DatabaseRootTable.fromJSON(e))
        : [],
    }
  },

  toJSON(message: DatabaseRoot): unknown {
    const obj: any = {}
    if (message.tables?.length) {
      obj.tables = message.tables.map((e) => DatabaseRootTable.toJSON(e))
    }
    return obj
  },

  create<I extends Exact<DeepPartial<DatabaseRoot>, I>>(
    base?: I,
  ): DatabaseRoot {
    return DatabaseRoot.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<DatabaseRoot>, I>>(
    object: I,
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
    writer: _m0.Writer = _m0.Writer.create(),
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
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseDatabaseRootTable()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.name = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.ref = BlockRef.decode(reader, reader.uint32())
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
  // Transform<DatabaseRootTable, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<DatabaseRootTable | DatabaseRootTable[]>
      | Iterable<DatabaseRootTable | DatabaseRootTable[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [DatabaseRootTable.encode(p).finish()]
        }
      } else {
        yield* [DatabaseRootTable.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, DatabaseRootTable>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<DatabaseRootTable> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [DatabaseRootTable.decode(p)]
        }
      } else {
        yield* [DatabaseRootTable.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): DatabaseRootTable {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : '',
      ref: isSet(object.ref) ? BlockRef.fromJSON(object.ref) : undefined,
    }
  },

  toJSON(message: DatabaseRootTable): unknown {
    const obj: any = {}
    if (message.name !== '') {
      obj.name = message.name
    }
    if (message.ref !== undefined) {
      obj.ref = BlockRef.toJSON(message.ref)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<DatabaseRootTable>, I>>(
    base?: I,
  ): DatabaseRootTable {
    return DatabaseRootTable.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<DatabaseRootTable>, I>>(
    object: I,
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
    primaryKeyOrdinals: [],
    tablePartitions: [],
    rowNonce: Long.UZERO,
    autoIncrVal: undefined,
    collationId: 0,
  }
}

export const TableRoot = {
  encode(
    message: TableRoot,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.tableSchema !== undefined) {
      TableSchema.encode(message.tableSchema, writer.uint32(10).fork()).ldelim()
    }
    writer.uint32(42).fork()
    for (const v of message.primaryKeyOrdinals) {
      writer.int32(v)
    }
    writer.ldelim()
    for (const v of message.tablePartitions) {
      TablePartitionRoot.encode(v!, writer.uint32(18).fork()).ldelim()
    }
    if (!message.rowNonce.equals(Long.UZERO)) {
      writer.uint32(24).uint64(message.rowNonce)
    }
    if (message.autoIncrVal !== undefined) {
      TableColumn.encode(message.autoIncrVal, writer.uint32(34).fork()).ldelim()
    }
    if (message.collationId !== 0) {
      writer.uint32(48).uint32(message.collationId)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TableRoot {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTableRoot()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.tableSchema = TableSchema.decode(reader, reader.uint32())
          continue
        case 5:
          if (tag === 40) {
            message.primaryKeyOrdinals.push(reader.int32())

            continue
          }

          if (tag === 42) {
            const end2 = reader.uint32() + reader.pos
            while (reader.pos < end2) {
              message.primaryKeyOrdinals.push(reader.int32())
            }

            continue
          }

          break
        case 2:
          if (tag !== 18) {
            break
          }

          message.tablePartitions.push(
            TablePartitionRoot.decode(reader, reader.uint32()),
          )
          continue
        case 3:
          if (tag !== 24) {
            break
          }

          message.rowNonce = reader.uint64() as Long
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.autoIncrVal = TableColumn.decode(reader, reader.uint32())
          continue
        case 6:
          if (tag !== 48) {
            break
          }

          message.collationId = reader.uint32()
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
  // Transform<TableRoot, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TableRoot | TableRoot[]>
      | Iterable<TableRoot | TableRoot[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [TableRoot.encode(p).finish()]
        }
      } else {
        yield* [TableRoot.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TableRoot>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<TableRoot> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [TableRoot.decode(p)]
        }
      } else {
        yield* [TableRoot.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): TableRoot {
    return {
      tableSchema: isSet(object.tableSchema)
        ? TableSchema.fromJSON(object.tableSchema)
        : undefined,
      primaryKeyOrdinals: globalThis.Array.isArray(object?.primaryKeyOrdinals)
        ? object.primaryKeyOrdinals.map((e: any) => globalThis.Number(e))
        : [],
      tablePartitions: globalThis.Array.isArray(object?.tablePartitions)
        ? object.tablePartitions.map((e: any) => TablePartitionRoot.fromJSON(e))
        : [],
      rowNonce: isSet(object.rowNonce)
        ? Long.fromValue(object.rowNonce)
        : Long.UZERO,
      autoIncrVal: isSet(object.autoIncrVal)
        ? TableColumn.fromJSON(object.autoIncrVal)
        : undefined,
      collationId: isSet(object.collationId)
        ? globalThis.Number(object.collationId)
        : 0,
    }
  },

  toJSON(message: TableRoot): unknown {
    const obj: any = {}
    if (message.tableSchema !== undefined) {
      obj.tableSchema = TableSchema.toJSON(message.tableSchema)
    }
    if (message.primaryKeyOrdinals?.length) {
      obj.primaryKeyOrdinals = message.primaryKeyOrdinals.map((e) =>
        Math.round(e),
      )
    }
    if (message.tablePartitions?.length) {
      obj.tablePartitions = message.tablePartitions.map((e) =>
        TablePartitionRoot.toJSON(e),
      )
    }
    if (!message.rowNonce.equals(Long.UZERO)) {
      obj.rowNonce = (message.rowNonce || Long.UZERO).toString()
    }
    if (message.autoIncrVal !== undefined) {
      obj.autoIncrVal = TableColumn.toJSON(message.autoIncrVal)
    }
    if (message.collationId !== 0) {
      obj.collationId = Math.round(message.collationId)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<TableRoot>, I>>(base?: I): TableRoot {
    return TableRoot.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<TableRoot>, I>>(
    object: I,
  ): TableRoot {
    const message = createBaseTableRoot()
    message.tableSchema =
      object.tableSchema !== undefined && object.tableSchema !== null
        ? TableSchema.fromPartial(object.tableSchema)
        : undefined
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
    message.collationId = object.collationId ?? 0
    return message
  },
}

function createBaseTablePartitionRoot(): TablePartitionRoot {
  return { rowKeyValue: undefined }
}

export const TablePartitionRoot = {
  encode(
    message: TablePartitionRoot,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.rowKeyValue !== undefined) {
      KeyValueStore.encode(
        message.rowKeyValue,
        writer.uint32(10).fork(),
      ).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TablePartitionRoot {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTablePartitionRoot()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.rowKeyValue = KeyValueStore.decode(reader, reader.uint32())
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
  // Transform<TablePartitionRoot, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TablePartitionRoot | TablePartitionRoot[]>
      | Iterable<TablePartitionRoot | TablePartitionRoot[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [TablePartitionRoot.encode(p).finish()]
        }
      } else {
        yield* [TablePartitionRoot.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TablePartitionRoot>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<TablePartitionRoot> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [TablePartitionRoot.decode(p)]
        }
      } else {
        yield* [TablePartitionRoot.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): TablePartitionRoot {
    return {
      rowKeyValue: isSet(object.rowKeyValue)
        ? KeyValueStore.fromJSON(object.rowKeyValue)
        : undefined,
    }
  },

  toJSON(message: TablePartitionRoot): unknown {
    const obj: any = {}
    if (message.rowKeyValue !== undefined) {
      obj.rowKeyValue = KeyValueStore.toJSON(message.rowKeyValue)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<TablePartitionRoot>, I>>(
    base?: I,
  ): TablePartitionRoot {
    return TablePartitionRoot.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<TablePartitionRoot>, I>>(
    object: I,
  ): TablePartitionRoot {
    const message = createBaseTablePartitionRoot()
    message.rowKeyValue =
      object.rowKeyValue !== undefined && object.rowKeyValue !== null
        ? KeyValueStore.fromPartial(object.rowKeyValue)
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
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.columns) {
      TableColumn.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TableRow {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTableRow()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.columns.push(TableColumn.decode(reader, reader.uint32()))
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
  // Transform<TableRow, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TableRow | TableRow[]>
      | Iterable<TableRow | TableRow[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [TableRow.encode(p).finish()]
        }
      } else {
        yield* [TableRow.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TableRow>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<TableRow> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [TableRow.decode(p)]
        }
      } else {
        yield* [TableRow.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): TableRow {
    return {
      columns: globalThis.Array.isArray(object?.columns)
        ? object.columns.map((e: any) => TableColumn.fromJSON(e))
        : [],
    }
  },

  toJSON(message: TableRow): unknown {
    const obj: any = {}
    if (message.columns?.length) {
      obj.columns = message.columns.map((e) => TableColumn.toJSON(e))
    }
    return obj
  },

  create<I extends Exact<DeepPartial<TableRow>, I>>(base?: I): TableRow {
    return TableRow.fromPartial(base ?? ({} as any))
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
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.msgpackBlob !== undefined) {
      MsgpackBlob.encode(message.msgpackBlob, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TableColumn {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTableColumn()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.msgpackBlob = MsgpackBlob.decode(reader, reader.uint32())
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
  // Transform<TableColumn, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TableColumn | TableColumn[]>
      | Iterable<TableColumn | TableColumn[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [TableColumn.encode(p).finish()]
        }
      } else {
        yield* [TableColumn.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TableColumn>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<TableColumn> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [TableColumn.decode(p)]
        }
      } else {
        yield* [TableColumn.decode(pkt as any)]
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
    if (message.msgpackBlob !== undefined) {
      obj.msgpackBlob = MsgpackBlob.toJSON(message.msgpackBlob)
    }
    return obj
  },

  create<I extends Exact<DeepPartial<TableColumn>, I>>(base?: I): TableColumn {
    return TableColumn.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<TableColumn>, I>>(
    object: I,
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
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    for (const v of message.columns) {
      TableSchemaColumn.encode(v!, writer.uint32(10).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): TableSchema {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTableSchema()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.columns.push(
            TableSchemaColumn.decode(reader, reader.uint32()),
          )
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
  // Transform<TableSchema, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TableSchema | TableSchema[]>
      | Iterable<TableSchema | TableSchema[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [TableSchema.encode(p).finish()]
        }
      } else {
        yield* [TableSchema.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TableSchema>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<TableSchema> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [TableSchema.decode(p)]
        }
      } else {
        yield* [TableSchema.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): TableSchema {
    return {
      columns: globalThis.Array.isArray(object?.columns)
        ? object.columns.map((e: any) => TableSchemaColumn.fromJSON(e))
        : [],
    }
  },

  toJSON(message: TableSchema): unknown {
    const obj: any = {}
    if (message.columns?.length) {
      obj.columns = message.columns.map((e) => TableSchemaColumn.toJSON(e))
    }
    return obj
  },

  create<I extends Exact<DeepPartial<TableSchema>, I>>(base?: I): TableSchema {
    return TableSchema.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<TableSchema>, I>>(
    object: I,
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
    writer: _m0.Writer = _m0.Writer.create(),
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
    if (message.autoIncrement !== false) {
      writer.uint32(32).bool(message.autoIncrement)
    }
    if (message.nullable !== false) {
      writer.uint32(40).bool(message.nullable)
    }
    if (message.source !== '') {
      writer.uint32(50).string(message.source)
    }
    if (message.primaryKey !== false) {
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
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseTableSchemaColumn()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.name = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.columnType = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.defaultValueExpr = reader.string()
          continue
        case 4:
          if (tag !== 32) {
            break
          }

          message.autoIncrement = reader.bool()
          continue
        case 5:
          if (tag !== 40) {
            break
          }

          message.nullable = reader.bool()
          continue
        case 6:
          if (tag !== 50) {
            break
          }

          message.source = reader.string()
          continue
        case 7:
          if (tag !== 56) {
            break
          }

          message.primaryKey = reader.bool()
          continue
        case 8:
          if (tag !== 66) {
            break
          }

          message.comment = reader.string()
          continue
        case 9:
          if (tag !== 74) {
            break
          }

          message.extra = reader.string()
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
  // Transform<TableSchemaColumn, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<TableSchemaColumn | TableSchemaColumn[]>
      | Iterable<TableSchemaColumn | TableSchemaColumn[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [TableSchemaColumn.encode(p).finish()]
        }
      } else {
        yield* [TableSchemaColumn.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, TableSchemaColumn>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<TableSchemaColumn> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [TableSchemaColumn.decode(p)]
        }
      } else {
        yield* [TableSchemaColumn.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): TableSchemaColumn {
    return {
      name: isSet(object.name) ? globalThis.String(object.name) : '',
      columnType: isSet(object.columnType)
        ? globalThis.String(object.columnType)
        : '',
      defaultValueExpr: isSet(object.defaultValueExpr)
        ? globalThis.String(object.defaultValueExpr)
        : '',
      autoIncrement: isSet(object.autoIncrement)
        ? globalThis.Boolean(object.autoIncrement)
        : false,
      nullable: isSet(object.nullable)
        ? globalThis.Boolean(object.nullable)
        : false,
      source: isSet(object.source) ? globalThis.String(object.source) : '',
      primaryKey: isSet(object.primaryKey)
        ? globalThis.Boolean(object.primaryKey)
        : false,
      comment: isSet(object.comment) ? globalThis.String(object.comment) : '',
      extra: isSet(object.extra) ? globalThis.String(object.extra) : '',
    }
  },

  toJSON(message: TableSchemaColumn): unknown {
    const obj: any = {}
    if (message.name !== '') {
      obj.name = message.name
    }
    if (message.columnType !== '') {
      obj.columnType = message.columnType
    }
    if (message.defaultValueExpr !== '') {
      obj.defaultValueExpr = message.defaultValueExpr
    }
    if (message.autoIncrement !== false) {
      obj.autoIncrement = message.autoIncrement
    }
    if (message.nullable !== false) {
      obj.nullable = message.nullable
    }
    if (message.source !== '') {
      obj.source = message.source
    }
    if (message.primaryKey !== false) {
      obj.primaryKey = message.primaryKey
    }
    if (message.comment !== '') {
      obj.comment = message.comment
    }
    if (message.extra !== '') {
      obj.extra = message.extra
    }
    return obj
  },

  create<I extends Exact<DeepPartial<TableSchemaColumn>, I>>(
    base?: I,
  ): TableSchemaColumn {
    return TableSchemaColumn.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<TableSchemaColumn>, I>>(
    object: I,
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
