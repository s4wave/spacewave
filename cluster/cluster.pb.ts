/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'forge.cluster'

/**
 * Cluster associates a set of Worker with a list of Jobs.
 * The name is used for API calls and command-line / UI tools.
 *
 * Graph links:
 * - forge/cluster-job: links to ongoing Jobs managed by the Cluster.
 * - forge/cluster-worker: links to Worker schedulable in the Cluster.
 *
 * There are multiple ways to modify Cluster state:
 * - Job, Target, World: can be modified outside of the scope of the Cluster.
 * - Pass, Execution: assigned peer can submit tx to update w/ validation.
 * - Cluster: assigned peer ID can submit forge/cluster/tx w/ validation.
 * - Cluster: can be updated w/ ops: ClusterCreate, ClusterAssignJob, ClusterDelete
 */
export interface Cluster {
  /**
   * Name is the cluster name.
   * Should be user-readable: like "my-cluster-1"
   * Must be a valid DNS label as defined in RFC 1123.
   */
  name: string
  /**
   * PeerId is the identifier of the peer controlling the Cluster.
   * This peer runs the high-level cluster scheduler.
   * Cannot be empty.
   */
  peerId: string
}

/** ClusterCreateOp creates a new Cluster. */
export interface ClusterCreateOp {
  /** ClusterKey is the object key for the new Cluster. */
  clusterKey: string
  /** Name is the name to create. */
  name: string
  /**
   * PeerId is the identifier of the peer controlling the Cluster.
   * This peer runs the high-level cluster scheduler.
   * Cannot be empty.
   */
  peerId: string
}

/**
 * ClusterAssignPeerOp sets the peer_id for the Cluster controller.
 * Creates and/or links to a Keypair for the peer id.
 */
export interface ClusterAssignPeerOp {
  /** ClusterKey is the object key for the Cluster. */
  clusterKey: string
  /**
   * PeerId is the updated peer id for the cluster.
   * Cannot be empty.
   */
  peerId: string
}

/** ClusterAssignJobOp assigns a Job to a Cluster. */
export interface ClusterAssignJobOp {
  /** ClusterKey is the object key for the Cluster. */
  clusterKey: string
  /** JobKey is the object key for the Job. */
  jobKey: string
}

/** ClusterAssignWorkerOp assigns a Worker to a Cluster. */
export interface ClusterAssignWorkerOp {
  /** ClusterKey is the object key for the Cluster. */
  clusterKey: string
  /** WorkerKey is the object key for the Worker. */
  workerKey: string
}

/** ClusterStartJobOp transitions a assigned Job from PENDING to RUNNING. */
export interface ClusterStartJobOp {
  /** ClusterKey is the object key for the Cluster. */
  clusterKey: string
  /** JobKey is the object key for the Job. */
  jobKey: string
}

/**
 * ClusterAssignTaskOp sets the peer_id for a Task to the Cluster peer ID.
 * Verifies that the cluster, job, and task are linked properly.
 */
export interface ClusterAssignTaskOp {
  /** ClusterKey is the object key for the Cluster. */
  clusterKey: string
  /** JobKey is the object key for the Job. */
  jobKey: string
  /** TaskKey is the object key for the Task. */
  taskKey: string
}

/**
 * ClusterCompleteJobOp transitions a assigned Job from RUNNING to COMPLETE.
 *
 * Must be in the RUNNING state.
 * All assigned Task must be in COMPLETE state.
 * If any failed, the Job will also fail.
 */
export interface ClusterCompleteJobOp {
  /** ClusterKey is the object key for the Cluster. */
  clusterKey: string
  /** JobKey is the object key for the Job. */
  jobKey: string
}

function createBaseCluster(): Cluster {
  return { name: '', peerId: '' }
}

export const Cluster = {
  encode(
    message: Cluster,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.name !== '') {
      writer.uint32(10).string(message.name)
    }
    if (message.peerId !== '') {
      writer.uint32(18).string(message.peerId)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Cluster {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseCluster()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.name = reader.string()
          break
        case 2:
          message.peerId = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Cluster, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Cluster | Cluster[]> | Iterable<Cluster | Cluster[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Cluster.encode(p).finish()]
        }
      } else {
        yield* [Cluster.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Cluster>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<Cluster> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Cluster.decode(p)]
        }
      } else {
        yield* [Cluster.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): Cluster {
    return {
      name: isSet(object.name) ? String(object.name) : '',
      peerId: isSet(object.peerId) ? String(object.peerId) : '',
    }
  },

  toJSON(message: Cluster): unknown {
    const obj: any = {}
    message.name !== undefined && (obj.name = message.name)
    message.peerId !== undefined && (obj.peerId = message.peerId)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<Cluster>, I>>(object: I): Cluster {
    const message = createBaseCluster()
    message.name = object.name ?? ''
    message.peerId = object.peerId ?? ''
    return message
  },
}

function createBaseClusterCreateOp(): ClusterCreateOp {
  return { clusterKey: '', name: '', peerId: '' }
}

export const ClusterCreateOp = {
  encode(
    message: ClusterCreateOp,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.clusterKey !== '') {
      writer.uint32(10).string(message.clusterKey)
    }
    if (message.name !== '') {
      writer.uint32(18).string(message.name)
    }
    if (message.peerId !== '') {
      writer.uint32(26).string(message.peerId)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ClusterCreateOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseClusterCreateOp()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.clusterKey = reader.string()
          break
        case 2:
          message.name = reader.string()
          break
        case 3:
          message.peerId = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ClusterCreateOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ClusterCreateOp | ClusterCreateOp[]>
      | Iterable<ClusterCreateOp | ClusterCreateOp[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ClusterCreateOp.encode(p).finish()]
        }
      } else {
        yield* [ClusterCreateOp.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ClusterCreateOp>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ClusterCreateOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ClusterCreateOp.decode(p)]
        }
      } else {
        yield* [ClusterCreateOp.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ClusterCreateOp {
    return {
      clusterKey: isSet(object.clusterKey) ? String(object.clusterKey) : '',
      name: isSet(object.name) ? String(object.name) : '',
      peerId: isSet(object.peerId) ? String(object.peerId) : '',
    }
  },

  toJSON(message: ClusterCreateOp): unknown {
    const obj: any = {}
    message.clusterKey !== undefined && (obj.clusterKey = message.clusterKey)
    message.name !== undefined && (obj.name = message.name)
    message.peerId !== undefined && (obj.peerId = message.peerId)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<ClusterCreateOp>, I>>(
    object: I
  ): ClusterCreateOp {
    const message = createBaseClusterCreateOp()
    message.clusterKey = object.clusterKey ?? ''
    message.name = object.name ?? ''
    message.peerId = object.peerId ?? ''
    return message
  },
}

function createBaseClusterAssignPeerOp(): ClusterAssignPeerOp {
  return { clusterKey: '', peerId: '' }
}

export const ClusterAssignPeerOp = {
  encode(
    message: ClusterAssignPeerOp,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.clusterKey !== '') {
      writer.uint32(10).string(message.clusterKey)
    }
    if (message.peerId !== '') {
      writer.uint32(18).string(message.peerId)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ClusterAssignPeerOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseClusterAssignPeerOp()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.clusterKey = reader.string()
          break
        case 2:
          message.peerId = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ClusterAssignPeerOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ClusterAssignPeerOp | ClusterAssignPeerOp[]>
      | Iterable<ClusterAssignPeerOp | ClusterAssignPeerOp[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ClusterAssignPeerOp.encode(p).finish()]
        }
      } else {
        yield* [ClusterAssignPeerOp.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ClusterAssignPeerOp>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ClusterAssignPeerOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ClusterAssignPeerOp.decode(p)]
        }
      } else {
        yield* [ClusterAssignPeerOp.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ClusterAssignPeerOp {
    return {
      clusterKey: isSet(object.clusterKey) ? String(object.clusterKey) : '',
      peerId: isSet(object.peerId) ? String(object.peerId) : '',
    }
  },

  toJSON(message: ClusterAssignPeerOp): unknown {
    const obj: any = {}
    message.clusterKey !== undefined && (obj.clusterKey = message.clusterKey)
    message.peerId !== undefined && (obj.peerId = message.peerId)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<ClusterAssignPeerOp>, I>>(
    object: I
  ): ClusterAssignPeerOp {
    const message = createBaseClusterAssignPeerOp()
    message.clusterKey = object.clusterKey ?? ''
    message.peerId = object.peerId ?? ''
    return message
  },
}

function createBaseClusterAssignJobOp(): ClusterAssignJobOp {
  return { clusterKey: '', jobKey: '' }
}

export const ClusterAssignJobOp = {
  encode(
    message: ClusterAssignJobOp,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.clusterKey !== '') {
      writer.uint32(10).string(message.clusterKey)
    }
    if (message.jobKey !== '') {
      writer.uint32(18).string(message.jobKey)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ClusterAssignJobOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseClusterAssignJobOp()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.clusterKey = reader.string()
          break
        case 2:
          message.jobKey = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ClusterAssignJobOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ClusterAssignJobOp | ClusterAssignJobOp[]>
      | Iterable<ClusterAssignJobOp | ClusterAssignJobOp[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ClusterAssignJobOp.encode(p).finish()]
        }
      } else {
        yield* [ClusterAssignJobOp.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ClusterAssignJobOp>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ClusterAssignJobOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ClusterAssignJobOp.decode(p)]
        }
      } else {
        yield* [ClusterAssignJobOp.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ClusterAssignJobOp {
    return {
      clusterKey: isSet(object.clusterKey) ? String(object.clusterKey) : '',
      jobKey: isSet(object.jobKey) ? String(object.jobKey) : '',
    }
  },

  toJSON(message: ClusterAssignJobOp): unknown {
    const obj: any = {}
    message.clusterKey !== undefined && (obj.clusterKey = message.clusterKey)
    message.jobKey !== undefined && (obj.jobKey = message.jobKey)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<ClusterAssignJobOp>, I>>(
    object: I
  ): ClusterAssignJobOp {
    const message = createBaseClusterAssignJobOp()
    message.clusterKey = object.clusterKey ?? ''
    message.jobKey = object.jobKey ?? ''
    return message
  },
}

function createBaseClusterAssignWorkerOp(): ClusterAssignWorkerOp {
  return { clusterKey: '', workerKey: '' }
}

export const ClusterAssignWorkerOp = {
  encode(
    message: ClusterAssignWorkerOp,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.clusterKey !== '') {
      writer.uint32(10).string(message.clusterKey)
    }
    if (message.workerKey !== '') {
      writer.uint32(18).string(message.workerKey)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): ClusterAssignWorkerOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseClusterAssignWorkerOp()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.clusterKey = reader.string()
          break
        case 2:
          message.workerKey = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ClusterAssignWorkerOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ClusterAssignWorkerOp | ClusterAssignWorkerOp[]>
      | Iterable<ClusterAssignWorkerOp | ClusterAssignWorkerOp[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ClusterAssignWorkerOp.encode(p).finish()]
        }
      } else {
        yield* [ClusterAssignWorkerOp.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ClusterAssignWorkerOp>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ClusterAssignWorkerOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ClusterAssignWorkerOp.decode(p)]
        }
      } else {
        yield* [ClusterAssignWorkerOp.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ClusterAssignWorkerOp {
    return {
      clusterKey: isSet(object.clusterKey) ? String(object.clusterKey) : '',
      workerKey: isSet(object.workerKey) ? String(object.workerKey) : '',
    }
  },

  toJSON(message: ClusterAssignWorkerOp): unknown {
    const obj: any = {}
    message.clusterKey !== undefined && (obj.clusterKey = message.clusterKey)
    message.workerKey !== undefined && (obj.workerKey = message.workerKey)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<ClusterAssignWorkerOp>, I>>(
    object: I
  ): ClusterAssignWorkerOp {
    const message = createBaseClusterAssignWorkerOp()
    message.clusterKey = object.clusterKey ?? ''
    message.workerKey = object.workerKey ?? ''
    return message
  },
}

function createBaseClusterStartJobOp(): ClusterStartJobOp {
  return { clusterKey: '', jobKey: '' }
}

export const ClusterStartJobOp = {
  encode(
    message: ClusterStartJobOp,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.clusterKey !== '') {
      writer.uint32(10).string(message.clusterKey)
    }
    if (message.jobKey !== '') {
      writer.uint32(18).string(message.jobKey)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ClusterStartJobOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseClusterStartJobOp()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.clusterKey = reader.string()
          break
        case 2:
          message.jobKey = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ClusterStartJobOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ClusterStartJobOp | ClusterStartJobOp[]>
      | Iterable<ClusterStartJobOp | ClusterStartJobOp[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ClusterStartJobOp.encode(p).finish()]
        }
      } else {
        yield* [ClusterStartJobOp.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ClusterStartJobOp>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ClusterStartJobOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ClusterStartJobOp.decode(p)]
        }
      } else {
        yield* [ClusterStartJobOp.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ClusterStartJobOp {
    return {
      clusterKey: isSet(object.clusterKey) ? String(object.clusterKey) : '',
      jobKey: isSet(object.jobKey) ? String(object.jobKey) : '',
    }
  },

  toJSON(message: ClusterStartJobOp): unknown {
    const obj: any = {}
    message.clusterKey !== undefined && (obj.clusterKey = message.clusterKey)
    message.jobKey !== undefined && (obj.jobKey = message.jobKey)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<ClusterStartJobOp>, I>>(
    object: I
  ): ClusterStartJobOp {
    const message = createBaseClusterStartJobOp()
    message.clusterKey = object.clusterKey ?? ''
    message.jobKey = object.jobKey ?? ''
    return message
  },
}

function createBaseClusterAssignTaskOp(): ClusterAssignTaskOp {
  return { clusterKey: '', jobKey: '', taskKey: '' }
}

export const ClusterAssignTaskOp = {
  encode(
    message: ClusterAssignTaskOp,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.clusterKey !== '') {
      writer.uint32(10).string(message.clusterKey)
    }
    if (message.jobKey !== '') {
      writer.uint32(18).string(message.jobKey)
    }
    if (message.taskKey !== '') {
      writer.uint32(26).string(message.taskKey)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): ClusterAssignTaskOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseClusterAssignTaskOp()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.clusterKey = reader.string()
          break
        case 2:
          message.jobKey = reader.string()
          break
        case 3:
          message.taskKey = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ClusterAssignTaskOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ClusterAssignTaskOp | ClusterAssignTaskOp[]>
      | Iterable<ClusterAssignTaskOp | ClusterAssignTaskOp[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ClusterAssignTaskOp.encode(p).finish()]
        }
      } else {
        yield* [ClusterAssignTaskOp.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ClusterAssignTaskOp>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ClusterAssignTaskOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ClusterAssignTaskOp.decode(p)]
        }
      } else {
        yield* [ClusterAssignTaskOp.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ClusterAssignTaskOp {
    return {
      clusterKey: isSet(object.clusterKey) ? String(object.clusterKey) : '',
      jobKey: isSet(object.jobKey) ? String(object.jobKey) : '',
      taskKey: isSet(object.taskKey) ? String(object.taskKey) : '',
    }
  },

  toJSON(message: ClusterAssignTaskOp): unknown {
    const obj: any = {}
    message.clusterKey !== undefined && (obj.clusterKey = message.clusterKey)
    message.jobKey !== undefined && (obj.jobKey = message.jobKey)
    message.taskKey !== undefined && (obj.taskKey = message.taskKey)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<ClusterAssignTaskOp>, I>>(
    object: I
  ): ClusterAssignTaskOp {
    const message = createBaseClusterAssignTaskOp()
    message.clusterKey = object.clusterKey ?? ''
    message.jobKey = object.jobKey ?? ''
    message.taskKey = object.taskKey ?? ''
    return message
  },
}

function createBaseClusterCompleteJobOp(): ClusterCompleteJobOp {
  return { clusterKey: '', jobKey: '' }
}

export const ClusterCompleteJobOp = {
  encode(
    message: ClusterCompleteJobOp,
    writer: _m0.Writer = _m0.Writer.create()
  ): _m0.Writer {
    if (message.clusterKey !== '') {
      writer.uint32(10).string(message.clusterKey)
    }
    if (message.jobKey !== '') {
      writer.uint32(18).string(message.jobKey)
    }
    return writer
  },

  decode(
    input: _m0.Reader | Uint8Array,
    length?: number
  ): ClusterCompleteJobOp {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseClusterCompleteJobOp()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.clusterKey = reader.string()
          break
        case 2:
          message.jobKey = reader.string()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<ClusterCompleteJobOp, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<ClusterCompleteJobOp | ClusterCompleteJobOp[]>
      | Iterable<ClusterCompleteJobOp | ClusterCompleteJobOp[]>
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ClusterCompleteJobOp.encode(p).finish()]
        }
      } else {
        yield* [ClusterCompleteJobOp.encode(pkt).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, ClusterCompleteJobOp>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>
  ): AsyncIterable<ClusterCompleteJobOp> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [ClusterCompleteJobOp.decode(p)]
        }
      } else {
        yield* [ClusterCompleteJobOp.decode(pkt)]
      }
    }
  },

  fromJSON(object: any): ClusterCompleteJobOp {
    return {
      clusterKey: isSet(object.clusterKey) ? String(object.clusterKey) : '',
      jobKey: isSet(object.jobKey) ? String(object.jobKey) : '',
    }
  },

  toJSON(message: ClusterCompleteJobOp): unknown {
    const obj: any = {}
    message.clusterKey !== undefined && (obj.clusterKey = message.clusterKey)
    message.jobKey !== undefined && (obj.jobKey = message.jobKey)
    return obj
  },

  fromPartial<I extends Exact<DeepPartial<ClusterCompleteJobOp>, I>>(
    object: I
  ): ClusterCompleteJobOp {
    const message = createBaseClusterCompleteJobOp()
    message.clusterKey = object.clusterKey ?? ''
    message.jobKey = object.jobKey ?? ''
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
