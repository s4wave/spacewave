/* eslint-disable */
import {
  HashType,
  hashTypeFromJSON,
  hashTypeToJSON,
} from '@go/github.com/aperturerobotics/bifrost/hash/hash.pb.js'
import { Info } from '@go/github.com/aperturerobotics/controllerbus/controller/controller.pb.js'
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'
import { BucketInfo } from '../bucket/bucket.pb.js'

export const protobufPackage = 'volume'

/** VolumeInfo contains basic information about a volume. */
export interface VolumeInfo {
  /** VolumeId is the volume ID as determined by the controller. */
  volumeId: string
  /** PeerId is the peer ID of the volume. */
  peerId: string
  /** PeerPub is the pem public key of the volume. */
  peerPub: string
  /**
   * ControllerInfo is information about the volume controller.
   * Note: may be empty.
   */
  controllerInfo: Info | undefined
  /**
   * HashType is the default block hash type to use for blocks.
   * If unset (0 value) will use default for Hydra (BLAKE3).
   */
  hashType: HashType
}

/** VolumeBucketInfo is information about a bucket in a volume. */
export interface VolumeBucketInfo {
  /** BucketInfo is the bucket information. */
  bucketInfo: BucketInfo | undefined
  /** VolumeInfo is the volume containing the bucket instance. */
  volumeInfo: VolumeInfo | undefined
}

/** ListBucketsRequest is a list buckets directive in proto form. */
export interface ListBucketsRequest {
  /**
   * BucketId limits information to a specific bucket.
   * Can be empty.
   */
  bucketId: string
  /**
   * VolumeIdRe limits to specific volumes by regex.
   * Can be empty.
   * Cannot be specified if VolumeIDList is set.
   */
  volumeIdRe: string
  /**
   * VolumeIdList returns a specific list of volumes to list.
   * If empty, uses the VolumeIDRe field instead.
   * Cannot be specified if VolumeIDRe is set.
   */
  volumeIdList: string[]
}

function createBaseVolumeInfo(): VolumeInfo {
  return {
    volumeId: '',
    peerId: '',
    peerPub: '',
    controllerInfo: undefined,
    hashType: 0,
  }
}

export const VolumeInfo = {
  encode(
    message: VolumeInfo,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.volumeId !== '') {
      writer.uint32(10).string(message.volumeId)
    }
    if (message.peerId !== '') {
      writer.uint32(18).string(message.peerId)
    }
    if (message.peerPub !== '') {
      writer.uint32(26).string(message.peerPub)
    }
    if (message.controllerInfo !== undefined) {
      Info.encode(message.controllerInfo, writer.uint32(34).fork()).ldelim()
    }
    if (message.hashType !== 0) {
      writer.uint32(40).int32(message.hashType)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): VolumeInfo {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseVolumeInfo()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.volumeId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.peerId = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.peerPub = reader.string()
          continue
        case 4:
          if (tag !== 34) {
            break
          }

          message.controllerInfo = Info.decode(reader, reader.uint32())
          continue
        case 5:
          if (tag !== 40) {
            break
          }

          message.hashType = reader.int32() as any
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
  // Transform<VolumeInfo, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<VolumeInfo | VolumeInfo[]>
      | Iterable<VolumeInfo | VolumeInfo[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [VolumeInfo.encode(p).finish()]
        }
      } else {
        yield* [VolumeInfo.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, VolumeInfo>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<VolumeInfo> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [VolumeInfo.decode(p)]
        }
      } else {
        yield* [VolumeInfo.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): VolumeInfo {
    return {
      volumeId: isSet(object.volumeId) ? String(object.volumeId) : '',
      peerId: isSet(object.peerId) ? String(object.peerId) : '',
      peerPub: isSet(object.peerPub) ? String(object.peerPub) : '',
      controllerInfo: isSet(object.controllerInfo)
        ? Info.fromJSON(object.controllerInfo)
        : undefined,
      hashType: isSet(object.hashType) ? hashTypeFromJSON(object.hashType) : 0,
    }
  },

  toJSON(message: VolumeInfo): unknown {
    const obj: any = {}
    message.volumeId !== undefined && (obj.volumeId = message.volumeId)
    message.peerId !== undefined && (obj.peerId = message.peerId)
    message.peerPub !== undefined && (obj.peerPub = message.peerPub)
    message.controllerInfo !== undefined &&
      (obj.controllerInfo = message.controllerInfo
        ? Info.toJSON(message.controllerInfo)
        : undefined)
    message.hashType !== undefined &&
      (obj.hashType = hashTypeToJSON(message.hashType))
    return obj
  },

  create<I extends Exact<DeepPartial<VolumeInfo>, I>>(base?: I): VolumeInfo {
    return VolumeInfo.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<VolumeInfo>, I>>(
    object: I
  ): VolumeInfo {
    const message = createBaseVolumeInfo()
    message.volumeId = object.volumeId ?? ''
    message.peerId = object.peerId ?? ''
    message.peerPub = object.peerPub ?? ''
    message.controllerInfo =
      object.controllerInfo !== undefined && object.controllerInfo !== null
        ? Info.fromPartial(object.controllerInfo)
        : undefined
    message.hashType = object.hashType ?? 0
    return message
  },
}

function createBaseVolumeBucketInfo(): VolumeBucketInfo {
  return { bucketInfo: undefined, volumeInfo: undefined }
}

export const VolumeBucketInfo = {
  encode(
    message: VolumeBucketInfo,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.bucketInfo !== undefined) {
      BucketInfo.encode(message.bucketInfo, writer.uint32(10).fork()).ldelim()
    }
    if (message.volumeInfo !== undefined) {
      VolumeInfo.encode(message.volumeInfo, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): VolumeBucketInfo {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseVolumeBucketInfo()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.bucketInfo = BucketInfo.decode(reader, reader.uint32())
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.volumeInfo = VolumeInfo.decode(reader, reader.uint32())
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
  // Transform<VolumeBucketInfo, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<VolumeBucketInfo | VolumeBucketInfo[]>
      | Iterable<VolumeBucketInfo | VolumeBucketInfo[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [VolumeBucketInfo.encode(p).finish()]
        }
      } else {
        yield* [VolumeBucketInfo.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, VolumeBucketInfo>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<VolumeBucketInfo> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [VolumeBucketInfo.decode(p)]
        }
      } else {
        yield* [VolumeBucketInfo.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): VolumeBucketInfo {
    return {
      bucketInfo: isSet(object.bucketInfo)
        ? BucketInfo.fromJSON(object.bucketInfo)
        : undefined,
      volumeInfo: isSet(object.volumeInfo)
        ? VolumeInfo.fromJSON(object.volumeInfo)
        : undefined,
    }
  },

  toJSON(message: VolumeBucketInfo): unknown {
    const obj: any = {}
    message.bucketInfo !== undefined &&
      (obj.bucketInfo = message.bucketInfo
        ? BucketInfo.toJSON(message.bucketInfo)
        : undefined)
    message.volumeInfo !== undefined &&
      (obj.volumeInfo = message.volumeInfo
        ? VolumeInfo.toJSON(message.volumeInfo)
        : undefined)
    return obj
  },

  create<I extends Exact<DeepPartial<VolumeBucketInfo>, I>>(
    base?: I
  ): VolumeBucketInfo {
    return VolumeBucketInfo.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<VolumeBucketInfo>, I>>(
    object: I
  ): VolumeBucketInfo {
    const message = createBaseVolumeBucketInfo()
    message.bucketInfo =
      object.bucketInfo !== undefined && object.bucketInfo !== null
        ? BucketInfo.fromPartial(object.bucketInfo)
        : undefined
    message.volumeInfo =
      object.volumeInfo !== undefined && object.volumeInfo !== null
        ? VolumeInfo.fromPartial(object.volumeInfo)
        : undefined
    return message
  },
}

function createBaseListBucketsRequest(): ListBucketsRequest {
  return { bucketId: '', volumeIdRe: '', volumeIdList: [] }
}

export const ListBucketsRequest = {
  encode(
    message: ListBucketsRequest,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.bucketId !== '') {
      writer.uint32(10).string(message.bucketId)
    }
    if (message.volumeIdRe !== '') {
      writer.uint32(18).string(message.volumeIdRe)
    }
    for (const v of message.volumeIdList) {
      writer.uint32(26).string(v!)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ListBucketsRequest {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseListBucketsRequest()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.bucketId = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.volumeIdRe = reader.string()
          continue
        case 3:
          if (tag !== 26) {
            break
          }

          message.volumeIdList.push(reader.string())
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
  // Transform<ListBucketsRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ListBucketsRequest | ListBucketsRequest[]>
      | Iterable<ListBucketsRequest | ListBucketsRequest[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ListBucketsRequest.encode(p).finish()]
        }
      } else {
        yield* [ListBucketsRequest.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ListBucketsRequest>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ListBucketsRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ListBucketsRequest.decode(p)]
        }
      } else {
        yield* [ListBucketsRequest.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ListBucketsRequest {
    return {
      bucketId: isSet(object.bucketId) ? String(object.bucketId) : '',
      volumeIdRe: isSet(object.volumeIdRe) ? String(object.volumeIdRe) : '',
      volumeIdList: Array.isArray(object?.volumeIdList)
        ? object.volumeIdList.map((e: any) => String(e))
        : [],
    }
  },

  toJSON(message: ListBucketsRequest): unknown {
    const obj: any = {}
    message.bucketId !== undefined && (obj.bucketId = message.bucketId)
    message.volumeIdRe !== undefined && (obj.volumeIdRe = message.volumeIdRe)
    if (message.volumeIdList) {
      obj.volumeIdList = message.volumeIdList.map((e) => e)
    } else {
      obj.volumeIdList = []
    }
    return obj
  },

  create<I extends Exact<DeepPartial<ListBucketsRequest>, I>>(
    base?: I
  ): ListBucketsRequest {
    return ListBucketsRequest.fromPartial(base ?? {})
  },

  fromPartial<I extends Exact<DeepPartial<ListBucketsRequest>, I>>(
    object: I
  ): ListBucketsRequest {
    const message = createBaseListBucketsRequest()
    message.bucketId = object.bucketId ?? ''
    message.volumeIdRe = object.volumeIdRe ?? ''
    message.volumeIdList = object.volumeIdList?.map((e) => e) || []
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
