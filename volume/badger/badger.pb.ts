/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { Config as Config1 } from '../../store/kvkey/kvkey.pb.js'
import { Config as Config3 } from '../../store/kvtx/kv_tx.pb.js'
import { Config as Config2 } from '../controller/controller.pb.js'

export const protobufPackage = 'volume.badger'

/**
 * FileLoadingMode specifies how data in LSM table files and value log files
 * should be loaded.
 */
export enum FileLoadingMode {
  /** FileLoadingMode_DEFAULT - FileLoadingMode_DEFAULT is the default. */
  FileLoadingMode_DEFAULT = 0,
  /** FileLoadingMode_FileIO - FileIO indicates that files must be loaded using standard I/O */
  FileLoadingMode_FileIO = 1,
  /** FileLoadingMode_LoadToRAM - LoadToRAM indicates that file must be loaded into RAM */
  FileLoadingMode_LoadToRAM = 2,
  /** FileLoadingMode_MemoryMap - MemoryMap indicates that that the file must be memory-mapped */
  FileLoadingMode_MemoryMap = 3,
  UNRECOGNIZED = -1,
}

export function fileLoadingModeFromJSON(object: any): FileLoadingMode {
  switch (object) {
    case 0:
    case 'FileLoadingMode_DEFAULT':
      return FileLoadingMode.FileLoadingMode_DEFAULT
    case 1:
    case 'FileLoadingMode_FileIO':
      return FileLoadingMode.FileLoadingMode_FileIO
    case 2:
    case 'FileLoadingMode_LoadToRAM':
      return FileLoadingMode.FileLoadingMode_LoadToRAM
    case 3:
    case 'FileLoadingMode_MemoryMap':
      return FileLoadingMode.FileLoadingMode_MemoryMap
    case -1:
    case 'UNRECOGNIZED':
    default:
      return FileLoadingMode.UNRECOGNIZED
  }
}

export function fileLoadingModeToJSON(object: FileLoadingMode): string {
  switch (object) {
    case FileLoadingMode.FileLoadingMode_DEFAULT:
      return 'FileLoadingMode_DEFAULT'
    case FileLoadingMode.FileLoadingMode_FileIO:
      return 'FileLoadingMode_FileIO'
    case FileLoadingMode.FileLoadingMode_LoadToRAM:
      return 'FileLoadingMode_LoadToRAM'
    case FileLoadingMode.FileLoadingMode_MemoryMap:
      return 'FileLoadingMode_MemoryMap'
    case FileLoadingMode.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/**
 * Config is the badger volume controller config.
 * Flag Dir is the only mandatory flag.
 */
export interface Config {
  /**
   * Dir is the directory to store the data in.
   * Should exist and be writable.
   */
  dir: string
  /**
   * ValueDir is the directory to store the value log in.
   * Can be the same as dir.
   * Should exist and be writable.
   * If empty, defaults to Dir.
   */
  valueDir: string
  /** KvKeyOpts are key/value options. */
  kvKeyOpts: Config1 | undefined
  /**
   * NoGenerateKey indicates the controller should not generate a private key if
   * one is not already present. Setting this to false will cause the system to
   * create a new private key if one is not present in the store at startup. If
   * no key is in the store at startup and this is true, returns an error.
   */
  noGenerateKey: boolean
  /**
   * NoWriteKey indicates the controller should not write a private key to
   * storage if it generates one. This results in an ephemeral volume peer
   * identity if there is no key present in the store already.
   *
   * Has no effect if the store has a peer private key.
   */
  noWriteKey: boolean
  /** Verbose indicates we should log every operation. */
  verbose: boolean
  /** BadgerDebug indicates to enable badger debug log messages. */
  badgerDebug: boolean
  /** VolumeConfig is the volume controller config. */
  volumeConfig: Config2 | undefined
  /** StoreConfig is the store configuration for kvtx. */
  storeConfig: Config3 | undefined
  /**
   * TableLoadingMode indicates how the LSM tree should be accessed
   * Defaults to LoadToRAM
   */
  tableLoadingMode: FileLoadingMode
  /**
   * ValueLogLoadingMode indicates how the value log should be accessed
   * Defaults to MemoryMap
   */
  valueLogLoadingMode: FileLoadingMode
  /**
   * NumVersionsToKeep indicates how many versions to keep per key.
   * Defaults to 1.
   */
  numVersionsToKeep: number
  /**
   * MaxTableSize is the max size each table/file can be.
   * Defaults to  64 << 20
   */
  maxTableSize: Long
  /**
   * LevelSizeMultiplier is SizeOf(Li+1)/SizeOf(Li).
   * Defaults to 10.
   */
  levelSizeMultiplier: number
  /**
   * MaxLevels is the maximum number of levels of compaction.
   * Defaults to 7
   */
  maxLevels: number
  /**
   * ValueThreshold if value size >= threshold, only store offsets in tree.
   * Defaults to 32
   */
  valueThreshold: number
  /**
   * NumMemtables is the Maximum number of tables to keep in memory, before
   * stalling.
   * Defaults to 5.
   */
  numMemtables: number
  /**
   * NumLevelZeroTables affects how LSM tree L0 is handled.
   * Maximum number of Level 0 tables before we start compacting.
   * Defaults to 5.
   */
  numLevelZeroTables: number
  /**
   * NumLevelZeroTablesStall is the number of level 0 tables to stall at until
   * l0 is compacted.
   * Defaults to 10.
   */
  numLevelZeroTablesStall: number
  /**
   * LevelOneSize is the maximum total size for L1.
   * Defaults to 256 << 20
   */
  levelOneSize: Long
  /**
   * ValueLogFileSize is the size of single value log file.
   * (2^30 - 1)*2 when mmapping < 2^31 - 1, max int32.
   * -1 so 2*ValueLogFileSize won't overflow on 32-bit systems.
   * Defaults to 1<<30 - 1
   */
  valueLogFileSize: Long
  /**
   * ValueLogMaxEntries is the max number of entries a value log file can hold
   * (approximately). A value log file would be determined by the smaller of its
   * file size and max entries.
   * Defaults to 1000000
   */
  valueLogMaxEntries: number
  /**
   * NumCompactors is the number of compaction workers to run concurrently.
   * Defaults to 3.
   */
  numCompactors: number
  /**
   * Truncate value log to delete corrupt data, if any.
   * Defaults to false.
   */
  truncate: boolean
  /**
   * NoSyncWrites indicates all writes should not require disk sync before
   * returning. If set, writes will return before the filesystem has confirmed
   * the write is complete. Setting this to false will increase performance but
   * introduces risk of data loss.
   */
  noSyncWrites: boolean
}

function createBaseConfig(): Config {
  return {
    dir: '',
    valueDir: '',
    kvKeyOpts: undefined,
    noGenerateKey: false,
    noWriteKey: false,
    verbose: false,
    badgerDebug: false,
    volumeConfig: undefined,
    storeConfig: undefined,
    tableLoadingMode: 0,
    valueLogLoadingMode: 0,
    numVersionsToKeep: 0,
    maxTableSize: Long.UZERO,
    levelSizeMultiplier: 0,
    maxLevels: 0,
    valueThreshold: 0,
    numMemtables: 0,
    numLevelZeroTables: 0,
    numLevelZeroTablesStall: 0,
    levelOneSize: Long.UZERO,
    valueLogFileSize: Long.UZERO,
    valueLogMaxEntries: 0,
    numCompactors: 0,
    truncate: false,
    noSyncWrites: false,
  }
}

export const Config = {
  encode(
    message: Config,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.dir !== '') {
      writer.uint32(10).string(message.dir)
    }
    if (message.valueDir !== '') {
      writer.uint32(18).string(message.valueDir)
    }
    if (message.kvKeyOpts !== undefined) {
      Config1.encode(message.kvKeyOpts, writer.uint32(26).fork()).ldelim()
    }
    if (message.noGenerateKey === true) {
      writer.uint32(32).bool(message.noGenerateKey)
    }
    if (message.noWriteKey === true) {
      writer.uint32(200).bool(message.noWriteKey)
    }
    if (message.verbose === true) {
      writer.uint32(168).bool(message.verbose)
    }
    if (message.badgerDebug === true) {
      writer.uint32(192).bool(message.badgerDebug)
    }
    if (message.volumeConfig !== undefined) {
      Config2.encode(message.volumeConfig, writer.uint32(178).fork()).ldelim()
    }
    if (message.storeConfig !== undefined) {
      Config3.encode(message.storeConfig, writer.uint32(186).fork()).ldelim()
    }
    if (message.tableLoadingMode !== 0) {
      writer.uint32(40).int32(message.tableLoadingMode)
    }
    if (message.valueLogLoadingMode !== 0) {
      writer.uint32(48).int32(message.valueLogLoadingMode)
    }
    if (message.numVersionsToKeep !== 0) {
      writer.uint32(56).uint32(message.numVersionsToKeep)
    }
    if (!message.maxTableSize.isZero()) {
      writer.uint32(64).uint64(message.maxTableSize)
    }
    if (message.levelSizeMultiplier !== 0) {
      writer.uint32(72).uint32(message.levelSizeMultiplier)
    }
    if (message.maxLevels !== 0) {
      writer.uint32(80).uint32(message.maxLevels)
    }
    if (message.valueThreshold !== 0) {
      writer.uint32(88).uint32(message.valueThreshold)
    }
    if (message.numMemtables !== 0) {
      writer.uint32(96).uint32(message.numMemtables)
    }
    if (message.numLevelZeroTables !== 0) {
      writer.uint32(104).uint32(message.numLevelZeroTables)
    }
    if (message.numLevelZeroTablesStall !== 0) {
      writer.uint32(112).uint32(message.numLevelZeroTablesStall)
    }
    if (!message.levelOneSize.isZero()) {
      writer.uint32(120).uint64(message.levelOneSize)
    }
    if (!message.valueLogFileSize.isZero()) {
      writer.uint32(128).uint64(message.valueLogFileSize)
    }
    if (message.valueLogMaxEntries !== 0) {
      writer.uint32(136).uint32(message.valueLogMaxEntries)
    }
    if (message.numCompactors !== 0) {
      writer.uint32(144).uint32(message.numCompactors)
    }
    if (message.truncate === true) {
      writer.uint32(152).bool(message.truncate)
    }
    if (message.noSyncWrites === true) {
      writer.uint32(160).bool(message.noSyncWrites)
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
          if (tag != 10) {
            break
          }

          message.dir = reader.string()
          continue
        case 2:
          if (tag != 18) {
            break
          }

          message.valueDir = reader.string()
          continue
        case 3:
          if (tag != 26) {
            break
          }

          message.kvKeyOpts = Config1.decode(reader, reader.uint32())
          continue
        case 4:
          if (tag != 32) {
            break
          }

          message.noGenerateKey = reader.bool()
          continue
        case 25:
          if (tag != 200) {
            break
          }

          message.noWriteKey = reader.bool()
          continue
        case 21:
          if (tag != 168) {
            break
          }

          message.verbose = reader.bool()
          continue
        case 24:
          if (tag != 192) {
            break
          }

          message.badgerDebug = reader.bool()
          continue
        case 22:
          if (tag != 178) {
            break
          }

          message.volumeConfig = Config2.decode(reader, reader.uint32())
          continue
        case 23:
          if (tag != 186) {
            break
          }

          message.storeConfig = Config3.decode(reader, reader.uint32())
          continue
        case 5:
          if (tag != 40) {
            break
          }

          message.tableLoadingMode = reader.int32() as any
          continue
        case 6:
          if (tag != 48) {
            break
          }

          message.valueLogLoadingMode = reader.int32() as any
          continue
        case 7:
          if (tag != 56) {
            break
          }

          message.numVersionsToKeep = reader.uint32()
          continue
        case 8:
          if (tag != 64) {
            break
          }

          message.maxTableSize = reader.uint64() as Long
          continue
        case 9:
          if (tag != 72) {
            break
          }

          message.levelSizeMultiplier = reader.uint32()
          continue
        case 10:
          if (tag != 80) {
            break
          }

          message.maxLevels = reader.uint32()
          continue
        case 11:
          if (tag != 88) {
            break
          }

          message.valueThreshold = reader.uint32()
          continue
        case 12:
          if (tag != 96) {
            break
          }

          message.numMemtables = reader.uint32()
          continue
        case 13:
          if (tag != 104) {
            break
          }

          message.numLevelZeroTables = reader.uint32()
          continue
        case 14:
          if (tag != 112) {
            break
          }

          message.numLevelZeroTablesStall = reader.uint32()
          continue
        case 15:
          if (tag != 120) {
            break
          }

          message.levelOneSize = reader.uint64() as Long
          continue
        case 16:
          if (tag != 128) {
            break
          }

          message.valueLogFileSize = reader.uint64() as Long
          continue
        case 17:
          if (tag != 136) {
            break
          }

          message.valueLogMaxEntries = reader.uint32()
          continue
        case 18:
          if (tag != 144) {
            break
          }

          message.numCompactors = reader.uint32()
          continue
        case 19:
          if (tag != 152) {
            break
          }

          message.truncate = reader.bool()
          continue
        case 20:
          if (tag != 160) {
            break
          }

          message.noSyncWrites = reader.bool()
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
      dir: isSet(object.dir) ? String(object.dir) : '',
      valueDir: isSet(object.valueDir) ? String(object.valueDir) : '',
      kvKeyOpts: isSet(object.kvKeyOpts)
        ? Config1.fromJSON(object.kvKeyOpts)
        : undefined,
      noGenerateKey: isSet(object.noGenerateKey)
        ? Boolean(object.noGenerateKey)
        : false,
      noWriteKey: isSet(object.noWriteKey) ? Boolean(object.noWriteKey) : false,
      verbose: isSet(object.verbose) ? Boolean(object.verbose) : false,
      badgerDebug: isSet(object.badgerDebug)
        ? Boolean(object.badgerDebug)
        : false,
      volumeConfig: isSet(object.volumeConfig)
        ? Config2.fromJSON(object.volumeConfig)
        : undefined,
      storeConfig: isSet(object.storeConfig)
        ? Config3.fromJSON(object.storeConfig)
        : undefined,
      tableLoadingMode: isSet(object.tableLoadingMode)
        ? fileLoadingModeFromJSON(object.tableLoadingMode)
        : 0,
      valueLogLoadingMode: isSet(object.valueLogLoadingMode)
        ? fileLoadingModeFromJSON(object.valueLogLoadingMode)
        : 0,
      numVersionsToKeep: isSet(object.numVersionsToKeep)
        ? Number(object.numVersionsToKeep)
        : 0,
      maxTableSize: isSet(object.maxTableSize)
        ? Long.fromValue(object.maxTableSize)
        : Long.UZERO,
      levelSizeMultiplier: isSet(object.levelSizeMultiplier)
        ? Number(object.levelSizeMultiplier)
        : 0,
      maxLevels: isSet(object.maxLevels) ? Number(object.maxLevels) : 0,
      valueThreshold: isSet(object.valueThreshold)
        ? Number(object.valueThreshold)
        : 0,
      numMemtables: isSet(object.numMemtables)
        ? Number(object.numMemtables)
        : 0,
      numLevelZeroTables: isSet(object.numLevelZeroTables)
        ? Number(object.numLevelZeroTables)
        : 0,
      numLevelZeroTablesStall: isSet(object.numLevelZeroTablesStall)
        ? Number(object.numLevelZeroTablesStall)
        : 0,
      levelOneSize: isSet(object.levelOneSize)
        ? Long.fromValue(object.levelOneSize)
        : Long.UZERO,
      valueLogFileSize: isSet(object.valueLogFileSize)
        ? Long.fromValue(object.valueLogFileSize)
        : Long.UZERO,
      valueLogMaxEntries: isSet(object.valueLogMaxEntries)
        ? Number(object.valueLogMaxEntries)
        : 0,
      numCompactors: isSet(object.numCompactors)
        ? Number(object.numCompactors)
        : 0,
      truncate: isSet(object.truncate) ? Boolean(object.truncate) : false,
      noSyncWrites: isSet(object.noSyncWrites)
        ? Boolean(object.noSyncWrites)
        : false,
    }
  },

  toJSON(message: Config): unknown {
    const obj: any = {}
    message.dir !== undefined && (obj.dir = message.dir)
    message.valueDir !== undefined && (obj.valueDir = message.valueDir)
    message.kvKeyOpts !== undefined &&
      (obj.kvKeyOpts = message.kvKeyOpts
        ? Config1.toJSON(message.kvKeyOpts)
        : undefined)
    message.noGenerateKey !== undefined &&
      (obj.noGenerateKey = message.noGenerateKey)
    message.noWriteKey !== undefined && (obj.noWriteKey = message.noWriteKey)
    message.verbose !== undefined && (obj.verbose = message.verbose)
    message.badgerDebug !== undefined && (obj.badgerDebug = message.badgerDebug)
    message.volumeConfig !== undefined &&
      (obj.volumeConfig = message.volumeConfig
        ? Config2.toJSON(message.volumeConfig)
        : undefined)
    message.storeConfig !== undefined &&
      (obj.storeConfig = message.storeConfig
        ? Config3.toJSON(message.storeConfig)
        : undefined)
    message.tableLoadingMode !== undefined &&
      (obj.tableLoadingMode = fileLoadingModeToJSON(message.tableLoadingMode))
    message.valueLogLoadingMode !== undefined &&
      (obj.valueLogLoadingMode = fileLoadingModeToJSON(
        message.valueLogLoadingMode
      ))
    message.numVersionsToKeep !== undefined &&
      (obj.numVersionsToKeep = Math.round(message.numVersionsToKeep))
    message.maxTableSize !== undefined &&
      (obj.maxTableSize = (message.maxTableSize || Long.UZERO).toString())
    message.levelSizeMultiplier !== undefined &&
      (obj.levelSizeMultiplier = Math.round(message.levelSizeMultiplier))
    message.maxLevels !== undefined &&
      (obj.maxLevels = Math.round(message.maxLevels))
    message.valueThreshold !== undefined &&
      (obj.valueThreshold = Math.round(message.valueThreshold))
    message.numMemtables !== undefined &&
      (obj.numMemtables = Math.round(message.numMemtables))
    message.numLevelZeroTables !== undefined &&
      (obj.numLevelZeroTables = Math.round(message.numLevelZeroTables))
    message.numLevelZeroTablesStall !== undefined &&
      (obj.numLevelZeroTablesStall = Math.round(
        message.numLevelZeroTablesStall
      ))
    message.levelOneSize !== undefined &&
      (obj.levelOneSize = (message.levelOneSize || Long.UZERO).toString())
    message.valueLogFileSize !== undefined &&
      (obj.valueLogFileSize = (
        message.valueLogFileSize || Long.UZERO
      ).toString())
    message.valueLogMaxEntries !== undefined &&
      (obj.valueLogMaxEntries = Math.round(message.valueLogMaxEntries))
    message.numCompactors !== undefined &&
      (obj.numCompactors = Math.round(message.numCompactors))
    message.truncate !== undefined && (obj.truncate = message.truncate)
    message.noSyncWrites !== undefined &&
      (obj.noSyncWrites = message.noSyncWrites)
    return obj
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig()
    message.dir = object.dir ?? ''
    message.valueDir = object.valueDir ?? ''
    message.kvKeyOpts =
      object.kvKeyOpts !== undefined && object.kvKeyOpts !== null
        ? Config1.fromPartial(object.kvKeyOpts)
        : undefined
    message.noGenerateKey = object.noGenerateKey ?? false
    message.noWriteKey = object.noWriteKey ?? false
    message.verbose = object.verbose ?? false
    message.badgerDebug = object.badgerDebug ?? false
    message.volumeConfig =
      object.volumeConfig !== undefined && object.volumeConfig !== null
        ? Config2.fromPartial(object.volumeConfig)
        : undefined
    message.storeConfig =
      object.storeConfig !== undefined && object.storeConfig !== null
        ? Config3.fromPartial(object.storeConfig)
        : undefined
    message.tableLoadingMode = object.tableLoadingMode ?? 0
    message.valueLogLoadingMode = object.valueLogLoadingMode ?? 0
    message.numVersionsToKeep = object.numVersionsToKeep ?? 0
    message.maxTableSize =
      object.maxTableSize !== undefined && object.maxTableSize !== null
        ? Long.fromValue(object.maxTableSize)
        : Long.UZERO
    message.levelSizeMultiplier = object.levelSizeMultiplier ?? 0
    message.maxLevels = object.maxLevels ?? 0
    message.valueThreshold = object.valueThreshold ?? 0
    message.numMemtables = object.numMemtables ?? 0
    message.numLevelZeroTables = object.numLevelZeroTables ?? 0
    message.numLevelZeroTablesStall = object.numLevelZeroTablesStall ?? 0
    message.levelOneSize =
      object.levelOneSize !== undefined && object.levelOneSize !== null
        ? Long.fromValue(object.levelOneSize)
        : Long.UZERO
    message.valueLogFileSize =
      object.valueLogFileSize !== undefined && object.valueLogFileSize !== null
        ? Long.fromValue(object.valueLogFileSize)
        : Long.UZERO
    message.valueLogMaxEntries = object.valueLogMaxEntries ?? 0
    message.numCompactors = object.numCompactors ?? 0
    message.truncate = object.truncate ?? false
    message.noSyncWrites = object.noSyncWrites ?? false
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
